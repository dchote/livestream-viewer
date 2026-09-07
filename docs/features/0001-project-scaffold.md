# 0001: Project Scaffold and Implementation Roadmap

## Status: Planned

## Summary

The repository currently contains documentation and a licence. This document sequences the work from an empty repository to a working v0.1, ordered by dependency so that each stage produces something runnable and verifiable.

The guiding principle is to **prove the risky parts first**. Two things could invalidate the architecture, and both are cheap to test in isolation: whether SDL3 renders NV12 to a KMSDRM output on a Pi at all, and whether hardware decode is reachable through go-astiav on the target hardware. Everything else is conventional web application work that carries no architectural risk.

## Stage 0 — Repository Skeleton

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
- Initialise with `SDL_VIDEO_DRIVER=kmsdrm` on a Pi and natively on the workstation
- Create an NV12 streaming texture, upload synthetic frames with `SDL_UpdateNVTexture`, present at vsync
- Draw four textures in a 2×2 arrangement
- Crossfade between two render targets

**Done when:** a Pi 4 and a Pi 5 both display a 2×2 grid of moving synthetic video with a working crossfade, from a headless boot with no desktop session.

**If this fails**, the fallback is raw EGL/GLES with SDL providing only the window — a significant change that must be discovered now, not after the control plane is built.

## Stage 2 — Decode Spike

The second risk, also throwaway, in `cmd/spike-decode`.

- Open an RTSP and an HLS source with go-astiav
- Probe available hwaccels and V4L2 devices
- Decode HEVC through `-hwaccel drm` on a Pi 5, and H.264 through V4L2 M2M on a Pi 4
- Transfer frames to system memory as NV12
- Measure per-frame cost and sustained frame rate for one, four, and nine concurrent 1080p streams on both models

**Done when:** there are real numbers for how many streams each Pi model sustains, for both codecs, hardware and software. These numbers go into [hardware-decode.md](../architecture/hardware-decode.md) to replace the current qualitative capacity guidance, and into the UI's capacity warnings.

## Stage 3 — Frame Pipeline

Join the two spikes.

- `internal/frame` — `Frame` type, per-source pool, triple-buffered lock-free slot
- `internal/ingest` — decode worker on a locked OS thread, reconnect with backoff
- `internal/ingest/capability` — platform probe surfaced as a structured result
- `internal/display/texture` — per-source texture cache keyed by `(source, w, h, format)`, upload on the render thread

**Done when:** a hard-coded list of stream URLs renders live in a 2×2 grid on a Pi, survives unplugging and reconnecting a camera, and passes `go test -race`.

## Stage 4 — Display Engine

- `internal/display/layout` — layout catalogue as data, geometry solver, fit modes
- `internal/display/transition` — the full catalogue; geometric and alpha families first, masked family behind a capability check
- `internal/display/compositor` — scene assembly as data
- `internal/display/engine.go` — the loop from [render-loop-pattern.md](../patterns/render-loop-pattern.md), command channel, atomic state snapshot
- `internal/schedule` — tour, playlist, and tile-sequence timers on a wall clock

**Done when:** the engine renders a strategy snapshot handed to it in code, steps a tour with transitions, and the layout solver, transitions, scheduler, and compositor all have unit tests that need no GPU.

## Stage 5 — Persistence and API

- `internal/model`, `internal/database` — schema and migrations for the entities in [display-strategy-pattern.md](../patterns/display-strategy-pattern.md)
- `internal/source` — registry, lifecycle, probing; `internal/source/resolver` for yt-dlp, direct, and file
- `internal/rest`, `internal/handler` — full endpoint set, SSE hub, SPA handler
- `api/openapi.yaml` — hand-maintained, served at `/docs`
- `internal/preview` — throttled read-back, downscale, JPEG encode, MJPEG stream
- Auth: bcrypt-hashed accounts, JWT

**Done when:** the entire display strategy can be built through the API and takes effect live, `/docs` is complete, and `openapi.yaml` matches every handler.

## Stage 6 — Frontend

- Vite, Vue 3, Vuetify 3, Pinia, file-based routing, `importMode: 'sync'`
- Common components: StandardCard, StandardDialog, BackButton
- `useEventStream` and `useDisplayState` composables
- Preview page with PreviewCanvas and tile status
- Stream Sources page with per-kind forms, probe display, and upload
- Display Strategy page with LayoutPicker, TileEditor, PlaylistEditor, TransitionEditor, TourEditor
- `//go:embed` integration and the `embed_frontend` build tag

**Done when:** every capability of the API is reachable from the UI, and the UI is usable with `-display=false` for development.

## Stage 7 — Packaging

- GoReleaser configs for `linux/amd64` and `linux/arm64`, plus `.deb` packages
- `scripts/build-deb.sh` using goreleaser-cross in Docker
- systemd unit with the required group membership and device access
- `addon/` — Home Assistant add-on: `config.yaml`, `build.yaml`, `Dockerfile`, s6 `run`/`finish` scripts, `translations/en.yaml`, `README.md`, `CHANGELOG.md`
- `repository.yaml` at the repository root
- `.github/workflows/addon.yml` — multi-arch images to GHCR
- Ingress support: `getIngressBase()` in the frontend, `<base href>` injection in the Go SPA handler

**Done when:** the add-on installs from the repository URL on a Home Assistant OS Pi, the UI opens through ingress, and the display lights up.

## Open Questions

These need answers from the spikes or from a decision before the stages that depend on them.

| Question | Blocks | Notes |
|----------|--------|-------|
| Does the Home Assistant add-on sandbox permit taking DRM master? | Stage 7 | Needs `devices: /dev/dri` and probably `video: true`. HA OS runs no desktop, so nothing should be holding it, but this is unverified. |
| Does the Pi's SDL3 GPU renderer support custom fragment shaders? | Masked transitions in Stage 4 | If not, `irisWipe`, `ellipseWipe`, and `clockWipe` degrade to `fade` permanently rather than situationally. |
| How many concurrent decoder instances will the Pi's hardware blocks accept? | Capacity limits in Stage 3 | Exceeding the limit fails confusingly; we need a hard cap and a clear error. |
| Is the Raspberry Pi OS system FFmpeg patched with the V4L2-request hwaccels? | Stage 2, packaging | If yes, we avoid shipping the `jc-kynesim` fork. If no, the `.deb` and add-on image must carry it. |
| Should the add-on image bundle `yt-dlp`? | Stage 7 | The standalone build discovers it on `PATH`. A container has no user-managed `PATH`, so the add-on may need to install it at build time and self-update, or accept that YouTube sources need a manual step. |
| Output rotation: SDL or KMS? | Stage 4 | Portrait installations are common. SDL-level rotation costs a render target; KMS-level rotation may not be available on all Pi outputs. |

## Non-Goals for v0.1

Restating from [product-overview.md](../product-overview.md#scope) so the roadmap is unambiguous: no audio output, no multiple simultaneous display outputs, no recording or analytics, no DRM-protected services, no fleet management.
