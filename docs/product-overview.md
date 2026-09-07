# Product Overview

**livestream-viewer** is a dedicated, always-on video wall: it renders one or more live streams directly to an attached display using hardware-accelerated decoding and GPU compositing, and it is configured entirely from a web interface. There is no browser kiosk and no requirement for a desktop environment — when display output is enabled, the application owns the panel.

**Status: Control plane implemented** — Sources, uploads, display strategy, the headless scheduler, and the Vue editors can be built and run with `-display=false` on any Go-supported platform. Display engine and stream decode are not implemented yet.

The project is structured as two cooperating planes inside a single Go binary: a **display engine** that decodes and composites video onto the physical output, and a **control plane** (REST API plus embedded Vue 3 + Vuetify UI) that manages sources, layouts, and scheduling. Like [go-mumble-server](https://github.com/dchote/go-mumble-server), the repository doubles as a Home Assistant add-on repository for one-click install on Home Assistant OS.

## Platforms

livestream-viewer is **platform-agnostic**:

| | |
|--|--|
| **Control plane** | Linux, macOS, Windows — API, UI, persistence, and the headless scheduler |
| **Display output** | Linux DRM/KMS for headless panels; native SDL window on desktop Linux and macOS for development |
| **Optimised for** | Low-cost and embedded hosts (Raspberry Pi and similar SBCs) — hardware decode when available, honest software-fallback reporting when not |

Raspberry Pi and other constrained boards are important optimisation targets, not a hard dependency. You can develop and operate the management plane without one on the desk.

## Motivation

The usual way to put live video on an attached screen is to run a full desktop session and point a browser at a page. That approach is expensive and fragile: Chromium will not use the host's hardware decoder for most stream types, a compositor and window system sit between the video and the panel, and a multi-stream grid quickly saturates the CPU — especially on low-cost hardware. Digital signage products that take this route (Anthias and similar) are effectively playlist-driven browser kiosks and inherit all of the same costs.

Purpose-built alternatives exist but are closed source, licence-encumbered, or aimed at commercial signage fleets rather than a single self-hosted screen. **livestream-viewer** takes the narrower, more achievable goal:

- **Native rendering** — Decode with the platform's video hardware where available, upload frames to GPU textures, and composite with SDL3. No browser in the path. On headless Linux the application can render straight to KMS/DRM.
- **Purpose-built for multi-stream** — A grid of streams is the primary use case, not an afterthought. Layout, scaling, and transitions are first-class configuration.
- **Configured, not scripted** — Everything is managed through a web UI and a documented REST API. No config files to hand-edit on the device, no restart to change a layout.
- **Single binary, single process** — One Go binary that serves its own management UI and drives the display. Trivial to package, trivial to run as a service or a Home Assistant add-on.
- **Honest about hardware** — Decode capacity varies widely across hosts and generations. The application probes what is actually available and reports it, rather than silently falling back to software decode and dropping frames.

## Core Functionality

### Stream Sources

A **source** is anything that can produce video frames. The user adds sources in the management UI and the application probes each one to report its codec, resolution, and frame rate:

- **YouTube live streams** — Resolved to a playable HLS manifest at playback time and refreshed as the manifest expires. Requires the user to install `yt-dlp` on the host; it is discovered on `PATH` rather than bundled.
- **RTSP** — IP cameras and NVRs, over TCP or UDP interleaved transport, with optional credentials.
- **HLS / DASH / MPEG-TS over HTTP** — The common denominator for web-based live streaming. Any URL that libavformat can open.
- **SRT and RTMP** — For contribution feeds and legacy ingest endpoints.
- **Uploaded files** — Video files uploaded through the management UI and stored on the device, played on loop. Useful for idle cards, station idents, and offline fallbacks.

Sources are configured once and reused across any number of screens.

### Display Strategies

A **screen** is one complete composition of the output. Two kinds exist:

- **Grid** — Multiple sources visible simultaneously, one per tile. Layouts follow the conventions established by CCTV and video management systems: a single full-bleed source (`full`), equal grids (`2x2`, `3x3`, `4x4`), hotspot layouts with one large tile and several small ones (`1+5`, `1+7`), and vertical and panoramic variants for portrait or wide sources. Each tile independently controls which source it shows and how that source is fitted to the tile (letterboxed, cropped to fill, or stretched).
- **Transition** — A single source is full-screen at a time, and the screen steps through an ordered playlist of sources. Each item has its own dwell time, and moving between items plays a configured transition.

Multiple screens can be defined and arranged into a **tour**: the display steps from one screen to the next after each screen's dwell time elapses, with a transition between them. A tour can loop indefinitely or be driven manually through the API.

### Transitions

Transition names follow the SMPTE 258M wipe vocabulary as codified by the W3C SMIL transition module, so anyone with a broadcast or video-production background will recognise them immediately:

- **Cut** — Instantaneous switch. The default and the cheapest.
- **Fade** — Crossfade (both sources visible during the transition) or fade through a solid colour.
- **Wipe** — A moving boundary reveals the incoming source: bar, box, barn door, iris, ellipse, and clock variants.
- **Push and slide** — The incoming source displaces the outgoing one (push) or moves over a stationary outgoing one (slide).

Every transition has a duration and an easing curve. Easing is expressed as CSS-compatible cubic Bézier control points, with the familiar named presets (`ease-in`, `ease-out`, `ease-in-out`) as shorthand.

### Management Interface

The web UI has two primary navigation items:

- **Preview** — A live representation of what the physical display is currently showing, including which screen is active, which tile holds which source, and the health of each decoder. This is a monitoring view, not a second renderer: it shows a throttled, downscaled read-back of the actual composited output so that what you see is what is on the wall. Until the display engine lands, the page renders the scheduler's live layout geometry and tile assignments instead of video, driven by the same SSE state.
- **Settings** — **Stream Sources** (add, edit, probe, and upload), **Display Strategy** (build screens, arrange tiles or playlists, and order the tour), and for administrators **Users** (RBAC accounts for the management UI).

The UI is built with Vue 3 and Vuetify 3 and is embedded in the Go binary, so there is nothing separate to deploy.

### REST API

The web UI is a consumer of the REST API, which is equally available for direct integration — scripting a screen change from a Home Assistant automation, for example, or driving the wall from an external scheduler. Interactive API documentation is served at `/docs` via Swagger UI.

## Deployment

**Home Assistant add-on** — The repository can be added as a Home Assistant add-on repository for one-click install. The management UI is exposed through ingress; on hosts with an attached panel the add-on is granted access to the DRM render nodes so it can drive the display.

**Standalone** — A single binary, run as a systemd (or equivalent) service. On headless Linux no desktop environment is required; the application can take DRM master directly.

**Development** — Runs on a normal desktop Linux or macOS workstation. Use `-display=false` for control-plane work; when the engine lands, windowed SDL output supports layout and transition development without dedicated hardware on the desk.

## Scope

**In scope for the initial release:** video-only playback, grid and transition screens, a screen tour with configurable dwell times, the transition catalogue above, YouTube/RTSP/HLS/file sources, single display output, and the management UI and API.

**Explicitly out of scope for now:**

- **Audio output.** Streams are decoded video-only; audio tracks are discarded. This removes the need for A/V synchronisation entirely and simplifies the pipeline considerably. It may be revisited later.
- **Multiple simultaneous display outputs.** One display per instance.
- **Recording, motion detection, or analytics.** This is a viewer, not an NVR. Point it at an NVR's RTSP output instead.
- **DRM-protected commercial streaming services.** Widevine and equivalents are not supported and will not be.
- **Fleet management.** One device, one instance, one configuration.

## Target Users

- **Home Assistant users** who want a wall-mounted screen showing camera feeds, alongside or instead of a dashboard
- **Self-hosters** running a small camera setup who want a dedicated monitor without paying for a VMS client licence
- **Makers and hobbyists** building an information wall, workshop display, or ambient screen — including on low-cost SBCs
- **Small venues** — a bar, gym, or office reception showing a rotating set of live feeds on inexpensive hardware

## Related Documentation

- [Technical Overview](technical-overview.md) — Architecture, subsystems, and design decisions
- [Glossary](reference/glossary.md) — Layout, transition, and scheduling terminology
- [Build and Test](build-and-test.md) — Toolchain and build process

## License

MIT License — Copyright (c) 2026 Daniel Chote
