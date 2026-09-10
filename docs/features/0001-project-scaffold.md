# 0001: Project Scaffold and Implementation Roadmap

## Status: Implemented (stages 0–7; Pi capacity numbers remain qualitative)

Stage 0 and the control-plane / UI / add-on **skeletons** from Stages 5–7 are delivered by [0002](0002-initial-codebase-framework.md). Source and display-strategy editors, the headless scheduler, and SSE landed in [0003](0003-stream-sources-and-display-strategy.md). Ingest, lock-free frames, and the windowed SDL engine landed in [0004](0004-ingest-and-display-engine.md). KMSDRM-on-panel output is confirmed working on Raspberry Pi as a Home Assistant add-on. Pi capacity numbers remain qualitative.

## Summary

This document sequences the work from an empty repository to a working v0.1, ordered by dependency so that each stage produces something runnable and verifiable.

The guiding principle is to **prove the risky parts first**. Two things could invalidate the architecture, and both are cheap to test in isolation: whether SDL3 renders NV12 to a KMS/DRM output on a headless Linux panel at all, and whether hardware decode is reachable through go-astiav on representative hardware (including low-cost SBCs). Everything else is conventional web application work that carries no architectural risk. The project is platform-agnostic; Raspberry Pi and similar boards are optimisation and validation targets, not the only supported hosts.

## Stage 0 — Repository Skeleton

**Delivered by [0002](0002-initial-codebase-framework.md).**

No behaviour, just structure.

- `go.mod` for `github.com/dchote/livestream-viewer`, Go 1.25
- Directory layout as specified in [technical-overview.md](../technical-overview.md#package-layout)
- `.gitignore`, `Makefile`, `scripts/build.sh`
- `.github/workflows/ci.yml` — vet, test, build, with FFmpeg and SDL3 dev packages installed
- `internal/config` — bootstrap loading (TOML, `LSV_` env vars, flags) with the documented precedence
- Structured logging with `slog`

**Done when:** `go build ./...`, `go vet ./...`, and `go test ./...` all succeed and CI is green.

## Stage 1 — Display Spike

The highest-risk work, deliberately first, in a throwaway `cmd/spike-display`.

- Load system SDL3 via `Zyko0/go-sdl3`, verify version ≥ 3.4.0
- Initialise with `SDL_VIDEO_DRIVER=kmsdrm` on a headless Linux panel and natively on a desktop workstation
- Create an NV12 streaming texture, upload synthetic frames with `SDL_UpdateNVTexture`, present at vsync
- Draw four textures in a 2×2 arrangement
- Crossfade between two render targets

**Done when:** at least one headless Linux board (for example a Raspberry Pi 4 or 5) and one desktop workstation both display a 2×2 grid of moving synthetic video with a working crossfade. The Linux board must boot without a desktop session.

**If this fails**, the fallback is raw EGL/GLES with SDL providing only the window — a significant change that must be discovered now, not after the control plane is built.

## Stage 2 — Decode Spike

The second risk, also throwaway, in `cmd/spike-decode`.

- Open an RTSP and an HLS source with go-astiav
- Probe available hwaccels and V4L2 devices
- Decode HEVC and H.264 through the platform's hwaccel where available (for example `-hwaccel drm` / V4L2 M2M on Raspberry Pi), and through software elsewhere
- Transfer frames to system memory as NV12
- Measure per-frame cost and sustained frame rate for one, four, and nine concurrent 1080p streams on at least one constrained SBC and one desktop host

**Done when:** there are real numbers for how many streams each Pi model sustains, for both codecs, hardware and software. These numbers go into [hardware-decode.md](../architecture/hardware-decode.md) to replace the current qualitative capacity guidance, and into the UI's capacity warnings.

## Stage 3 — Frame Pipeline

Join the two spikes.

- `internal/frame` — `Frame` type, per-source pool, triple-buffered lock-free slot
- `internal/ingest` — decode worker on a locked OS thread, reconnect with backoff
- `internal/ingest/capability` — platform probe surfaced as a structured result
- `internal/display/texture` — per-source texture cache keyed by `(source, w, h, format)`, upload on the render thread

**Done when:** a hard-coded list of stream URLs renders live in a 2×2 grid on a constrained Linux host (for example a Raspberry Pi), survives unplugging and reconnecting a camera, and passes `go test -race`.

## Stage 4 — Display Engine

- `internal/display/layout` — layout catalogue as data, geometry solver, fit modes
- `internal/display/transition` — the full catalogue; geometric and alpha families first, masked family behind a capability check
- `internal/display/compositor` — scene assembly as data
- `internal/display/engine.go` — the loop from [render-loop-pattern.md](../patterns/render-loop-pattern.md), command channel, atomic state snapshot
- `internal/schedule` — tour, playlist, and tile-sequence timers on a wall clock

**Done when:** the engine renders a strategy snapshot handed to it in code, steps a tour with transitions, and the layout solver, transitions, scheduler, and compositor all have unit tests that need no GPU.

## Stage 5 — Persistence and API

**Skeleton delivered by [0002](0002-initial-codebase-framework.md)** (SQLite, OpenAPI, stub handlers, Swagger, auth). **Control-plane CRUD, probing, SSE, and the source resolver landed in [0003](0003-stream-sources-and-display-strategy.md); preview MJPEG landed in [0004](0004-ingest-and-display-engine.md).** This stage is complete.

- `internal/model`, `internal/database` — schema and migrations for the entities in [display-strategy-pattern.md](../patterns/display-strategy-pattern.md)
- `internal/source` — validation and probe; `internal/source/resolver` for yt-dlp, direct, and file (PATH tools, no libav link)
- `internal/rest`, `internal/handler` — sources, uploads, screens, tour, display commands, SSE hub, SPA handler
- `api/openapi.yaml` — hand-maintained, served at `/docs`
- `internal/schedule` — headless wall-clock scheduler (Stage 4 engine stand-in until SDL)
- `internal/preview` — throttled read-back, downscale, JPEG encode, MJPEG stream ([0004](0004-ingest-and-display-engine.md))
- Auth: bcrypt-hashed accounts, JWT

**Done when:** the entire display strategy can be built through the API and takes effect live, `/docs` is complete, and `openapi.yaml` matches every handler.

## Stage 6 — Frontend

**Shell delivered by [0002](0002-initial-codebase-framework.md)** (theme, nav, page layouts, common components, embed). **Source forms, display-strategy editors, SSE-driven Preview fallback, and `PasswordField` landed in [0003](0003-stream-sources-and-display-strategy.md); the PreviewCanvas MJPEG landed in [0004](0004-ingest-and-display-engine.md).** This stage is complete.

- Vite, Vue 3, Vuetify 3, Pinia, file-based routing, `importMode: 'sync'`
- Common components: StandardCard, StandardDialog, BackButton, PasswordField
- `useEventStream` and `useDisplayState` composables (SSE with reconnect)
- Preview page with client-side layout diagram and tile status until the engine runs
- Stream Sources page with per-kind forms, probe display, and upload
- Display Strategy page with LayoutPicker, TileEditor, PlaylistEditor, TransitionEditor, TourEditor
- `//go:embed` integration and the `embed_frontend` build tag

**Done when:** every capability of the API is reachable from the UI, and the UI is usable with `-display=false` for development.

## Stage 7 — Packaging

**Add-on scaffold, packaging, and Raspberry Pi panel output delivered.** DRM-on-panel output is confirmed working on Raspberry Pi Home Assistant OS.

- GoReleaser is **not** used for CGO cross builds. `.deb` packages are built with **nFPM** (`packaging/nfpm-*.yaml`, `scripts/package-deb.sh`) from binaries extracted out of the add-on image
- `scripts/build-deb.sh` builds the add-on image, smoke-checks YouTube/FFmpeg/SDL tooling, extracts the runtime, and packages `.deb`s
- systemd unit with `video`/`render` groups, `DeviceAllow=char-drm*`, and `/dev/dri` access (no `PrivateDevices`)
- `addon/` — Home Assistant add-on: `config.yaml`, `build.yaml`, `Dockerfile`, s6 `run`/`finish` scripts, `translations/en.yaml`, `README.md`, `CHANGELOG.md`
- `repository.yaml` at the repository root
- `.github/workflows/release.yml` — manual `workflow_dispatch` for GHCR images, `.deb`s, and GitHub Releases (stable tags created only after a successful build; rolling `dev` assets wiped each run)
- Ingress support: `getIngressBase()` in the frontend, `<base href>` injection in the Go SPA handler; `ingress_stream: true` for Preview MJPEG and SSE

**Done when:** the add-on installs from the repository URL on Home Assistant OS, the UI opens through ingress, and — when a panel is attached and DRM access is granted — the display lights up. Confirmed on Raspberry Pi Home Assistant OS: the add-on takes DRM master and drives the attached panel.

## Open Questions

These need answers from the spikes or from a decision before the stages that depend on them.

| Question | Blocks | Notes |
|----------|--------|-------|
| Does the Home Assistant add-on sandbox permit taking DRM master? | Stage 7 | **Yes, confirmed on Raspberry Pi.** `devices: /dev/dri` and `video: true` are sufficient. HA OS runs no desktop, so nothing else holds DRM master when the add-on is the display owner. |
| Does the Pi's SDL3 GPU renderer support custom fragment shaders? | Masked transitions in Stage 4 | Example of an embedded-Mesa question. If not, `irisWipe`, `ellipseWipe`, and `clockWipe` degrade to `fade` permanently rather than situationally. |
| How many concurrent decoder instances will constrained SBC hardware blocks accept? | Capacity limits in Stage 3 | Exceeding the limit fails confusingly; we need a hard cap and a clear error. Validate on Raspberry Pi and at least one other target. |
| Is the Raspberry Pi OS system FFmpeg patched with the V4L2-request hwaccels? | Stage 2, packaging | Pi-specific packaging question. If yes, we avoid shipping the `jc-kynesim` fork for that target. If no, the `.deb` and add-on image must carry it. |
| Should the add-on image bundle `yt-dlp`? | Stage 7 | **Yes, as PATH tools in the image.** The Go binary still discovers `yt-dlp` on `PATH` and does not embed it. The add-on image installs `yt-dlp[default]` (EJS), Deno, and the BgUtils PO token provider, and runs a non-fatal self-update on start. The Go binary embeds the bgutil yt-dlp plugin. Standalone `.deb` installs still expect host `yt-dlp` and ship Deno plus the provider server. |
| Output rotation: SDL or KMS? | Stage 4 | Portrait installations are common. SDL-level rotation costs a render target; KMS-level rotation may not be available on all outputs. |

## Non-Goals for v0.1

Restating from [product-overview.md](../product-overview.md#scope) so the roadmap is unambiguous: no audio output, no multiple simultaneous display outputs, no recording or analytics, no DRM-protected services, no fleet management.
