# Technical Overview

> **Status:** Control plane, ingest, and windowed SDL display engine are implemented. Linux KMSDRM is coded but not verified on a panel.

livestream-viewer is a single Go binary that decodes live video streams with hardware acceleration and composites them onto a physically attached display using SDL3, while serving a Vue 3 + Vuetify management UI and REST API.

It is **platform-agnostic**: the control plane runs on any Go-supported OS; display output targets Linux DRM/KMS for headless panels and a native SDL window on desktop Linux and macOS for development. Low-cost and embedded boards (Raspberry Pi and similar SBCs) are first-class optimisation targets — hardware decode paths, capacity reporting, and packaging — not a hard requirement to build or operate the management plane.

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25+ |
| Rendering | SDL3 (3.4+) via `github.com/Zyko0/go-sdl3` |
| Video driver | KMS/DRM on headless Linux; native SDL window on desktop Linux and macOS |
| Demux and decode | FFmpeg 8.x `libav*` via `github.com/asticode/go-astiav` (cgo) |
| Hardware decode | Platform-specific (e.g. V4L2 on Linux SBCs) via FFmpeg hwaccel; software fallback everywhere |
| Stream URL resolution | `yt-dlp` invoked as an external subprocess (discovered on `PATH`; shipped in the add-on image) |
| Database | SQLite (GORM) |
| REST API | Go standard library `net/http` with router |
| API docs | OpenAPI 3.0 served as Swagger UI at `/docs` |
| Frontend | Vue 3 (Composition API), Vuetify 3, Vite, Vue Router, Pinia |
| Frontend embedding | Go `//go:embed` — frontend dist compiled into the binary |
| Configuration | TOML bootstrap + SQLite runtime config, editable via REST API |
| Logging | Structured logging (`slog`) |
| Packaging | Home Assistant add-on image (primary); `.deb` via nFPM; single binary |

## The Two Planes

The application is not a typical server. It has a hard structural split driven by a platform constraint: **SDL rendering and texture uploads must happen on the thread that initialised SDL**, and on headless Linux that thread typically holds DRM master for the whole lifetime of the process. Everything else — HTTP, database, stream resolution — is ordinary Go concurrency.

This produces two planes:

1. **Display plane** — Owns the process's main OS thread (`runtime.LockOSThread`). Runs the render loop at the display refresh rate. Never blocks on I/O, never blocks on a decoder, never touches the database.

2. **Control plane** — Ordinary goroutines. REST API, SPA serving, SQLite persistence, source probing, and stream URL resolution. Communicates with the display plane exclusively by sending commands over a channel and reading an atomically-published state snapshot.

The decoders sit between the two: each runs on its own locked OS thread (because libav calls are cgo and long-running), produces frames into a per-source slot, and is started and stopped by the control plane.

```
┌────────────────────────────────────────────────────────────────────┐
│                        livestream-viewer                            │
│                                                                    │
│  ── Control plane (goroutines) ──────────────────────────────────  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │
│  │ REST API +   │  │  SQLite      │  │  Source resolver         │ │
│  │ SPA  :8099   │  │  (GORM)      │  │  (yt-dlp, probe, refresh)│ │
│  │ /api /docs / │  │              │  │                          │ │
│  └──────┬───────┘  └──────┬───────┘  └────────────┬─────────────┘ │
│         │                 │                        │               │
│         └────────┬────────┴────────────────────────┘               │
│                  │                                                 │
│         ┌────────▼─────────┐        ┌──────────────────────┐      │
│         │  Engine command  │        │  Engine state        │      │
│         │  channel  (→)    │        │  snapshot  (←)       │      │
│         └────────┬─────────┘        └──────────▲───────────┘      │
│                  │                             │                   │
│  ── Display plane (main OS thread) ────────────┼─────────────────  │
│         ┌────────▼─────────────────────────────┴──────────┐        │
│         │              Render loop  (vsync-paced)         │        │
│         │  scheduler → compositor → transition → present  │        │
│         └────────▲───────────────────────────────┬────────┘        │
│                  │ frame due at next refresh     │ SDL3            │
│         ┌────────┴─────────┐                     │                 │
│         │  Frame slots     │              ┌──────▼────────┐        │
│         │  (bounded queue) │              │  KMSDRM / GPU │        │
│         └────────▲─────────┘              └───────────────┘        │
│                  │                                                 │
│  ── Ingest (one locked OS thread per source) ───────────────────   │
│    ┌─────────────┴──────────────────────────────────────────┐      │
│    │  demux (libavformat) → decode (libavcodec + hwaccel)   │      │
│    │  → hwdownload/detile or native I420 → publish         │      │
│    └────────────────────────────────────────────────────────┘      │
└────────────────────────────────────────────────────────────────────┘
```

## Package Layout

```
livestream-viewer/
├── cmd/
│   └── livestream-viewer/
│       ├── main.go              # LockOSThread, wire planes, run render loop
│       ├── embed.go             # //go:embed frontend-dist
│       └── frontend-dist/       # Vite build output (git-ignored)
├── frontend/                    # ── Vue 3 + Vuetify management UI ──
│   ├── src/
│   │   ├── components/
│   │   ├── composables/
│   │   ├── layouts/
│   │   ├── pages/               # File-based routes
│   │   ├── plugins/
│   │   ├── stores/              # Pinia stores
│   │   ├── styles/
│   │   └── utils/               # API client, ingress base, formatters
│   ├── index.html
│   ├── package.json
│   └── vite.config.js
├── internal/
│   ├── config/                  # Bootstrap (TOML/env/flags) + DB-backed config
│   ├── database/                # SQLite schema, migrations, repositories
│   ├── model/                   # Source, Screen, Tile, PlaylistItem, Tour, Transition
│   ├── rest/                    # Router, middleware, SPA handler
│   ├── events/                  # SSE hub (`display.state`)
│   ├── handler/                 # REST handlers
│   ├── source/                  # Source validation, probing, lifecycle
│   │   ├── resolver/            # URL resolution: yt-dlp, direct, file, rtsp
│   │   └── youtube/             # Cookie jar, PO token provider, bundled yt-dlp plugin
│   ├── ingest/                  # Demux, decode workers, reconnect, worker manager
│   │   └── capability/          # Platform probe: V4L2 devices, codecs, DRM nodes
│   ├── frame/                   # Frame type, plane layout, lock-free slots
│   ├── display/                 # ── Display engine ──
│   │   ├── engine.go            # Render loop
│   │   ├── strategy/            # Screen/tour validation and immutable snapshot
│   │   ├── texture/             # Per-source texture cache and upload
│   │   ├── layout/              # Layout catalogue and tile geometry solver
│   │   ├── transition/          # Transition catalogue and interpolators
│   │   ├── compositor/          # Scene assembly and draw ordering
│   │   └── output/              # SDL init, video driver selection, display modes
│   ├── schedule/                # Headless tour, dwell, playlist, and sequence timers
│   └── preview/                 # Throttled read-back, JPEG encode, MJPEG stream
├── api/
│   └── openapi.yaml             # OpenAPI 3.0 specification
├── configs/
│   └── livestream-viewer.toml   # Default bootstrap configuration
├── packaging/                   # nFPM .deb configs (amd64/arm64)
├── addon/                       # Home Assistant add-on
├── docs/                        # Project documentation
├── scripts/                     # build.sh, build-deb.sh, export-youtube-cookies.sh, …
└── go.mod
```

## Domain Model

Four entities carry the whole configuration. They are deliberately small and fully serialisable so that the entire display strategy can be exported, diffed, and restored.

### Source

A source is a producer of video frames, independent of where it is shown.

| Field | Notes |
|-------|-------|
| `id`, `name` | Identity |
| `kind` | `youtube` \| `rtsp` \| `hls` \| `dash` \| `http` \| `srt` \| `rtmp` \| `file` |
| `url` | Stream URL, or the stored path for `file` |
| `credentials` | Username/password for RTSP, stored encrypted at rest is out of scope; see security notes |
| `options` | Kind-specific: RTSP transport (`tcp`/`udp`), file loop, `buffer_ms` (jitter buffer; default 4000 for YouTube/HLS/DASH), `force_software` |
| `enabled` | Sources can be defined but held inactive |
| `probe` | Cached result: codec, resolution, frame rate, whether hardware decode is available |

Sources are referenced by ID from tiles and playlist items. Deleting a source that is in use is refused; the API returns the referencing screens.

### Screen

A screen is one complete composition of the output. `kind` selects between two shapes:

**`kind: grid`**

| Field | Notes |
|-------|-------|
| `layout` | Layout ID from the catalogue (`2x2`, `1+5`, `3v`, …) |
| `tiles[]` | One per layout cell: `{ index, source_id, fit }` |
| `fit` | `contain` (letterbox, preserves aspect), `cover` (crop to fill), `fill` (stretch) |

A tile may be left empty, and a tile may optionally carry its own **sequence**: an ordered list of sources that rotate within that single tile on their own dwell timer. This is the standard VMS "camera sequence" behaviour and is what makes a `1+5` hotspot layout useful.

**`kind: transition`**

| Field | Notes |
|-------|-------|
| `items[]` | Ordered `{ source_id, dwell_ms, fit }` |
| `transition` | Applied between consecutive items |
| `loop` | Whether the playlist wraps |

### Transition

| Field | Notes |
|-------|-------|
| `type` | `cut` \| `fade` \| `barWipe` \| `boxWipe` \| `barnDoorWipe` \| `pushWipe` \| `slideWipe` |
| `subtype` | Direction or variant, e.g. `leftToRight`, `crossfade`, `fadeToColor`, `clockwiseTwelve` |
| `duration_ms` | Zero for `cut` |
| `easing` | Named preset or explicit `cubic-bezier(x1,y1,x2,y2)` control points |
| `color` | Only for `fadeToColor`/`fadeFromColor` |

Names come from SMPTE 258M via the W3C SMIL transition module. See [Glossary](reference/glossary.md).

### Tour

The top-level display strategy: which screens play, in what order, and for how long.

| Field | Notes |
|-------|-------|
| `entries[]` | Ordered `{ screen_id, dwell_ms, transition }` |
| `loop` | Whether the tour wraps at the end |
| `enabled` | When false, the display holds on a single pinned screen |

A tour with a single entry and `loop: false` is a static wall — this is the expected starting configuration, and everything else is layered on top of it.

## Subsystem Design

### Display Engine

The engine owns the main OS thread and runs a fixed loop:

1. **Drain commands** — Non-blocking read from the command channel. Commands are things like "reload screen 3", "go to next screen", "pause the tour", "a source's decoder is now healthy". Commands mutate engine-local state only.
2. **Advance the schedule** — Tick dwell timers, decide whether a transition should start, and compute the current transition progress from wall-clock time.
3. **Sample frames** — For each visible source, take the frame its presentation clock says belongs on screen at the next refresh, discarding any older ones still queued. This is a non-blocking read; if nothing new is due, the previous texture is reused and no upload happens.
4. **Upload** — Push new frames into per-source streaming textures. Must happen on this thread.
5. **Composite** — Build the scene: resolve tile geometry from the layout, apply fit and alpha, draw into the transition's source and destination render targets when a transition is active.
6. **Present** — One `SDL_RenderPresent`, vsync-paced.
7. **Publish state** — Atomically swap a state snapshot that the control plane can read without locking.

The loop never blocks. Decoder stalls, network failures, and missing sources degrade to a held last frame or a placeholder card, never to a stalled display.

See [Display Pipeline](architecture/display-pipeline.md) and [Render Loop Pattern](patterns/render-loop-pattern.md).

### Layout Solver

Layouts are declarative: a layout is a list of normalised rectangles in the unit square, plus metadata (name, cell count, aspect preference). The solver maps those normalised rects onto the actual output resolution, applies a uniform gutter, and then fits each source's video rect inside its cell according to the tile's `fit` mode.

Keeping layouts as data rather than code means the catalogue is served to the frontend from `GET /api/v1/layouts` and the Preview page renders the same geometry the engine does, from the same numbers.

Catalogue: `full`, `2x1`, `1x2`, `2x2`, `3x3`, `4x4`, `1+3`, `1+5`, `1+7`, `1+12`, `3v`, `1v+6`, `2p`, `1p+6`. `full` is the only single-cell layout — an earlier `1x1` was geometrically identical and has been retired, with existing screens migrated to `full` on startup. See [Display Strategy Pattern](patterns/display-strategy-pattern.md).

### Transitions

A transition is a pure function of progress `t ∈ [0,1]` (after easing) that produces draw instructions for an outgoing and an incoming layer. Implementations fall into three families:

- **Alpha** (`fade`) — Alpha-modulate the incoming layer over the outgoing one, or through a solid colour.
- **Geometric** (`pushWipe`, `slideWipe`, `boxWipe`, `barWipe`) — Translate or clip source rectangles. Implemented entirely with `SDL_RenderTexture` destination rects and clip rects; no shader needed.
- **Masked** (`irisWipe`, `ellipseWipe`, `clockWipe`) — **Withdrawn, not implemented.** These need a per-pixel alpha mask, which means a fragment shader through `SDL_CreateGPURenderer`/`SDL_SetGPURenderState`. That work has not been done, and advertising them meant the catalogue offered three effects that silently rendered as a crossfade. They are no longer in the catalogue or accepted by validation; configurations saved before the withdrawal are migrated to `fade` on startup and render as a crossfade with `masked_wipe_fade` in the engine's degradation list until they are.

Every type and subtype the catalogue advertises reaches a distinct implementation, and that is enforced by tests rather than left to review.

Because a grid screen is itself just a layer, transitioning between two grid screens is the same operation as transitioning between two full-screen sources: composite each screen into a render target, then run the transition over the two targets.

### Ingest and Decode

One worker per needed source (every enabled source on any screen in the strategy), each on its own locked OS thread:

```
open input (libavformat)
  → find video stream, discard audio and subtitle streams
  → select hwaccel (see below)
  → loop: read packet
      → hardware sessions wait for a keyframe before SendPacket
      → send to decoder → receive frame
      → if frame is hardware-backed: hwdownload + detile to NV12
      → software 4:2:0 stays I420; other formats swscale to NV12
      → wait until the frame's PTS is due (YouTube/HLS/DASH/file only)
      → publish into the source's frame slot
```

**Frame slots** are a bounded single-producer/single-consumer presentation queue — six frames over a pool of eight buffers. The decoder packs into a reserved pool buffer and appends; the renderer takes what is due and discards the rest. The queue evicts its oldest entry when full rather than growing, because an unbounded queue would let the renderer fall behind permanently and turn a decode stall into standing latency. This is a viewer, and freshness beats completeness. The depth exists so the renderer can hold a frame that has arrived but is not yet due, which a newest-only handoff cannot express.

**The loop runs on the display clock; which frame it shows comes from the stream clock.** Each source has a presentation clock anchoring its PTS to wall time, and the renderer picks the frame due at the *next* refresh, using the refresh interval SDL reports for the current display mode. Showing whatever is newest instead — the original design — makes cadence depend on the drifting phase between two unrelated clocks: a 30 fps source holds for two refreshes, then three, then two, and loses a frame whenever two publishes land inside one refresh. That is invisible on a fixed camera and reads as judder on a moving one, which is why it surfaced on a YouTube livestream rather than on the RTSP wall. With media time driving the choice, 30 fps on 60 Hz is a clean 2:2 and uneven ratios stay on a fixed repeating pattern.

**Sources are not synchronised to each other, and should not be.** There is no common timebase between a YouTube stream and an RTSP camera, so each tile keeps its own clock and the display is master only for presentation. What they share is the render pass: sources are anchored a refresh apart, cycling every four, so a grid of 1080p tiles does not converge its uploads onto one iteration and overrun the refresh — which would hitch every tile at once. Steady state draws straight to the backbuffer; the offscreen targets are used only to blend a transition or feed the preview. A loop period beyond 1.5× the refresh counts as a dropped frame in display state. Tearing is structural rather than incidental: vsync flips at vblank and all uploads precede all draws, and a driver refusing vsync is reported as `vsync_unavailable`. See [Presentation timing](architecture/display-pipeline.md#presentation-timing).

HLS and YouTube must still not *produce* faster than media time: a whole segment arrives at once, and dumping it would simply overflow the queue. Those sources preroll by `options.buffer_ms` (default 4s) via FFmpeg `live_start_index`, and the worker waits on PTS **before** each publish. RTSP is not paced. `options.force_software` forces the software decoder.

**Failure handling** is per-source and never propagates. A worker that loses its input enters a reconnect loop with exponential backoff and jitter, capped at 30 seconds; the attempt counter resets after a session that produced frames. Because a permanently unreachable source increments that counter indefinitely, the doubling is bounded iteration rather than a shift — an overflow there produced a negative delay and a panic after about 25 minutes of continuous failure. The engine keeps showing the last good frame while that source is still needed.

YouTube sources re-run URL resolution before every reconnect, and additionally recycle a healthy session every 30 minutes, because their manifest URLs carry `expire` and `ip` parameters. Resolution itself is bounded: `yt-dlp` and `ffprobe` always run under a deadline; on Unix the subprocess is killed as a process group so Deno/Python children do not leak after the timeout. After five consecutive resolution failures the source trips a circuit breaker — it is reported as `failed` and retried at the backoff ceiling instead of hammering the subprocess. Extractor errors are classified (`youtube_bot_check` when YouTube treats the host as a bot, `youtube_auth` when a signed-in session is required) so Preview and Stream Sources can show a sentence instead of a log dump. The PO token provider becoming ready, or uploading cookies, restarts those workers.

**A decode worker cannot take down the process.** Panics in a decode session — libav is cgo, drivers are third-party, and camera bitstreams are hostile input — are recovered and turned into ordinary session errors that the reconnect loop handles. The same containment applies to the ingest manager loop, the preview encoder, and the render loop.

See [Stream Ingest](architecture/stream-ingest.md).

### Hardware Decode

Decode capacity is host-specific. The application probes what is available at startup and surfaces it through `GET /api/v1/system/info` and in the UI, so a dense grid of software-only streams is an informed choice rather than a silent failure.

Illustrative Linux SBC matrix (see the dedicated doc for full detail):

| Platform | H.264 | HEVC | Interface |
|----------|-------|------|-----------|
| Raspberry Pi 4 | Hardware | Hardware | Stateful V4L2 M2M for H.264; stateless V4L2 request for HEVC |
| Raspberry Pi 5 | **Software only** | Hardware (4K60) | Stateless V4L2 request only; H.264 block removed from BCM2712 |
| Desktop Linux / macOS | Depends | Depends | VA-API, VideoToolbox, or software |

On constrained hosts the binding limit is usually decode, not compositing. **The baseline design copies frames to system memory** (`hwdownload` to NV12, or a packed I420 copy for software 4:2:0) and uploads them with `SDL_UpdateNVTexture` / `SDL_UpdateYUVTexture`. Zero-copy paths (DMA-BUF import into GL textures) are platform-specific later optimisations behind a stable frame-delivery interface.

On macOS, VideoToolbox is the hardware path. It cannot start mid-GOP: joining live RTSP without waiting for an IDR used to spam `hardware accelerator failed to decode picture`. Hardware sessions wait for a keyframe, do not set `AV_CODEC_FLAG_LOW_DELAY`, and pass `hwaccel_flags=+allow_profile_mismatch+ignore_level`. RTSP still stays on software unless a probe actually produced a hardware frame.

See [Hardware Decode](architecture/hardware-decode.md) for platform notes (including Raspberry Pi SAND / V4L2 details, VideoToolbox, and the zero-copy analysis).

### Source Resolution

Some sources need work before libavformat can open them:

- **YouTube** — `yt-dlp -g -f "bv*[protocol*=m3u8]/b[protocol*=m3u8]/bv*/b"` yields a single HLS video URL. Live streams only publish separate video/audio playlists, so a muxed `best` selector fails. A BgUtils PO token provider (HTTP server on `127.0.0.1:4416` by default) supplies proof-of-origin tokens. `auto` uses a managed Deno server when Deno and `pot_server_dir` exist; otherwise it stays on the default URL and keeps pinging until a sidecar answers. The process copies an **embedded** bgutil plugin to `data/yt-dlp-plugins/bgutil/` (yt-dlp only loads `plugin-dirs/*/yt_dlp_plugins`) and passes `--plugin-dirs`; without the plugin the HTTP server is ignored. When `data/secrets/youtube.cookies` exists, `--cookies` is passed; otherwise optional `cookies_from_browser` becomes `--cookies-from-browser`. Cookies are required when the bot check still fires after a PO token, and for private, members-only, or age-gated videos. On a Mac, `./scripts/export-youtube-cookies.sh` dumps a jar from Chrome/Safari/Firefox (Chrome may prompt for the keychain). An optional manual PO token becomes `--extractor-args youtube:po_token=…` and wins over the provider. The URL carries `expire` and `ip` parameters and must be re-resolved periodically and on every reconnect. `yt-dlp` is discovered on `PATH` and is never compiled into the Go binary. The Home Assistant add-on and the Linux `.deb` ship Deno and the token provider; `auto` starts the managed server. Closing the SDL window cancels the process (HTTP, ingest, and provider).
- **RTSP** — Passed through to libavformat, which handles negotiation and depacketisation. Transport is forced to TCP by default; interleaved TCP is far more reliable than UDP on a congested network and the latency cost is small.
- **HLS / DASH / TS / SRT / RTMP** — Handed directly to libavformat.
- **File** — Uploaded through the API to a data directory, played on loop.

### Preview

The Preview page shows what is actually on the wall, which means reading pixels back from the composited output. Read-back is expensive, so it is heavily constrained:

- Only runs when at least one client is subscribed.
- Throttled to a configurable low rate (default 2 fps).
- Downscaled on the GPU to a small render target (default 640px wide) before `SDL_RenderReadPixels`, so the read-back is of the small target, not the full frame.
- JPEG encoding happens off the render thread; the render thread only hands off a buffer.

Served as `GET /api/v1/preview/stream` (multipart MJPEG) and `GET /api/v1/preview/frame` (single JPEG). Closing the display window (or SIGINT) cancels the HTTP server's base context and closes the preview mailbox so those long-lived handlers return before `Shutdown` waits out its deadline. If the display engine is not running — headless development, or a display initialisation failure — the frontend falls back to rendering the layout geometry client-side from `GET /api/v1/layouts` plus per-source thumbnails, so the Settings pages remain fully usable.

Structured display state (active screen, tile assignments, per-source decoder health, frame rate, dropped frame counts) is pushed separately over Server-Sent Events at `GET /api/v1/events`.

### Database

SQLite via GORM, opened in WAL mode with a 5-second busy timeout and a single connection. The dataset is tiny, but writes arrive concurrently from API handlers, probe results, and the scheduler; with the default rollback journal those collide as "database is locked" errors. One writer connection removes the contention outright and WAL keeps readers from queueing behind it.

The engine never queries the database. The ingest manager is driven from the render loop, so it only re-reads source rows when the needed set actually changes — otherwise a four-camera wall issued a few hundred SQLite queries per second forever. Configuration edits call `Resync`, which forces a pass even when the set is unchanged.

The schema is small:

| Table | Description |
|-------|-------------|
| `config` | Runtime configuration, single row |
| `sources` | Stream source definitions and cached probe results |
| `screens` | Screen definitions (kind, layout) |
| `screen_tiles` | Grid tiles: screen, cell index, source, fit |
| `screen_items` | Transition-screen playlist items: screen, position, source, dwell |
| `tile_sequences` | Per-tile source rotation within a grid cell |
| `tour_entries` | Ordered screens with dwell time and transition |
| `uploads` | Uploaded media file metadata |
| `users` | Management UI accounts (bcrypt hashes, roles `admin` and `user`) |

Configuration changes are written to the database by the API handler, which then sends a reload command to the engine. The engine never reads the database directly — it is handed fully-resolved, immutable configuration snapshots.

### REST API

Served on port `8099` by default (`LSV_HTTP_PORT`), alongside the embedded SPA.

| Endpoint | Methods | Description |
|----------|---------|-------------|
| `/health` | GET | Liveness and readiness (see [Health and resource guards](#health-and-resource-guards)) |
| `/api/v1/auth/status` | GET | Unauthenticated: whether first-run setup is required |
| `/api/v1/auth/login` | POST | JWT login |
| `/api/v1/auth/me` | GET | Current user |
| `/api/v1/auth/change-password` | POST | Change password (required when `must_change_password` is set) |
| `/api/v1/users` | GET, POST | Admin-only user list and create |
| `/api/v1/users/:id` | PATCH, DELETE | Admin-only role update and delete |
| `/api/v1/system/info` | GET | Platform, decode capabilities, detected displays, version |
| `/api/v1/sources` | GET, POST | Source list and create |
| `/api/v1/sources/:id` | GET, PATCH, DELETE | Single source |
| `/api/v1/sources/:id/probe` | POST | Re-probe: codec, resolution, hardware decode availability |
| `/api/v1/sources/:id/thumbnail` | GET | Cached still for the UI |
| `/api/v1/uploads` | GET, POST | Uploaded media files (multipart) |
| `/api/v1/uploads/:id` | DELETE | Remove an uploaded file |
| `/api/v1/screens` | GET, POST | Screen list and create |
| `/api/v1/screens/:id` | GET, PATCH, DELETE | Single screen, including tiles and playlist items |
| `/api/v1/tour` | GET, PUT | The screen tour: ordering, dwell times, transitions |
| `/api/v1/display/state` | GET | Current screen, tile status, fps, decoder health |
| `/api/v1/display/next` | POST | Advance the tour |
| `/api/v1/display/previous` | POST | Step back |
| `/api/v1/display/goto/:screenId` | POST | Jump to a screen and pin it |
| `/api/v1/display/pause`, `/resume` | POST | Hold or release the tour |
| `/api/v1/layouts` | GET | Layout catalogue with normalised cell geometry |
| `/api/v1/transitions` | GET | Transition catalogue with valid subtypes |
| `/api/v1/preview/stream` | GET | MJPEG of the composited output |
| `/api/v1/preview/frame` | GET | Single JPEG snapshot |
| `/api/v1/events` | GET | Server-Sent Events: display and decoder state |
| `/api/v1/config` | GET, PATCH | Runtime configuration (PATCH is admin-only and sparse) |
| `/docs` | GET | Swagger UI |
| `/api/v1/openapi.yaml` | GET | OpenAPI 3.0 specification |
| `/` | GET | Web management UI (SPA) |

`api/openapi.yaml` is hand-maintained and is the contract. Any handler change must be reflected there in the same commit.

### Health and Resource Guards

This runs unattended for weeks on hardware with a few hundred megabytes of headroom, so the control plane is bounded rather than trusting.

`/health` distinguishes two failures because they call for different responses. An unreachable database means nothing works and a restart is the right answer, so it returns `503` with `status: unhealthy`. A failed display engine does **not** return an error status: the whole point of keeping the API up when SDL cannot initialise is that the operator can read `display_error` and fix the host, and a `503` would make a service manager restart the process in a loop instead. That case returns `200` with `status: degraded` and `ready: false`. The database check is a real ping with a two-second deadline, not a constant.

| Guard | Limit | Rejection |
|-------|-------|-----------|
| Login attempts, per client address | 5 in a burst, then 1 per 20s | `429 rate_limited` with `Retry-After` |
| Concurrent source probes | 2 | `429 probe_busy` |
| MJPEG preview viewers | 4 | `429 too_many_clients` |
| SSE clients | 16 | `429 too_many_clients` |
| Request body, non-multipart | 1 MiB | `400`/`413` |
| Upload body | 512 MiB, 8 MiB held in memory | `413 too_large` |

Login is the only unauthenticated write endpoint and each attempt costs a bcrypt comparison, so it is throttled per address; the bucket map is swept so it cannot accumulate an entry per address seen. Probing opens the stream and runs a decode test, costing about as much as a live tile, so concurrent probes are rejected rather than queued — the caller learns immediately that the host is busy instead of holding a connection open. The streaming endpoints each hold a connection and a goroutine for as long as the client stays, so both are capped.

Probe failures are persisted on the source row and returned over the API. RTSP credentials are injected into the open URL and libav diagnostics sometimes echo that URL, so userinfo is stripped and the message is truncated to 512 characters before it leaves the ingest package.

Every long-lived goroutine — decode workers, the ingest manager, the preview encoder, the render loop — recovers from panics. A panic in cgo-adjacent code would otherwise take down the whole process, including the API needed to diagnose it.

The HTTP server sets `ReadHeaderTimeout`, `ReadTimeout`, `IdleTimeout`, and `MaxHeaderBytes`. It deliberately sets no `WriteTimeout`, because that would cut off the MJPEG and SSE streams mid-flight; the upload handler raises its own read deadline instead, since half a gigabyte over a slow link legitimately takes minutes.

### Web Management Frontend

Vue 3 with the Composition API, Vuetify 3, Vite, file-based routing via `unplugin-vue-router`, and Pinia for state. Embedded into the binary with `//go:embed`, following the same pattern as go-mumble-server: Vite builds to `frontend/dist/`, the build script copies it to `cmd/livestream-viewer/frontend-dist/`, and the binary is compiled with the `embed_frontend` tag.

Navigation is deliberately shallow:

```
Preview
Settings
 ├── Stream Sources
 ├── Display Strategy
 └── Users (admin)
```

The Display Strategy page is the substantial one. It lists screens, lets you create a grid screen (pick a layout, then assign a source to each cell with a visual layout picker) or a transition screen (build an ordered playlist with per-item dwell times and a transition), and then lets you order those screens into the tour.

The SPA is served with SPA-aware fallback: requests for static assets that do not exist return 404; every other unmatched path serves `index.html`. Under Home Assistant ingress the API base and router base are derived from the ingress path prefix, and the Go server injects a `<base href>` when serving `index.html`.

See [Frontend Guide](patterns/frontend-guide.md) and [UI Style Guidelines](patterns/ui-style-guidelines.md).

### Configuration

Two-tier, matching go-mumble-server.

**Tier 1 — Bootstrap** (TOML file, environment variables with the `LSV_` prefix, command-line flags; precedence flags > env > TOML > defaults). These are the settings needed before the database is open, or that select the runtime environment:

| Setting | Description |
|---------|-------------|
| `database.path` | SQLite file path |
| `data.dir` | Directory for uploads and thumbnails |
| `http.port` | Management UI and API port (default 8099) |
| `http.bind` | Bind address |
| `display.enabled` | Start the display engine at all (false for API-only/development) |
| `display.driver` | SDL video driver override (`kmsdrm`, `wayland`, `x11`, `cocoa`) |
| `display.device` | Pin a KMSDRM card index. Omit it (or set `-1`) to let SDL scan for the card with a connected panel; pinning skips that scan |
| `logging.level` | `debug`, `info`, `warn`, `error` |
| `youtube.pot_mode` | PO token provider: `auto` (default; managed, else a sidecar answering `/ping`, else off), `managed`, `external`, `off` |
| `youtube.pot_url` | External provider URL |
| `youtube.pot_port` | Managed listen port (default 4416) |
| `youtube.pot_server_dir` | Managed server directory (add-on: `/opt/bgutil-pot`) |
| `youtube.cookies_from_browser` | Optional yt-dlp `--cookies-from-browser` value (ignored when a cookie jar is uploaded) |
| `frontend-embed` | Flag only; serve the embedded SPA |

**Tier 2 — Database** (editable via `GET/PATCH /api/v1/config`; no settings page exposes these yet): output resolution, default transition and dwell time, decoder preferences (allow software fallback, max concurrent hardware decoders), reconnect backoff, preview frame rate and width, the grid gutter, and the placeholder card appearance.

Every numeric field is range-checked on PATCH and an out-of-range value is rejected with `400 invalid_config` rather than clamped, because these feed the render loop directly: `preview_width` sizes a GPU render target, `preview_fps` gates a full read-back, and `gutter_px` can consume an entire cell. `output_width`/`output_height` size the window at startup and drive the Preview aspect ratio; changing them does not resize a running window. Output rotation is not implemented and the knob has been removed rather than left as a setting that does nothing.

TOML values seed the database on first run only. Subsequent changes go through the API.

## Design Decisions

| Decision | Rationale |
|----------|-----------|
| Go for both planes | One binary, one deployment. The concurrency model fits the decode fan-in naturally, and the control plane is ordinary Go. |
| SDL3 over raw EGL/GLES | SDL3 handles KMSDRM setup, mode selection, input, and GL context creation. Writing that by hand buys nothing until zero-copy becomes necessary, and SDL's own `testffmpeg.c` shows the escape hatch when it does. |
| SDL_Renderer, not SDL_GPU | SDL_GPU on Linux is Vulkan-only, and the Pi's V3DV driver cannot import the decoder's SAND format modifiers. SDL 3.4's `SDL_CreateGPURenderer` gives shader access from the 2D renderer where needed, without committing to Vulkan. |
| `Zyko0/go-sdl3` binding | MIT, actively maintained, idiomatic Go error handling, and covers the needed surface (`UpdateNVTexture`, render targets, blend modes, alpha mod). `veandco/go-sdl2` will never support SDL3. Load a system SDL3 built with KMSDRM rather than the binding's embedded library blob. |
| Purego vs cgo is not a criterion for the SDL binding | The binary is cgo regardless because of astiav, so pick the SDL binding on API quality and maintenance instead. |
| `go-astiav` in-process, not `ffmpeg` subprocess | Raw NV12 at 1080p30 is roughly 93 MB/s per stream through a pipe. A 3×3 wall would spend more memory bandwidth on copying than on decoding. In-process also keeps the door open for zero-copy. |
| `yt-dlp` as an external subprocess | It releases constantly and is the only thing that reliably works. Never compiled into the Go binary; discovered on `PATH`. The process copies an **embedded** bgutil plugin into `data/yt-dlp-plugins/bgutil/` so Homebrew yt-dlp can talk to the PO token HTTP server. The Home Assistant add-on and the Linux `.deb` install Deno and the BgUtils provider and run it under `auto`. Standalone hosts without those files still wait for a sidecar on `:4416`. YouTube degrades cleanly when yt-dlp is absent. |
| Video only, no audio | Removes A/V synchronisation entirely. The renderer can be display-clock driven and drop stale frames freely, which is exactly right for a wall. |
| Newest-frame-wins, no frame queue | **Superseded.** Each source now has a bounded presentation queue (depth 6, pool of 8) and a presentation clock so the renderer can hold a frame that has arrived but is not due. Freshness still beats completeness: a full queue evicts the oldest frame. |
| Layouts as data, not code | The same normalised geometry drives the engine, the API, and the Preview page's client-side fallback. Adding a layout is a data change. |
| Transitions as pure functions of progress | Testable without a GPU, and composable — a grid screen and a full-screen source are both just layers. |
| Engine never reads the database | The render thread must not touch I/O. The control plane resolves configuration and hands the engine immutable snapshots over a channel. |
| Baseline copies frames to system memory | Zero-copy needs an out-of-tree FFmpeg fork and SAND detiling, and is unreachable through SDL's public API. Ship the working path first, behind an interface that allows the fast path later. |
| SQLite for persistence | Zero-config, embedded, and the dataset is tiny. |
| Two-tier config (TOML + SQLite) | TOML for what is needed before the database opens; SQLite for everything the user should be able to change from the UI without a restart. |
| Embedded frontend via `//go:embed` | Single binary. No separate web server, no static file hosting. |
| Standard SMPTE/SMIL transition names | Free, well-defined vocabulary that is instantly legible to anyone from a video background, with an existing type/subtype structure that maps cleanly onto configuration. |
| CCTV/VMS layout vocabulary | `2x2`, `1+5`, hotspot, dwell, tour, sequence — these terms already mean something specific to the target users. |
| Swagger at `/docs` | Matches go-mumble-server; self-documenting API and standard client generation. |

## Documentation Index

### Core

- [Product Overview](product-overview.md) — Vision, scope, and feature summary
- **Technical Overview** — This document
- [Build and Test](build-and-test.md) — Toolchain, build, and test process

### Architecture

- [Display Pipeline](architecture/display-pipeline.md) — SDL3, KMS/DRM, compositing, and presentation
- [Stream Ingest](architecture/stream-ingest.md) — Demux, decode, frame handoff, and failure handling
- [Hardware Decode](architecture/hardware-decode.md) — Platform decode capabilities and the zero-copy analysis

### Patterns

- [Display Strategy Pattern](patterns/display-strategy-pattern.md) — Screens, layouts, tiles, and tours
- [Render Loop Pattern](patterns/render-loop-pattern.md) — Main-thread ownership and loop structure
- [Concurrent State Pattern](patterns/concurrent-state-pattern.md) — Command channel, state snapshots, frame slots
- [Frontend Guide](patterns/frontend-guide.md) — Vue and Vuetify patterns
- [UI Style Guidelines](patterns/ui-style-guidelines.md) — Layout, spacing, and component conventions

### Reference

- [Glossary](reference/glossary.md) — Layout, transition, and scheduling terminology
- [Features](features/) — Numbered feature plans

## External References

| Resource | URL |
|----------|-----|
| SDL3 wiki | https://wiki.libsdl.org/SDL3/ |
| SDL `testffmpeg.c` (reference video pipeline) | https://github.com/libsdl-org/SDL/blob/main/test/testffmpeg.c |
| `Zyko0/go-sdl3` | https://github.com/Zyko0/go-sdl3 |
| `jupiterrider/purego-sdl3` (alternative binding) | https://github.com/JupiterRider/purego-sdl3 |
| `asticode/go-astiav` | https://github.com/asticode/go-astiav |
| `jc-kynesim/rpi-ffmpeg` (zero-copy fork) | https://github.com/jc-kynesim/rpi-ffmpeg |
| mpv V4L2 DRM PRIME notes | https://github.com/mpv-player/mpv/wiki/V4L2-drmprime-support |
| W3C SMIL 3.0 Transition Effects | https://www.w3.org/TR/smil/smil-transitions.html |
| Home Assistant add-on configuration | https://developers.home-assistant.io/docs/add-ons/configuration |
