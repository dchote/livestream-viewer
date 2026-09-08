# livestream-viewer Home Assistant Add-on

Native hardware-accelerated livestream viewer and video wall. **This is the primary install path.** Use **Open Web UI** to manage sources, screens, and the tour. The REST API and embedded web UI run via Home Assistant ingress.

The image includes FFmpeg 8, SDL3 3.4+ (KMSDRM), and a YouTube stack (`yt-dlp` with EJS, Deno, the BgUtils PO token provider, and the bgutil yt-dlp plugin). `yt-dlp` self-updates when the add-on starts so extractors do not rot between releases.

## Installation

1. In Home Assistant, go to **Settings** → **Add-ons** → **Add-on store**.
2. Click the three dots (⋮) → **Repositories**.
3. Add this repository URL: `https://github.com/dchote/livestream-viewer`
4. Find **livestream-viewer** in the add-on list and click **Install**.
5. Configure options if needed, then **Start** the add-on.

Supervisor pulls `ghcr.io/dchote/{arch}-addon-livestream-viewer` tagged with the add-on `version`. Those GHCR packages must be **public** or Home Assistant OS cannot pull them.

## Configuration

- **HTTP port** — Management UI and API (default 8099). Used by ingress.
- **Log level** — `debug`, `info`, `warn`, or `error`.
- **Display engine** — Start the display engine. Requires `/dev/dri` when driving a panel. Leave enabled on a host with an attached display; disable for control-plane-only use.

Data (database, uploads, JWT secret) is stored in the add-on’s persistent `/data` directory.

Default login after first start: username `admin`, password `admin`. Change this before exposing the UI.

## Web UI (ingress)

Click **Open Web UI** in the add-on panel. Preview, Stream Sources, and Display Strategy are the primary pages.

Preview MJPEG and live events are streamed through ingress. Swagger is at `/docs` on the add-on port.

## YouTube

YouTube sources work with no extra host packages. The image ships `yt-dlp` (with EJS solvers), Deno, the BgUtils PO token provider, and the `bgutil-ytdlp-pot-provider` yt-dlp plugin. On each start the add-on tries `pip install -U yt-dlp bgutil-ytdlp-pot-provider` and continues if that update fails.

Public livestreams use a local token provider that starts automatically with the add-on. That is not a guarantee: if YouTube still returns the bot check, upload cookies from a browser that can play the stream.

Cookies are also required for private, members-only, or age-restricted videos. Export a Netscape `cookies.txt` and upload it under **Stream Sources**. The file is stored in the add-on data volume (`data/secrets/youtube.cookies`) and survives restarts. There is no browser inside the add-on, so `--cookies-from-browser` is not used.

## Local image build

`BUILD_FROM` is **required** (arch-specific Home Assistant debian-base). From the repository root:

```bash
# amd64
docker build -f addon/Dockerfile \
  --build-arg BUILD_FROM=ghcr.io/hassio-addons/debian-base/amd64:9.2.0 \
  -t livestream-viewer-addon .

# arm64 / aarch64
docker build -f addon/Dockerfile \
  --build-arg BUILD_FROM=ghcr.io/hassio-addons/debian-base/aarch64:9.2.0 \
  -t livestream-viewer-addon .

docker run --rm -p 8099:8099 livestream-viewer-addon
```

A raw `docker run` is a smoke test (API/UI), not a substitute for Supervisor.

## Forks

If you install from a fork, update `image` in `addon/config.yaml` to your registry, or build and push your own images.

## Support

- [GitHub repository](https://github.com/dchote/livestream-viewer)
