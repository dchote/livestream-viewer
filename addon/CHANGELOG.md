# Changelog

## 0.1.1

- Drop the native 8WI `application.yaml`; Home Assistant and 8WI Supervisor both use `config.yaml`.

## 0.1.0

- Control plane, embedded UI, Swagger, and Home Assistant ingress.
- Ingest, hardware decode probe (VideoToolbox waits for a keyframe; RTSP stays on software unless probe produced a hardware frame), and SDL3 display engine (windowed and KMSDRM).
- Add-on image includes FFmpeg 8, SDL3 3.4+ with KMSDRM, Mesa, and a YouTube stack (yt-dlp with EJS, Deno, BgUtils PO token provider, bgutil yt-dlp plugin). yt-dlp self-updates on start. Cookies remain available for the bot check and for private/members/age-gated videos.
- Preview MJPEG and SSE require ingress streaming.
