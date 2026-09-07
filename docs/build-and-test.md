# Build and Test Guide

> **Status:** Control-plane build and tests run without linking FFmpeg or SDL3. Optional `ffprobe` / `yt-dlp` / `ffmpeg` on `PATH` enable probe and thumbnails. FFmpeg **link** dependencies and SDL3 are not required until the display/decode stages land.

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.25+** | Match CI, Docker, and goreleaser-cross |
| **CGO enabled** | Required for SQLite (and later for FFmpeg bindings) |
| **Node.js 20+ and Yarn** | Only for full builds with the embedded frontend |
| **`yt-dlp`** | Runtime only, optional. Required to resolve YouTube sources; discovered on `PATH` |
| **`ffprobe` / `ffmpeg`** | Runtime only, optional. Probe and thumbnail extraction use subprocesses when present; missing tools return `probe.status: unavailable` rather than failing the build |
| **FFmpeg 8.x development libraries** | **Not linked yet.** Needed when the decode stage lands (`libavcodec`, `libavformat`, `libavutil`, `libavfilter`, `libswscale`) |
| **SDL3 3.4+** | **Not linked yet.** Runtime shared library; must be built with KMSDRM for headless Linux panel output |
| **pkg-config** | Used later to locate the FFmpeg libraries |

The current control-plane binary only needs Go (CGO for SQLite) and, for a full UI build, Node/Yarn. FFmpeg **development** libraries and SDL3 are for later display/decode stages and are **not linked** today. On a Mac used for API/UI work, optional `brew install yt-dlp ffmpeg` puts `yt-dlp`/`ffprobe`/`ffmpeg` on `PATH` for probe and thumbnails; SDL3 is unnecessary with `-display=false`.

### Installing dependencies

**Debian / Ubuntu / Raspberry Pi OS / other Linux:**

```bash
sudo apt install -y build-essential pkg-config libsqlite3-dev \
  libavcodec-dev libavformat-dev libavutil-dev libavfilter-dev libswscale-dev \
  libsdl3-dev libdrm-dev libgbm-dev
```

If `libsdl3-dev` is not available for your distribution, build SDL3 from source with `-DSDL_KMSDRM=ON` when you need headless panel output. Verify libdrm and libgbm development packages are installed **before** configuring SDL, or KMSDRM will be silently omitted from the build and a headless host will show a black screen with no error.

**macOS (development):**

```bash
brew install pkg-config ffmpeg sdl3
```

macOS has no KMSDRM; the display engine opens a normal window. This is the expected development setup for layout and transition work. Control-plane-only work needs neither SDL3 nor FFmpeg development libraries — use `-display=false`.

### Headless Linux display configuration (Raspberry Pi example)

On Raspberry Pi OS, add to `/boot/firmware/config.txt`:

```
dtoverlay=vc4-kms-v3d
```

Append `,cma-512` for 4K output. Add the service user to the `video` and `render` groups so it can open `/dev/dri/*`. Nothing else may hold DRM master — no desktop session, no other DRM client. Other SBCs have equivalent DRM enablement steps; document them as they are validated.

## Building

### Server only (no frontend)

The fast path. Skips the Vue build and uses a stub that returns `nil` for the embedded frontend:

```bash
SKIP_FRONTEND=true ./scripts/build.sh
# Binary at build/livestream-viewer
```

Or directly:

```bash
CGO_ENABLED=1 go build -o livestream-viewer ./cmd/livestream-viewer
```

### Full build (with embedded frontend)

Requires Node and Yarn. Builds the Vue app, copies it to the embed location, and compiles with the `embed_frontend` tag:

```bash
./scripts/build.sh
# Binary at build/livestream-viewer
```

**Important:** the `embed_frontend` tag requires `cmd/livestream-viewer/frontend-dist/` to exist and contain at least one file. Without it the build fails:

```
pattern frontend-dist: cannot embed directory frontend-dist: contains no embeddable files
```

Never build with `-tags embed_frontend` unless that directory is populated. Either run the full build script or build the frontend first:

```bash
cd frontend && yarn build && cd ..
cp -r frontend/dist cmd/livestream-viewer/frontend-dist
CGO_ENABLED=1 go build -tags embed_frontend -o build/livestream-viewer ./cmd/livestream-viewer
```

### Cross-compiling for Linux targets (amd64 / arm64)

Because the binary will link FFmpeg and needs the target's SDL3 once the display stage lands, cross-compilation requires a matching sysroot. The supported path is building inside containers, the same as the release pipeline:

```bash
make build-deb
# or
./scripts/build-deb.sh
```

This builds the frontend, then runs GoReleaser in snapshot mode inside Docker for both `linux/amd64` and `linux/arm64` so CGO uses the correct toolchain for each. Output lands in `dist/`:

- `dist/*_amd64.deb`
- `dist/*_arm64.deb`

Use `SKIP_FRONTEND=1 ./scripts/build-deb.sh` if the frontend is already built. `make frontend` builds only the frontend.

Plain `GOARCH=arm64 go build` will not work for CGO-linked stages — CGO needs a cross toolchain and the target's FFmpeg and SDL3 headers. The current control-plane binary (SQLite only) can be built natively on any Go host.

## Testing

Always use a timeout. Decode and render tests can hang on a misconfigured environment, and a hung CI job is worse than a failed one.

```bash
CGO_ENABLED=1 go test -timeout=30s ./...
```

With the race detector (use a longer timeout):

```bash
CGO_ENABLED=1 go test -race -timeout=60s ./...
```

Race detection is **mandatory** for the scheduler and (later) frame-handoff packages, because their correctness rests on concurrent command handling and atomics:

```bash
CGO_ENABLED=1 go test -race -timeout=60s ./internal/schedule/...
```

### Test a single package

```bash
CGO_ENABLED=1 go test -timeout=30s ./internal/display/layout/...
```

### Tests that need a display

Tests requiring a real SDL context are behind the `sdl` build tag and excluded from the default run:

```bash
CGO_ENABLED=1 go test -tags sdl -timeout=60s ./internal/display/...
```

Do not add SDL-dependent tests to the default set. CI has no display, and on a headless Linux host such a test would take DRM master away from a running instance.

### What not to do

- **Do not run the application to validate a change.** It takes the main thread and, when display is enabled on Linux, DRM master. Use `go build` and `go test`.
- **Do not start the display engine in a unit test.** The layout solver, transitions, scheduler, and compositor are all deliberately testable without one.
- **Do not add tests that open network streams.** Use recorded fixtures or a synthetic source.

## Vet and Lint

```bash
go vet ./...
```

Frontend:

```bash
cd frontend && yarn lint
```

## Running in Development

The display engine and the management UI are developed separately most of the time.

**API and UI only, no display:**

```bash
go run ./cmd/livestream-viewer -display=false -frontend-embed=false
```

`-display=false` skips SDL entirely, so the API and UI run on any machine. `-frontend-embed=false` stops the Go server serving the SPA so it does not conflict with the Vite dev server.

**Vite dev server with hot reload:**

```bash
cd frontend && yarn dev
# UI on :3000, proxying /api, /docs, /health, and /events to :8099
```

**With a display window (macOS or desktop Linux):**

```bash
go run ./cmd/livestream-viewer -display=true
```

SDL opens a normal window. Layouts, transitions, and the scheduler all behave identically to the Pi; only the output surface differs.

## Build Pipeline

1. `cd frontend && yarn build` — Vite produces `frontend/dist/` as a single bundle (`importMode: 'sync'`)
2. `cp -r frontend/dist cmd/livestream-viewer/frontend-dist` — copy to the embed location
3. `CGO_ENABLED=1 go build -tags embed_frontend ./cmd/livestream-viewer` — `//go:embed` bakes it into the binary

## CI Alignment

`.github/workflows/ci.yml` runs:

1. `gofmt` check
2. `go vet ./cmd/... ./internal/... ./api/...`
3. `CGO_ENABLED=1 go test -race -timeout=60s ./cmd/... ./internal/... ./api/...`
4. Frontend `yarn lint` and `yarn build`
5. `./scripts/build.sh` (embedded frontend)

FFmpeg and SDL3 packages are **not** installed in CI until those libraries are linked.

Run the same commands locally to catch CI failures early.

## Releases

GitHub Releases are produced on a version tag (e.g. `v0.1.0`) and include Linux `amd64` and `arm64` binaries plus `.deb` packages. The add-on workflow separately builds and pushes multi-arch Home Assistant add-on images to GHCR.

The release workflow can also be run manually from **Actions → Release → Run workflow** to produce a snapshot without publishing; the artifacts are attached to the workflow run.

## Common Issues

### `cannot embed directory frontend-dist`

Run the full build (`./scripts/build.sh`) or use `SKIP_FRONTEND=true`.

### `pkg-config: exec: "pkg-config": executable file not found`

Install `pkg-config`. The FFmpeg bindings use it to locate the libav libraries.

### CGO errors mentioning `libavcodec` or `libavutil`

The FFmpeg development headers are missing or are the wrong major version. go-astiav pins FFmpeg `n8.0`; a 6.x or 7.x system FFmpeg will fail to compile or link.

### Black screen on the Pi, no error

In order of likelihood:

1. `SDL_VIDEO_DRIVER=kmsdrm` — SDL3 renamed SDL2's `SDL_VIDEODRIVER`. The old name is silently ignored.
2. SDL3 was built without KMSDRM. Check that libdrm and libgbm development packages were present at configure time.
3. `dtoverlay=vc4-kms-v3d` is missing from `config.txt`.
4. Something else holds DRM master — a desktop session or another DRM client.
5. The event queue is not being pumped. Under KMSDRM this produces a black screen with no diagnostic.

### Hardware decode reports 200% CPU

Almost certainly `hevc_v4l2m2m`, which cannot work — HEVC on the Pi uses the stateless V4L2 request API and must be reached through `-hwaccel drm`. See [Hardware Decode](architecture/hardware-decode.md).

### Tests hang

Add `-timeout=30s`. Check for tests that open sockets, start the engine, or block on I/O without a fixture.

### Go version mismatch

Use Go 1.25+ to match CI and the release containers. Confirm with `go version`.
