# Build and Test Guide

> **Status:** The binary links FFmpeg 8.x (`go-astiav`) to build. SDL3 3.4+ is loaded at runtime when `-display=true`. Default tests do not initialise SDL.

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.25+** | Match CI, Docker, and nFPM packaging |
| **CGO enabled** | Required for SQLite and FFmpeg bindings |
| **Node.js 20+ and Yarn** | Only for full builds with the embedded frontend |
| **FFmpeg 8.x development libraries** | `libavcodec` 62.x. `go-astiav` is n8.0-only — do not link FFmpeg 9 |
| **SDL3 3.4+** | Runtime shared library (`libSDL3.dylib` / `libSDL3.so.0`). Loaded with `sdl.LoadLibrary`, never `binsdl` |
| **pkg-config** | Locates the FFmpeg 8 libraries |
| **`yt-dlp`** | Runtime only. Required to resolve YouTube sources. Optional on a standalone host; **included in the Home Assistant add-on image** (with Deno/EJS and the BgUtils PO token provider). The Go binary **embeds** the bgutil plugin and copies it into `data/yt-dlp-plugins/bgutil/`. The Linux `.deb` also ships Deno and the provider server. |

### macOS (development)

Homebrew `ffmpeg` / `ffmpeg-full` may be 9.x. cgo must use keg-only **ffmpeg@8**:

```bash
brew install pkg-config ffmpeg@8 sdl3 yt-dlp
source ./scripts/dev-env.sh
```

`scripts/dev-env.sh` sets `PKG_CONFIG_PATH` and `DYLD_FALLBACK_LIBRARY_PATH` to `/opt/homebrew/opt/ffmpeg@8`. Confirm:

```bash
pkg-config --modversion libavcodec   # expect 62.x, not 63.x (FFmpeg 9)
```

Leave FFmpeg 9 on `PATH` for the `ffmpeg`/`ffprobe` CLIs if you want; do not put `/usr/local` Intel leftovers on `PKG_CONFIG_PATH`.

Public YouTube livestreams on a Mac need a PO token HTTP server. Homebrew yt-dlp has no bgutil plugin (the Go process copies the embedded one). Typical local setup when Deno and `pot_server_dir` are not installed:

```bash
docker run --name bgutil-provider -d --init \
  -p 127.0.0.1:4416:4416 brainicism/bgutil-ytdlp-pot-provider
```

Leave `[youtube] pot_mode = "auto"` so the process uses the sidecar when `/ping` succeeds. If YouTube still returns the bot check, export cookies from Terminal (Chrome may prompt for the keychain):

```bash
./scripts/export-youtube-cookies.sh
```

### Debian / Ubuntu / Raspberry Pi OS

Stock distro packages are not FFmpeg 8. Use `brew install ffmpeg@8` (Homebrew-on-Linux) or build FFmpeg 8 shared libraries into a prefix and export `PKG_CONFIG_PATH`. SDL3 3.4+ is required at runtime for `-display=true`. Build SDL3 from source with `-DSDL_KMSDRM=ON` for headless panels.

CI uses Homebrew-on-Linux to install `ffmpeg@8`. The add-on image compiles FFmpeg 8 and SDL3 in Docker, and ships `yt-dlp`, Deno, and the BgUtils PO token provider for YouTube. Running that image as a Home Assistant add-on on Raspberry Pi is the confirmed production path for panel output.

## Releases

GitHub Actions are **manual** (`workflow_dispatch` only). CI does not run on push or pull request.

| Workflow | When to run |
| --- | --- |
| **CI** | Actions → CI → Run workflow. Vet, test, race, frontend, full binary. |
| **Release** (`channel=dev`) | Add-on images to GHCR `:dev`, `.deb` + linux binaries on a rolling `dev` pre-release (previous assets wiped). |
| **Release** (`channel=stable`, `version=X.Y.Z`) | `version` must match `addon/config.yaml`. Builds first, then creates git tag `vX.Y.Z` at that commit (fails if the tag already points elsewhere), pushes GHCR `:X.Y.Z`, publishes a GitHub Release. |

After the first GHCR push, mark `ghcr.io/dchote/{arch}-addon-livestream-viewer` **public** or Home Assistant OS cannot pull the add-on.

Linux `.deb` packages vendor FFmpeg 8 and SDL3 under `/usr/lib/livestream-viewer` and declare Debian `Depends` for OpenSSL, DRM/Mesa, VA-API, and SRT. They recommend host `yt-dlp` (and do not ship Deno or the PO token provider). For public YouTube livestreams on a `.deb` host, run the provider as a sidecar:

```bash
docker run --name bgutil-provider -d --init \
  -p 127.0.0.1:4416:4416 brainicism/bgutil-ytdlp-pot-provider
```

then leave `[youtube] pot_mode = "auto"` (it uses the sidecar when `/ping` on `127.0.0.1:4416` succeeds). Local package build:

```bash
./scripts/build-deb.sh              # snapshot: <config>-dev.<gitsha>
VERSION=0.1.0 ./scripts/build-deb.sh  # exact version for release-style packages
ARCHS=amd64 ./scripts/build-deb.sh
```

This builds the add-on image per architecture (with a runtime smoke check), extracts the binary and libraries, then packages with **nFPM** (`scripts/package-deb.sh`).

### Headless Linux display configuration (Raspberry Pi example)

The Home Assistant add-on on Raspberry Pi is the confirmed production path; Home Assistant OS already provides a KMS display stack, so no `config.txt` edits are required for that install. For a standalone Raspberry Pi OS host, add to `/boot/firmware/config.txt`:

```
dtoverlay=vc4-kms-v3d
```

Append `,cma-512` for 4K output. Add the service user to the `video` and `render` groups. Nothing else may hold DRM master.

## Building

```bash
source ./scripts/dev-env.sh   # macOS ffmpeg@8
SKIP_FRONTEND=true ./scripts/build.sh
# or
CGO_ENABLED=1 go build -o livestream-viewer ./cmd/livestream-viewer
```

Full UI build: `./scripts/build.sh`. Never build with `-tags embed_frontend` unless `cmd/livestream-viewer/frontend-dist/` contains files.

## Testing

Always use a timeout. No test opens a live network stream. SDL-only tests stay behind `-tags sdl`.

```bash
source ./scripts/dev-env.sh
CGO_ENABLED=1 go test -timeout=30s ./...
CGO_ENABLED=1 go test -race -timeout=60s ./internal/frame/... ./internal/display/... ./internal/schedule/...
CGO_ENABLED=1 go test -tags sdl -timeout=60s ./internal/display/output/...
```

Do not start the display engine in a default unit test. Iterate the windowed binary against real cameras separately (`go run ./cmd/livestream-viewer -display=true`).

## Running in Development

```bash
source ./scripts/dev-env.sh
go run ./cmd/livestream-viewer -display=false -frontend-embed=false
# Vite: cd frontend && yarn dev
```

Windowed SDL (macOS / desktop Linux):

```bash
source ./scripts/dev-env.sh
go run ./cmd/livestream-viewer -display=true
```

## Common Issues

### CGO errors mentioning `libavcodec` 63.x or FFmpeg 9

`go-astiav` does not compile against FFmpeg 9. Point `PKG_CONFIG_PATH` at `ffmpeg@8`.

### `pkg-config: exec: "pkg-config": executable file not found`

Install `pkg-config`.

### Black screen on the Pi, no error

1. `SDL_VIDEO_DRIVER=kmsdrm` (SDL3; `SDL_VIDEODRIVER` is ignored)
2. SDL3 built without KMSDRM
3. Missing `dtoverlay=vc4-kms-v3d`
4. Another client holds DRM master
5. Event queue not pumped

### `sdl init: error getting KMSDRM displays information`

SDL found no card it could drive. The error now carries a per-card breakdown of
`/dev/dri` with each connector's status, which separates the three causes:
`/dev/dri` missing from the container, no connected panel, or a `display.device`
pinned to the wrong card. Setting `display.device` makes SDL take that index
verbatim and skip its own scan, so on a board whose `card0` is the render-only
node (`v3d`, with `vc4` on `card1`) a pin to 0 fails outright. Leave it unset.

### `hardware accelerator failed to decode picture` (macOS)

VideoToolbox cannot start on a P-frame. Joining live RTSP mid-GOP used to log that line for every picture until the next IDR. Current builds wait for a keyframe, skip `AV_CODEC_FLAG_LOW_DELAY` on hardware opens, and demote residual VT lines. If a camera never produces a hardware frame, probe records `hw_decode: false` and the worker stays on software. See [Hardware Decode](architecture/hardware-decode.md#macos--videotoolbox).

### Tests hang

Add `-timeout=30s`. Check for tests that open sockets or initialise SDL without the `sdl` tag.
