# Concurrent State Pattern

> **Status:** Implemented. Frame slots, ingest workers, and the render-thread engine exchange state through atomics and a bounded command channel.

Three kinds of thread coexist in this process and each has different rules. Getting the boundaries right is what keeps the render loop non-blocking and the control plane ordinary.

| Thread kind | Count | Pinned | May block? | May call SDL? | May touch the DB? |
|-------------|-------|--------|-----------|---------------|-------------------|
| Render thread | 1 (main) | Yes, for process lifetime | **Never** | **Only this one** | Never |
| Decode workers | 1 per needed source | Yes, per worker | Yes | Never | Never |
| Control plane | Many goroutines | No | Yes | Never | Yes |

Everything else in this document is about the three channels of communication between them.

## Why the Pinning

**The render thread** is pinned because SDL requires rendering and texture uploads on the thread that initialised it, and because KMSDRM requires holding DRM master for the process lifetime. See [Render Loop Pattern](render-loop-pattern.md).

**Decode workers** are pinned because libav calls are cgo and long-running. A cgo call blocks its OS thread; leaving several of them on scheduler threads makes the Go runtime spawn replacements under load and produces erratic latency across the whole process. Pinning each worker makes the cost explicit and bounded.

```go
func (w *Worker) Run(ctx context.Context) {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    // ... open input, decode loop ...
}
```

**The control plane** is ordinary Go and needs no special handling. HTTP handlers, GORM, `yt-dlp` subprocesses, and SSE fan-out are all normal.

## Channel 1: Control Plane → Engine (Commands)

A buffered channel of command values. The engine drains it non-blockingly at the top of each frame.

```go
type Command interface{ isCommand() }

type ApplyStrategy struct{ Snapshot *strategy.Snapshot }
type GotoScreen    struct{ ScreenID int64 }
type NextScreen    struct{}
type PauseTour     struct{}
type ResumeTour    struct{}
type SourceReady   struct{ SourceID int64; Slot *frame.Slot }
type SourceLost    struct{ SourceID int64; Reason error }
type SetPreview    struct{ Enabled bool; FPS int; Width int }
```

Until the SDL engine exists, `internal/schedule.Runtime` is the Stage-4 stand-in: the same command set (`ApplyStrategy`, `Next`, `Previous`, `Goto`, `Pause`, `Resume`) and an `atomic.Pointer` state snapshot that `GET /api/v1/display/state` and the SSE hub publish. Stage 4 should wrap this runtime rather than replace the command/state types.

Rules:

- **Commands carry values, not references to mutable state.** `ApplyStrategy` carries a fully-resolved immutable snapshot, not a database ID the engine would have to look up.
- **Senders never block.** The channel is buffered; a full buffer means the engine is wedged, which is a bug to surface, not to wait on. Send with a `default` case and record the drop.
- **Commands are idempotent where possible**, so a retry after a dropped send is safe.

### Strategy Snapshots

The engine never reads the database. When configuration changes, the control plane resolves everything — tour entries, screens, tiles, sources, layout geometry, transition parameters — into one immutable struct and sends it:

```go
type Snapshot struct {
    Rev     int64          // monotonic; for diagnostics and dedup
    Tour    []TourEntry
    Screens map[int64]*Screen
    Sources map[int64]*SourceRef
}
```

Once sent, the snapshot is never mutated. The engine swaps to it at a frame boundary. A configuration edit therefore cannot tear a frame, cannot block the loop on SQLite, and cannot leave the engine holding a half-applied change.

## Channel 2: Engine → Control Plane (State Snapshot)

The engine publishes an immutable state struct through an `atomic.Pointer`. Readers load it without locking.

```go
type State struct {
    At             time.Time
    DisplayReady   bool
    DisplayError   string
    ActiveScreenID int64
    NextScreenID   int64
    Transition     *TransitionState
    TourPaused     bool
    FPS            float64
    DroppedFrames  uint64
    Tiles          []TileState
    Degradations   []string
}

func (e *Engine) publishState() { e.state.Store(newState) }
func (e *Engine) State() *State { return e.state.Load() }
```

Because the struct is replaced rather than mutated, any reader gets a consistent view of one frame's worth of state with no torn reads and no lock contention with the render thread.

`GET /api/v1/display/state` returns the current snapshot. The SSE hub at `GET /api/v1/events` diffs consecutive snapshots and pushes only what changed, so the Preview page never polls.

## Channel 3: Decoder → Engine (Frame Slots)

The highest-frequency handoff, and the one that most needs to avoid locks.

Each source owns a **frame slot**: a bounded single-producer/single-consumer presentation queue.

```
pool:  [8]*Frame          // queue depth 6, plus the one being filled and the one being read
queue: up to 6 indices, oldest first

producer (decoder):  copy into a free buffer, append it to the queue,
                     evicting the oldest entry when the queue is full
consumer (renderer): peek the oldest frame's PTS, advance past everything
                     already due, present the last one it passed
```

The pool is sized so a free buffer always exists: at most six frames queued, one held by the consumer, one being filled.

### Bounded, Deliberately

The bound is the important half. A queue that can grow lets the renderer fall behind and never catch up, and converts a transient decode stall into permanent added latency. Evicting the oldest frame keeps that from happening: freshness beats completeness, and nobody watching a camera wall wants a frame from four seconds ago because the decoder briefly stuttered.

The depth is the other half, and it exists for one reason: **a newest-only handoff cannot express "not yet"**. The renderer schedules presentation against the display clock, so it needs to hold a frame that has arrived but is not due. With a single newest slot, a frame arriving a millisecond early is overwritten by its successor before the refresh it belonged to — a silent drop that is invisible on a fixed camera and reads as judder on anything that moves. Six frames is roughly 200 ms at 30 fps, well inside the latency budget and far short of anything that could accumulate.

The renderer keeps its hard guarantee: the slot's mutex covers index bookkeeping only and is never held across a frame copy, so neither side ever waits on the other for longer than a few comparisons.

### Frame Ownership

```go
type Frame struct {
    Width, Height int
    Format        Format      // NV12 or I420; DRMPrime reserved for DMA-BUF
    Planes        [][]byte    // Y, UV or Y, U, V
    Strides       []int
    PTS           time.Duration
    Received      time.Time
}
```

- The slot owns a pool of eight buffers (queue depth 6, plus the one being filled and the one being read) and sizes them on the first frame. A resolution or format change resizes them in place.
- The decoder writes packed pixels into the reserved pool buffer. Nothing on the ingest publish path allocates or copies a second time in steady state — at frame rate, across several cameras, an extra full-frame copy is the difference between a quiet bus and hundreds of megabytes per second of churn.
- The renderer owns whatever it took until its next sample; the frame it holds borrows the slot's buffer and must not be retained.
- `AVFrame` and `AVPacket` are unreferenced immediately after their contents are copied. Every allocation site pairs with a `defer`, and the `defer` goes *before* the fallible call — a failed hardware transfer can still leave buffers attached. Leaked libav references are invisible until the process is out of memory.

## Source Lifecycle

Starting and stopping decoders is control-plane work, but the engine must not see a torn transition.

```
control plane: user enables a source
  → create frame slot
  → start worker on its own locked OS thread
  → worker opens input; on first successful frame:
      → send SourceReady{SourceID, Slot} to the engine
  → engine binds the slot to any tile referencing that source

control plane: user disables or deletes a source
  → send SourceLost{SourceID} to the engine
  → engine unbinds the slot and releases the texture
  → only then cancel the worker's context and wait for it to exit
  → release the slot's buffers
```

The ordering matters: the engine unbinds **before** the worker is torn down, so the renderer can never sample a slot whose buffers are being freed.

Worker shutdown is `context.Context` cancellation plus a `sync.WaitGroup`. Because a worker may be inside a blocking libav call, it also sets an interrupt callback on the format context so cancellation is prompt rather than eventual.

## Rules

1. **SDL calls happen only on the render thread.** Any handler wanting display data reads the state snapshot instead.
2. **The engine never performs I/O.** No database, no network, no filesystem, no subprocess.
3. **Commands carry immutable values.** Never a pointer to something the sender will keep mutating.
4. **Published state is replaced, not mutated.** Always build a new struct and `Store` it.
5. **Frame slots queue a bounded handful of frames and evict the oldest when full.** Never unbounded, never blocking the producer, never holding the slot's mutex across a copy.
6. **Every worker is pinned and every pinned worker unlocks on exit.**
7. **Every libav allocation has a matching unref**, established at the allocation site with `defer`.
8. **Unbind before teardown.** The engine releases its reference to a slot before the worker owning it is cancelled.

## Testing

- **Frame slot** — Concurrent producer/consumer under `-race`, asserting no torn frames, a monotonically non-decreasing PTS at the consumer, eviction of the oldest frame when the queue fills, and that the producer never reuses the buffer the consumer is holding.
- **Presentation clock** — A simulated source and display, asserting that 30 fps on 60 Hz holds every frame for exactly two refreshes despite jittered publish times, that uneven ratios stay on a repeating pattern, and that a stall re-anchors instead of replaying its backlog.
- **Engine command handling** — Drive the engine's `apply` directly with a fake clock; no SDL needed.
- **Source lifecycle** — Start and stop workers against a synthetic source, asserting clean shutdown and no leaked goroutines (`goleak`).

Race detection is mandatory for this package: `go test -race -timeout=60s ./internal/frame/... ./internal/display/...`
