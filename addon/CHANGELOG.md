# Changelog

## 0.1.6

- Cap concurrent V4L2 M2M H.264 hardware decoders at one (Pi 4 / CM4 shared block); extras fall back to software so multi-source walls do not wedge `/dev/video*` and go black.
- Bound `Codec.Free` during session close so a stuck V4L2 driver cannot hang ingest workers.

## 0.1.5

- Prefer hardware decode whenever the host has a path for the stream codec; a prior `hw_decode=false` probe no longer locks V4L2/VA-API onto software.
- Probe records hardware for V4L2/VA-API from host capability (VideoToolbox still requires a proven frame).
- Require an H.264 decode M2M node before claiming H.264 HW (Pi 5 HEVC-only nodes no longer qualify).
- Refresh capability probing when `/dev/video*` / media / DRM nodes change.
- Shorten the Stream Sources decode capability line.

## 0.1.4

- Use V4L2 M2M (`h264_v4l2m2m`) for H.264 hardware decode on Pi 4/CM4 instead of incorrectly requiring a DRM hwdevice context.
- Probe real M2M nodes (`VIDIOC_QUERYCAP` / sysfs) so Pi 5 is not claimed as H.264-capable just because the decoder is linked.
- Expose board model, decode paths, and V4L2 M2M nodes on `GET /api/v1/system/info`.
- Add source option **Force hardware decode** (mutually exclusive with force software) to override a stale `hw_decode=false` probe after host device changes.

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
