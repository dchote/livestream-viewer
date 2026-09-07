# livestream-viewer

A native, hardware-accelerated livestream viewer and video wall for the Raspberry Pi. Renders one or more live streams directly to an attached display using SDL3 and the platform's video decoder — no browser, no desktop environment, no compositor.

Configured entirely from an embedded Vue 3 + Vuetify web interface. Installable as a Home Assistant add-on.

> **Status: Framework scaffold in progress.** The control plane, embedded management UI, Swagger, and Home Assistant add-on files can be built and run on macOS. Display engine and stream decode are not implemented yet. See [docs/features/0002-initial-codebase-framework.md](docs/features/0002-initial-codebase-framework.md) and the [roadmap](docs/features/0001-project-scaffold.md).

## Development (macOS)

```bash
./scripts/build.sh
./build/livestream-viewer -display=false
```

The management UI is at `http://127.0.0.1:8099` (first-run login `admin` / `admin`). Swagger is at `/docs`. For hot reload, run the server with `-frontend-embed=false` and `cd frontend && yarn dev`.

## Overview

**Display engine** — Decodes streams with hardware acceleration where the hardware provides it, uploads frames to GPU textures, and composites them with SDL3 straight to KMS/DRM. Owns the main thread and the display.

**Management UI and REST API on :8099** — Vue 3 + Vuetify frontend embedded in the binary, with Swagger docs at `/docs`.

**Single Go binary** — One process, one deployment. Runs as a systemd service on Raspberry Pi OS Lite, or as a Home Assistant add-on with the UI behind ingress.

## Features

### Stream Sources

- **YouTube live** — Resolved to an HLS manifest via `yt-dlp` (installed by the user, discovered on `PATH`) and refreshed as the manifest expires
- **RTSP** — IP cameras and NVRs, TCP or UDP transport, with credentials
- **HLS, DASH, MPEG-TS, SRT, RTMP** — Anything libavformat can open
- **Uploaded files** — Played on loop, for idle cards and offline fallbacks
- **Probing** — Every source reports its codec, resolution, frame rate, and whether it will decode on hardware or in software

### Display Strategies

- **Grid screens** — Multiple sources at once. Layouts follow CCTV/VMS convention: equal grids (`1x1`, `2x2`, `3x3`, `4x4`), hotspot layouts (`1+3`, `1+5`, `1+7`, `1+12`), and vertical and panoramic variants. Per-tile fit mode, and optional source sequencing within a single tile.
- **Transition screens** — One full-screen source at a time, stepping through a playlist with a configured transition and per-item dwell time.
- **Screen tours** — Step through multiple screens on a timer, with a transition between each.
- **Transitions** — `cut`, `fade` (crossfade or through a colour), and the SMPTE 258M wipe family: bar, box, barn door, iris, ellipse, clock, push, and slide. Duration and CSS-compatible cubic Bézier easing on every one.

### Management Interface

Two primary navigation items:

- **Preview** — A live, throttled read-back of the actual composited output, plus per-tile decoder health and manual tour controls
- **Settings** — **Stream Sources** and **Display Strategy**

## Requirements

### Runtime

- Raspberry Pi 4 or 5 (or any Linux system with DRM/KMS), or a desktop Linux/macOS machine for development
- SDL3 3.4+, built with KMSDRM on the Pi
- FFmpeg 8.x shared libraries
- `yt-dlp` on `PATH` — optional, required only for YouTube sources

### Build

- Go 1.25+ with CGO enabled
- FFmpeg 8.x development libraries, SDL3 development files, `pkg-config`
- Node.js 20+ and Yarn, for the frontend

See [docs/build-and-test.md](docs/build-and-test.md) for the full dependency and build guide.

## A Note on Raspberry Pi Hardware Decode

This matters enough to state up front: **the Raspberry Pi 5 has no H.264 hardware decoder.** The block was removed from BCM2712, leaving only HEVC. Since most cameras and web streams are H.264, the common case on a Pi 5 is software decode across the four Cortex-A76 cores.

The application probes what is actually available and reports it per source, so a nine-tile grid of 1080p H.264 cameras tells you what you are asking for rather than silently dropping frames. If you control the encoder, use HEVC — it has hardware decode on both the Pi 4 and the Pi 5.

The full analysis is in [docs/architecture/hardware-decode.md](docs/architecture/hardware-decode.md).

## Documentation

### Core

- [Product Overview](docs/product-overview.md) — Vision, scope, and features
- [Technical Overview](docs/technical-overview.md) — Architecture, subsystems, and design decisions
- [Build and Test](docs/build-and-test.md) — Toolchain, build, and test process

### Architecture

- [Display Pipeline](docs/architecture/display-pipeline.md) — SDL3, KMSDRM, compositing, and presentation
- [Stream Ingest](docs/architecture/stream-ingest.md) — Demux, decode, frame handoff, and failure handling
- [Hardware Decode](docs/architecture/hardware-decode.md) — Raspberry Pi decode capabilities and the zero-copy analysis

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
