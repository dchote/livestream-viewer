# Render Loop Pattern

> **Status:** Design. Not yet implemented.

The render loop is the heart of the display plane. Its structure is dictated by one hard platform constraint and one design commitment.

**The constraint:** SDL rendering and texture uploads must happen on the thread that initialised SDL, and on the Pi that thread must hold DRM master for the process lifetime.

**The commitment:** the loop never blocks. Not on I/O, not on a mutex held by anything outside the display plane, not on a decoder, not on the database. A wall that freezes is worse than a wall showing a placeholder.

Everything below follows from those two.

## Thread Ownership

```go
func main() {
    runtime.LockOSThread() // held for the process lifetime; never unlocked

    // ... bootstrap config, open database, start control plane goroutines ...

    engine.Run(ctx) // returns only on shutdown
}
```

`runtime.LockOSThread()` in `main` pins the main goroutine to the OS thread that the process started on. SDL is initialised there and the loop runs there. It is never unlocked, because unlocking would let the Go scheduler move the goroutine and SDL would start failing in ways that are extremely hard to attribute.

The control plane starts before the loop and runs on ordinary goroutines. If the display engine cannot start, `Run` reports the failure and blocks until the context is cancelled, so the API and UI stay reachable and the user can see why the display is dark.

## Loop Structure

```go
func (e *Engine) Run(ctx context.Context) error {
    if err := e.output.Init(); err != nil {
        e.publishState(StateFailed(err))
        <-ctx.Done()
        return err
    }
    defer e.output.Close()

    for {
        select {
        case <-ctx.Done():
            return nil
        default:
        }

        e.pumpEvents()      // 1. SDL event queue — mandatory under KMSDRM
        e.drainCommands()   // 2. non-blocking read of the command channel
        e.advanceSchedule() // 3. dwell timers, transition progress
        e.sampleFrames()    // 4. newest frame per visible source
        e.uploadTextures()  // 5. main-thread-only uploads
        scene := e.compose()// 6. build the scene as data
        e.draw(scene)       // 7. issue SDL draw calls
        e.present()         // 8. RenderPresent, vsync-paced
        e.publishState()    // 9. atomic snapshot for the control plane
        e.capturePreview()  // 10. throttled, only when subscribed
    }
}
```

Every step is bounded. There is no `time.Sleep`, no frame timer, and no manual pacing — vsync in `present` is the clock.

### 1. Pump events

The SDL event queue must be drained every iteration. Under KMSDRM, failing to pump events produces a black screen with no error, which is a memorably unhelpful failure. Quit and display-change events are handled here.

### 2. Drain commands

A non-blocking drain of the command channel, bounded per iteration so a burst of commands cannot extend one frame indefinitely:

```go
for i := 0; i < maxCommandsPerFrame; i++ {
    select {
    case cmd := <-e.commands:
        e.apply(cmd)
    default:
        return
    }
}
```

Commands mutate engine-local state only. They never perform I/O and never call back into the control plane.

### 3. Advance the schedule

Tick dwell timers, decide whether a transition should begin, and compute transition progress:

```go
raw := clamp(float64(now.Sub(e.transitionStart))/float64(e.transitionDuration), 0, 1)
t := e.easing.Eval(raw)
```

**Progress is wall-clock, never frame-counted.** A frame-counted transition changes speed exactly when the loop drops frames, which is when it is most noticeable. Tile sequence timers and the tour timer are advanced the same way.

### 4. Sample frames

For each visible source, an atomic load of the newest published frame from its slot. Non-blocking by construction — if the decoder has published nothing new, the previously uploaded texture is reused and nothing further happens for that source this frame.

### 5. Upload textures

`SDL_UpdateNVTexture` per source that produced a new frame. This is the step that forces the whole main-thread design. Uploads are skipped for sources with no new frame, so a wall of 5 fps cameras on a 60 Hz display does almost no upload work.

A resolution change here destroys and recreates the texture; see [Display Pipeline](../architecture/display-pipeline.md#texture-lifecycle).

### 6. Compose

Build the scene as plain data — an ordered list of `{ texture, srcRect, dstRect, alpha, clipRect, blendMode }` — before any SDL call. Two benefits: the compositor is unit-testable without a GPU, and the scene can be logged or diffed when something looks wrong.

### 7. Draw

Walk the scene and issue SDL calls. When a transition is active, each screen composes into its own render target first, then the transition draws the two targets.

### 8. Present

One `SDL_RenderPresent`. Vsync blocks here, which is the loop's only wait and the thing that sets the frame rate.

### 9. Publish state

Atomically swap an immutable state snapshot that the control plane reads without locking. Contains the active screen, per-tile source and status, measured frame rate, dropped frame counters, and any degradations (a transition that fell back to `fade`, a source running on software decode).

### 10. Capture preview

Only when a preview client is subscribed, and throttled to the configured rate (default 2 fps). Downscales on the GPU to a small render target first, then reads back from the small target. The encode happens off-thread; this step only hands off a buffer.

## Rules

These are enforced by review and by `.cursor/rules/display-engine.mdc`.

1. **Never call SDL from anywhere but the render thread.** No exceptions, including from a handler that "just wants a screenshot."
2. **Never block in the loop.** No mutex that another plane can hold, no channel receive without `default`, no syscall, no database, no network.
3. **Never read the database from the engine.** The control plane resolves configuration and hands over immutable snapshots.
4. **Wall-clock time for all animation.** Never derive progress from a frame counter.
5. **Bound every per-frame operation.** Command drains, scene size, and texture allocations all have ceilings.
6. **Degrade, never stall.** A missing frame holds the last one; a failed texture shows a placeholder; an unavailable shader falls back to `fade`. Report the degradation in the state snapshot so the UI can show it.
7. **Round tile geometry to whole pixels.** Sub-pixel edges shimmer.

## Testing

The loop is deliberately structured so that most of it is testable without a display:

- **Layout solver** — Given a layout and an output size, assert the pixel rects.
- **Transitions** — Pure functions of eased progress. Assert the scene at `t = 0`, `0.5`, `1`.
- **Scheduler** — Inject a clock; assert the screen sequence and transition starts over a simulated timeline.
- **Compositor** — Given a strategy snapshot and a set of frame descriptors, assert the scene.

Only `draw`, `present`, and `uploadTextures` need real SDL, and those are thin. Tests that require SDL are behind a build tag and are not part of the default `go test ./...` run.

Never start the display engine in a unit test. It takes the main thread and, on a Pi, DRM master.
