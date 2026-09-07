# Technical Overview

> **Status:** Framework scaffold in progress. The control plane, API shell, and embedded UI exist; treat display-engine and ingest sections as design specification until those stages land.

livestream-viewer is a single Go binary that decodes live video streams with hardware acceleration and composites them onto a physically attached display using SDL3, while serving a Vue 3 + Vuetify management UI and REST API. The primary target is the Raspberry Pi 4 and 5 running headless (no X11, no Wayland, no desktop session).

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.25+ |
| Rendering | SDL3 (3.4+) via `github.com/Zyko0/go-sdl3` |
| Video driver | KMSDRM on the Pi; native window on a development workstation |
| Demux and decode | FFmpeg 8.x `libav*` via `github.com/asticode/go-astiav` (cgo) |
| Hardware decode | V4L2 stateless (HEVC) and stateful M2M (H.264, Pi 4) via FFmpeg `drm` hwaccel |
| Stream URL resolution | `yt-dlp` invoked as an external subprocess (optional, discovered on `PATH`) |
| Database | SQLite (GORM) |
| REST API | Go standard library `net/http` with router |
| API docs | OpenAPI 3.0 served as Swagger UI at `/docs` |
| Frontend | Vue 3 (Composition API), Vuetify 3, Vite, Vue Router, Pinia |
| Frontend embedding | Go `//go:embed` — frontend dist compiled into the binary |
| Configuration | TOML bootstrap + SQLite runtime config, editable via REST API and UI |
| Logging | Structured logging (`slog`) |
| Packaging | Single binary, `.deb` via GoReleaser, Home Assistant add-on image |

## The Two Planes

The application is not a typical server. It has a hard structural split driven by a platform constraint: **SDL rendering and texture uploads must happen on the thread that initialised SDL**, and on the Pi that thread must hold DRM master for the whole lifetime of the process. Everything else — HTTP, database, stream resolution — is ordinary Go concurrency.

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
│                  │ newest frame per source       │ SDL3            │
│         ┌────────┴─────────┐                     │                 │
│         │  Frame slots     │              ┌──────▼────────┐        │
│         │  (triple buffer) │              │  KMSDRM / GPU │        │
│         └────────▲─────────┘              └───────────────┘        │
│                  │                                                 │
│  ── Ingest (one locked OS thread per source) ───────────────────   │
│    ┌─────────────┴──────────────────────────────────────────┐      │
│    │  demux (libavformat) → decode (libavcodec + hwaccel)   │      │
│    │  → hwdownload/detile → NV12 frame → publish            │      │
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
│   ├── rest/                    # Router, middleware, SPA handler, SSE hub
│   ├── handler/                 # REST handlers
│   ├── source/                  # Source registry, probing, lifecycle
│   │   └── resolver/            # URL resolution: yt-dlp, direct, file, rtsp
│   ├── ingest/                  # Demux + decode workers
│   │   ├── decoder/             # libav decode loop, hwaccel selection
│   │   └── capability/          # Platform probe: V4L2 devices, codecs, DRM nodes
│   ├── frame/                   # Frame type, plane layout, pooling, slots
│   ├── display/                 # ── Display engine ──
│   │   ├── engine.go            # Render loop, command handling, state publish
│   │   ├── texture/             # Per-source texture cache and upload
│   │   ├── layout/              # Layout catalogue and tile geometry solver
│   │   ├── transition/          # Transition catalogue and interpolators
│   │   ├── compositor/          # Scene assembly and draw ordering
│   │   └── output/              # SDL init, video driver selection, display modes
│   ├── schedule/                # Screen tour, dwell timers, playlist stepping
│   └── preview/                 # Throttled read-back, JPEG encode, MJPEG stream
├── api/
│   └── openapi.yaml             # OpenAPI 3.0 specification
├── configs/
│   └── livestream-viewer.toml   # Default bootstrap configuration
├── addon/                       # Home Assistant add-on
├── docs/                        # Project documentation
├── scripts/                     # build.sh, build-deb.sh
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
| `options` | Kind-specific: RTSP transport (`tcp`/`udp`), reconnect backoff, loop for files |
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
| `type` | `cut` \| `fade` \| `barWipe` \| `boxWipe` \| `barnDoorWipe` \| `irisWipe` \| `ellipseWipe` \| `clockWipe` \| `pushWipe` \| `slideWipe` |
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
3. **Sample frames** — For each visible source, take the newest published frame from its slot. This is a non-blocking read; if no new frame has arrived, the previous texture is reused.
4. **Upload** — Push new frames into per-source streaming textures. Must happen on this thread.
5. **Composite** — Build the scene: resolve tile geometry from the layout, apply fit and alpha, draw into the transition's source and destination render targets when a transition is active.
6. **Present** — One `SDL_RenderPresent`, vsync-paced.
7. **Publish state** — Atomically swap a state snapshot that the control plane can read without locking.

The loop never blocks. Decoder stalls, network failures, and missing sources degrade to a held last frame or a placeholder card, never to a stalled display.

See [Display Pipeline](architecture/display-pipeline.md) and [Render Loop Pattern](patterns/render-loop-pattern.md).

### Layout Solver

Layouts are declarative: a layout is a list of normalised rectangles in the unit square, plus metadata (name, cell count, aspect preference). The solver maps those normalised rects onto the actual output resolution, applies a uniform gutter, and then fits each source's video rect inside its cell according to the tile's `fit` mode.

Keeping layouts as data rather than code means the catalogue is served to the frontend from `GET /api/v1/layouts` and the Preview page renders the same geometry the engine does, from the same numbers.

Initial catalogue: `1x1`, `2x2`, `3x3`, `4x4`, `1+3`, `1+5`, `1+7`, `1+12`, `2x1`, `1x2`, `3v`, `1v+6`, `2p`, `1p+6`. See [Display Strategy Pattern](patterns/display-strategy-pattern.md).

### Transitions

A transition is a pure function of progress `t ∈ [0,1]` (after easing) that produces draw instructions for an outgoing and an incoming layer. Implementations fall into three families:

- **Alpha** (`fade`) — Alpha-modulate the incoming layer over the outgoing one, or through a solid colour.
- **Geometric** (`pushWipe`, `slideWipe`, `boxWipe`, `barWipe`) — Translate or clip source rectangles. Implemented entirely with `SDL_RenderTexture` destination rects and clip rects; no shader needed.
- **Masked** (`irisWipe`, `ellipseWipe`, `clockWipe`) — Require a generated alpha mask. On SDL 3.4+ these use a custom fragment shader via `SDL_CreateGPURenderer`/`SDL_SetGPURenderState`; on hardware where that is unavailable they degrade to `fade`.

Because a grid screen is itself just a layer, transitioning between two grid screens is the same operation as transitioning between two full-screen sources: composite each screen into a render target, then run the transition over the two targets.

### Ingest and Decode

One worker per active source, each on its own locked OS thread:

```
open input (libavformat)
  → find video stream, discard audio and subtitle streams
  → select hwaccel (see below)
  → loop: read packet → send to decoder → receive frame
      → if frame is hardware-backed: hwdownload + detile to NV12
      → publish into the source's frame slot
```

**Frame slots** are a triple-buffered, lock-free single-producer/single-consumer handoff. The decoder always writes to the free buffer and publishes it as "newest"; the renderer always takes the newest. There is no queue, because a queue would introduce latency and encourage the renderer to fall behind. Old frames are dropped, not buffered — this is a viewer, and freshness beats completeness.

**Presentation is display-clock driven, not stream-clock driven.** With no audio there is nothing to synchronise to, so the renderer simply shows whatever is newest at each vsync. Streams running at 25, 30, or 60 fps all present correctly on a 60 Hz output without resampling logic; a slow stream just repeats frames.

**Failure handling** is per-source and never propagates. A worker that loses its input enters a reconnect loop with exponential backoff and jitter. The engine keeps showing the last good frame for a configurable grace period, then swaps the tile to an offline placeholder. YouTube sources additionally re-run URL resolution before reconnecting, because their manifest URLs are time-limited and IP-bound.

See [Stream Ingest](architecture/stream-ingest.md).

### Hardware Decode

This is the part of the system most tied to the specific Pi model, and the design has to be explicit about it rather than assuming acceleration is available.

| Platform | H.264 | HEVC | Interface |
|----------|-------|------|-----------|
| Pi 4 | Hardware | Hardware | Stateful V4L2 M2M (`/dev/video10`) for H.264; stateless V4L2 request (`rpivid`, `/dev/video19`) for HEVC |
| Pi 5 | **Software only** | Hardware (4K60) | Stateless V4L2 request only; the H.264 block was removed from BCM2712 |
| Desktop dev | Depends | Depends | VA-API or software |

The Pi 5's removal of the H.264 decoder is the single most consequential fact for capacity planning. Most web and camera sources are H.264, so on a Pi 5 the common case is CPU decode across four Cortex-A76 cores. The application probes the platform at startup, records what is available per codec, and surfaces it through `GET /api/v1/system/info` and in the UI, so a user configuring a 3×3 grid of 1080p H.264 cameras on a Pi 5 is told what they are asking for.

Zero-copy (keeping frames as DMA-BUF handles and importing them into GL textures) is possible but requires the out-of-tree `jc-kynesim/rpi-ffmpeg` fork and handling Broadcom SAND tiled formats, and is not reachable through SDL's public API. **The baseline design copies frames to system memory** (`hwdownload` to NV12) and uploads them with `SDL_UpdateNVTexture`. The frame delivery interface is defined so that the upload step is swappable, making zero-copy a later optimisation behind a stable seam rather than a rewrite.

See [Hardware Decode](architecture/hardware-decode.md) for the full analysis, including the SAND format problem and why SDL_GPU is not viable on the Pi.

### Source Resolution

Some sources need work before libavformat can open them:

- **YouTube** — `yt-dlp -g -f "best[protocol*=m3u8]"` yields an HLS manifest URL. The URL carries `expire` and `ip` parameters and must be re-resolved periodically and on every reconnect. `yt-dlp` is discovered on `PATH`, never bundled: it releases frequently, and bundling a tool whose purpose is to work around a service's interface is not something this project ships. If it is absent, YouTube sources are marked unavailable with a clear message.
- **RTSP** — Passed through to libavformat, which handles negotiation and depacketisation. Transport is forced to TCP by default; interleaved TCP is far more reliable than UDP on a congested network and the latency cost is small.
- **HLS / DASH / TS / SRT / RTMP** — Handed directly to libavformat.
- **File** — Uploaded through the API to a data directory, played on loop.

### Preview

The Preview page shows what is actually on the wall, which means reading pixels back from the composited output. Read-back is expensive, so it is heavily constrained:

- Only runs when at least one client is subscribed.
- Throttled to a configurable low rate (default 2 fps).
- Downscaled on the GPU to a small render target (default 640px wide) before `SDL_RenderReadPixels`, so the read-back is of the small target, not the full frame.
- JPEG encoding happens off the render thread; the render thread only hands off a buffer.

Served as `GET /api/v1/preview/stream` (multipart MJPEG) and `GET /api/v1/preview/frame` (single JPEG). If the display engine is not running — headless development, or a display initialisation failure — the frontend falls back to rendering the layout geometry client-side from `GET /api/v1/layouts` plus per-source thumbnails, so the Settings pages remain fully usable.

Structured display state (active screen, tile assignments, per-source decoder health, frame rate, dropped frame counts) is pushed separately over Server-Sent Events at `GET /api/v1/events`.

### Database

SQLite via GORM. The schema is small:

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
| `/health` | GET | Liveness and readiness |
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
| `/api/v1/config` | GET, PATCH | Runtime configuration |
| `/docs` | GET | Swagger UI |
| `/api/v1/openapi.yaml` | GET | OpenAPI 3.0 specification |
| `/` | GET | Web management UI (SPA) |

`api/openapi.yaml` is hand-maintained and is the contract. Any handler change must be reflected there in the same commit.

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
| `display.device` | DRM device index when multiple are present |
| `logging.level` | `debug`, `info`, `warn`, `error` |
| `frontend-embed` | Flag only; serve the embedded SPA |

**Tier 2 — Database** (editable via `GET/PATCH /api/v1/config` and the UI): output resolution and rotation, default transition and dwell time, decoder preferences (allow software fallback, max concurrent hardware decoders), reconnect backoff, offline grace period, preview frame rate and width, and the placeholder card appearance.

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
| `yt-dlp` as an external subprocess | It releases constantly, it is the only thing that reliably works, and bundling it is neither practical nor appropriate. Discovered on `PATH`; YouTube support degrades cleanly when absent. |
| Video only, no audio | Removes A/V synchronisation entirely. The renderer can be display-clock driven and drop stale frames freely, which is exactly right for a wall. |
| Newest-frame-wins, no frame queue | Latency and freshness matter more than showing every frame. A queue would let the renderer fall behind and never catch up. |
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

- [Display Pipeline](architecture/display-pipeline.md) — SDL3, KMSDRM, compositing, and presentation
- [Stream Ingest](architecture/stream-ingest.md) — Demux, decode, frame handoff, and failure handling
- [Hardware Decode](architecture/hardware-decode.md) — Raspberry Pi decode capabilities and the zero-copy analysis

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
