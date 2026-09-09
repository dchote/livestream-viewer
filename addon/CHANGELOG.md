# Changelog

## 0.1.3

- Stop pinning KMSDRM to `card0`. Setting `SDL_KMSDRM_DEVICE_INDEX` makes SDL skip its scan for the card with a connected panel, so on boards whose `card0` is a render-only node the display engine failed with `error getting KMSDRM displays information`. `display.device` now defaults to auto-detect.
- Report the `/dev/dri` card and connector inventory on startup, and include it in the SDL init error so a missing device, an unplugged panel, and a held DRM master are distinguishable.

## 0.1.2

- Keep Debian `/usr/sbin` on `PATH` so the base image timezone service can find `dpkg-reconfigure` (exit 127 under 8WI Supervisor).
- Drop the Options `http_port` field; the app always binds 8099 to match ingress and the Network `8099/tcp` mapping.

## 0.1.1

- Drop the native 8WI `application.yaml`; Home Assistant and 8WI Supervisor both use `config.yaml`.

## 0.1.0

- Control plane, embedded UI, Swagger, and Home Assistant ingress.
- Ingest, hardware decode probe (VideoToolbox waits for a keyframe; RTSP stays on software unless probe produced a hardware frame), and SDL3 display engine (windowed and KMSDRM).
- Add-on image includes FFmpeg 8, SDL3 3.4+ with KMSDRM, Mesa, and a YouTube stack (yt-dlp with EJS, Deno, BgUtils PO token provider, bgutil yt-dlp plugin). yt-dlp self-updates on start. Cookies remain available for the bot check and for private/members/age-gated videos.
- Preview MJPEG and SSE require ingress streaming.
