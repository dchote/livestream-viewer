# livestream-viewer

A native, hardware-accelerated livestream viewer and video wall. Renders one or more live streams directly to an attached display using SDL3 and the host's video decoder — no browser, no desktop environment, no compositor in the path.

Configured entirely from an embedded Vue 3 + Vuetify web interface. Runs as a standalone service or as a Home Assistant add-on.

> **Status: Control plane implemented.** Sources and uploads, the display strategy (screens, layouts, tiles, playlists, transitions, tour), the headless scheduler, SSE state, auth and user management, the embedded Vue management UI, Swagger, and the Home Assistant add-on files can all be built and run today — including headless on any Go-supported platform with `-display=false`. The display engine and stream decode are not implemented yet. See [docs/features/0003-stream-sources-and-display-strategy.md](docs/features/0003-stream-sources-and-display-strategy.md) and the [roadmap](docs/features/0001-project-scaffold.md).

## Supported platforms

| Role | Platforms |
|------|-----------|
| **Control plane** (API, UI, scheduler) | Linux, macOS, Windows — any Go target. Use `-display=false` when there is no attached panel to drive. |
| **Display output** (when the engine lands) | Linux with DRM/KMS (headless or windowed), plus a native SDL window on desktop Linux and macOS for development. |
| **Optimised for** | Low-cost and embedded boards — Raspberry Pi 4/5, similar ARM SBCs, and other constrained hosts — with hardware decode when the platform provides it, honest software-fallback reporting when it does not. |

The project is **platform-agnostic by design**. Raspberry Pi and other embedded targets are first-class optimisation targets (capacity planning, V4L2/DRM paths, packaging), not a hard dependency.

## Quick start (development)

```bash
./scripts/build.sh
./build/livestream-viewer -display=false
```

Open `http://127.0.0.1:8099` (first-run login `admin` / `admin`). Swagger is at `/docs`. Stop with Ctrl+C (a second Ctrl+C forces exit if shutdown stalls).

For hot reload, run the server with `-frontend-embed=false` and `cd frontend && yarn dev`.

## Overview

**Display engine** — Decodes streams with hardware acceleration where the host provides it, uploads frames to GPU textures, and composites them with SDL3. On Linux it can own the panel via KMS/DRM with no desktop session; on a workstation it opens a normal window for development.

**Management UI and REST API on :8099** — Vue 3 + Vuetify frontend embedded in the binary, with Swagger docs at `/docs`.

**Single Go binary** — One process, one deployment. Runs as a systemd (or equivalent) service, or as a Home Assistant add-on with the UI behind ingress.

## Features

### Stream Sources

- **YouTube live** — Resolved to an HLS manifest via `yt-dlp` (installed by the user, discovered on `PATH`) and refreshed as the manifest expires
- **RTSP** — IP cameras and NVRs, TCP or UDP transport, with credentials
- **HLS, DASH, MPEG-TS, SRT, RTMP** — Anything libavformat can open
- **Uploaded files** — Played on loop, for idle cards and offline fallbacks
- **Probing** — Every source reports its codec, resolution, frame rate, and whether it will decode on hardware or in software

### Display Strategies

- **Grid screens** — Multiple sources at once. Layouts follow CCTV/VMS convention: a single full-bleed source (`full`), equal grids (`2x2`, `3x3`, `4x4`), hotspot layouts (`1+3`, `1+5`, `1+7`, `1+12`), and vertical and panoramic variants. Per-tile fit mode, and optional source sequencing within a single tile.
- **Transition screens** — One full-screen source at a time, stepping through a playlist with a configured transition and per-item dwell time.
- **Screen tours** — Step through multiple screens on a timer, with a transition between each.
- **Transitions** — `cut`, `fade` (crossfade or through a colour), and the SMPTE 258M wipe family: bar, box, barn door, iris, ellipse, clock, push, and slide. Duration and CSS-compatible cubic Bézier easing on every one.

### Management Interface

Two primary navigation items:

- **Preview** — Live layout and tile state from the scheduler (composited MJPEG read-back when the display engine lands), plus per-tile decoder health and manual tour controls
- **Settings** — **Stream Sources**, **Display Strategy**, and for administrators **Users**

## Requirements

### Runtime (control plane today)

- Go-supported OS (Linux, macOS, Windows)
- Optional on `PATH`: `yt-dlp` (YouTube sources), `ffprobe` / `ffmpeg` (probe and thumbnails)

### Runtime (display engine — not linked yet)

- Linux with DRM/KMS for headless panel output, or desktop Linux/macOS for a windowed SDL surface
- SDL3 3.4+ (built with KMSDRM on headless Linux targets)
- FFmpeg 8.x shared libraries

### Build

- Go 1.25+ with CGO enabled
- Node.js 20+ and Yarn, for the frontend
- FFmpeg 8.x development libraries, SDL3 development files, and `pkg-config` — only when linking the display/decode stages

See [docs/build-and-test.md](docs/build-and-test.md) for the full dependency and build guide.

## Hardware decode and constrained hosts

On low-cost and embedded platforms, decode capacity is often the binding constraint. The application probes what the host can actually do and reports it per source, so a dense grid of H.264 cameras tells you what you are asking for rather than silently dropping frames.

Platform-specific notes (for example Raspberry Pi 4 vs Pi 5 V4L2 capabilities) live in [docs/architecture/hardware-decode.md](docs/architecture/hardware-decode.md). Those details inform capacity planning and packaging; they are not a requirement to run the control plane.

## Documentation

### Core

- [Product Overview](docs/product-overview.md) — Vision, scope, and features
- [Technical Overview](docs/technical-overview.md) — Architecture, subsystems, and design decisions
- [Build and Test](docs/build-and-test.md) — Toolchain, build, and test process

### Architecture

- [Display Pipeline](docs/architecture/display-pipeline.md) — SDL3, KMS/DRM, compositing, and presentation
- [Stream Ingest](docs/architecture/stream-ingest.md) — Demux, decode, frame handoff, and failure handling
- [Hardware Decode](docs/architecture/hardware-decode.md) — Platform decode capabilities and the zero-copy analysis

### Patterns

- [Display Strategy](docs/patterns/display-strategy-pattern.md) — Screens, layouts, tiles, and tours
- [Render Loop](docs/patterns/render-loop-pattern.md) — Main-thread ownership and loop structure
- [Concurrent State](docs/patterns/concurrent-state-pattern.md) — Command channel, state snapshots, frame slots
- [Frontend Guide](docs/patterns/frontend-guide.md) — Vue and Vuetify patterns
- [UI Style Guidelines](docs/patterns/ui-style-guidelines.md) — Layout, spacing, and component conventions

### Reference

- [Glossary](docs/reference/glossary.md) — Layout, transition, and scheduling terminology
- [Features](docs/features/) — Numbered feature plans

## License

MIT License — Copyright (c) 2026 Daniel Chote
