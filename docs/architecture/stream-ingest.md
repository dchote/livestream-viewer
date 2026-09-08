# Stream Ingest

> **Status:** Implemented. In-process libav ingest via go-astiav, lock-free frame slots, reconnect, and libav probe/thumbnails.

This document covers the path from a configured source to a frame sitting in a slot ready for the renderer: URL resolution, demuxing, decoding, the frame handoff, and failure handling.

## Library Choice

### In-Process libav, Not an FFmpeg Subprocess

Almost every Go project in this space shells out to the `ffmpeg` binary. [go2rtc](https://github.com/AlexxIT/go2rtc) is pure Go for protocol handling and spawns `ffmpeg` for anything needing a codec. [MediaMTX](https://github.com/bluenviron/mediamtx) deliberately does not decode at all. [Frigate](https://github.com/blakeblackshear/frigate)'s birdseye multi-camera view — the closest functional analogue to this project — composites in Python and pushes rawvideo through a named pipe.

The subprocess approach buys real things: crash isolation, trivial upgrades, no cgo, and the ability to swap in a platform-specific FFmpeg build (for example a Raspberry Pi fork) without recompiling. It is genuinely the right call for many projects.

It is the wrong call here, for one arithmetic reason. Raw NV12 at 1080p30 is approximately **93 MB/s per stream** through a pipe, and every byte is a decoder→kernel→process copy. A 3×3 wall is roughly 840 MB/s of pure copying, which saturates memory bandwidth on low-cost boards before a single pixel is composited. Frigate's birdseye CPU cost is a well-known complaint and is good evidence for the alternative. It also adds at least a frame of buffering per hop, and it permanently forecloses zero-copy.

**We link libav in-process via cgo.**

### go-astiav

[`asticode/go-astiav`](https://github.com/asticode/go-astiav) is the maintained choice. It is pinned to FFmpeg `n8.0` and exposes what we need:

- `HardwareDeviceTypeDRM`, `HardwareDeviceTypeVAAPI` and friends
- `CreateHardwareDeviceContext` wrapping `av_hwdevice_ctx_create`
- `AVHWFramesContext` via `AllocHardwareFramesContext` / `SetHardwarePixelFormat` / `Initialize`
- `PixelFormatDrmPrime` and `PixelFormatNv12`
- A `get_format` callback hook, so we can *select* a hardware pixel format rather than being forced through the CPU readback path
- Working `hardware_decoding_filtering` and `hardware_encoding` examples

Two things to design around:

1. **astiav ships breaking changes without a major version bump** — see its [BREAKING_CHANGES.md](https://github.com/asticode/go-astiav/blob/master/BREAKING_CHANGES.md). Pin an exact version in `go.mod` and treat upgrades as deliberate work.
2. **It does not wrap `AVDRMFrameDescriptor` field-by-field.** Reaching the DMA-BUF file descriptors requires a small local cgo shim casting `frame.Data()[0]`. Only needed if we pursue zero-copy.

Rejected alternatives: `u2takey/ffmpeg-go` (CLI wrapper, last pushed May 2024), `3d0c/gmf` (abandoned 2022, FFmpeg 4.x), `giorgisio/goav` and `asticode/goav` (both superseded by astiav).

### Protocol Handling

libavformat handles RTSP, HLS, DASH, MPEG-TS, SRT, and RTMP directly, so there is no need for a separate Go protocol stack in the hot path.

[`bluenviron/gortsplib`](https://github.com/bluenviron/gortsplib) is worth knowing about but is not used for playback: it handles RTSP negotiation and RTP depacketisation but **does not decode** — its own examples add a hand-written cgo libav decoder to get frames. Using it would mean maintaining our own depacketisation layer for no benefit, since libavformat already does that work.

Pure-Go manifest libraries ([`Eyevinn/hls-m3u8`](https://github.com/Eyevinn/hls-m3u8), [`asticode/go-astits`](https://github.com/asticode/go-astits)) may be useful later for inspecting or rewriting manifests, but not for playback. Note that `grafov/m3u8` is archived; `Eyevinn/hls-m3u8` is its maintained successor.

## Source Resolution

Some sources need work before libavformat can open them. Resolution runs in the control plane, never in the render loop.

### YouTube

There is no legitimate pure-Go option. [`kkdai/youtube`](https://github.com/kkdai/youtube) exposes `HLSManifestURL` but does not handle live playback, and it lags badly behind YouTube's signature and PoToken changes.

We use [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) as an external subprocess to obtain a manifest URL, then hand that URL to libavformat:

```bash
yt-dlp -g --no-warnings [--plugin-dirs <data/yt-dlp-plugins> --plugin-dirs default] [--cookies <data/secrets/youtube.cookies> | --cookies-from-browser <browser>] [--extractor-args youtube:po_token=…] [--extractor-args youtubepot-bgutilhttp:base_url=…] -f "bv*[protocol*=m3u8]/b[protocol*=m3u8]/bv*/b" "<url>"
```

YouTube live streams publish separate video-only and audio-only HLS playlists, not a muxed `best` format. The wall is video-only, so the selector prefers video-only HLS (`bv*`) and falls back to muxed HLS or non-HLS for VOD. `-g` must return a single URL; requesting `bv*+ba` would print two.

YouTube's extractor often refuses anonymous requests (`Sign in to confirm you're not a bot`). That check is **process-wide**, not per source, and it is an IP-reputation / missing proof-of-origin-token problem rather than an account problem. A [BgUtils PO token provider](https://github.com/Brainicism/bgutil-ytdlp-pot-provider) mints tokens locally on `127.0.0.1:4416`. Modes: `auto` (managed when Deno and the server directory exist; otherwise **external on the default URL, keeping `/ping` until a sidecar or managed server answers**), `managed`, `external`, `off`. The Home Assistant add-on and the Linux `.deb` ship Deno and the server directory. A Homebrew `yt-dlp` does **not** include the bgutil plugin; the process copies the **embedded** plugin into `data/yt-dlp-plugins/bgutil/` (yt-dlp only loads `plugin-dirs/*/yt_dlp_plugins`) and passes `--plugin-dirs`. Without that plugin a running HTTP provider is ignored. A PO token does not guarantee a bypass: if the watch page itself returns `LOGIN_REQUIRED`, cookies from a browser that can play the stream on this host are required. PO tokens are bound to the egress IP, so the provider must run on the same host as yt-dlp.

Cookies are used for the bot check when a PO token is not enough, and for private, members-only, or age-restricted videos. The operator can upload one Netscape cookie jar on Stream Sources; it is stored at `data/secrets/youtube.cookies` (mode 0600) and passed as `--cookies` when the file exists. On a Mac/desktop host, `[youtube] cookies_from_browser = "chrome"` (or `LSV_YOUTUBE_COOKIES_FROM_BROWSER`) passes `--cookies-from-browser` instead until a jar is uploaded; Chrome may prompt for the keychain. `./scripts/export-youtube-cookies.sh` dumps a jar from Terminal. An optional manual PO token lives beside the jar and, when set, is passed as `youtube:po_token=` (it wins over the provider). Cookie values are never returned by the API. The Go resolver does **not** put `--cookies` in the add-on `yt-dlp.conf` — an empty path would break every extract.

Extractor failures are classified (`youtube_bot_check` for the bot check, `youtube_auth` for a signed-in session, `youtube_unavailable`, `tool_missing`). The UI shows one sentence; the raw dump stays in the logs. Uploading or clearing cookies, or the provider transitioning to running, restarts every YouTube decode worker so a tripped circuit breaker does not sit on `failed` until something else changes.

Three rules:

1. **`yt-dlp` is discovered on `PATH`, never compiled into the Go binary.** It releases constantly. If it is absent, YouTube sources are marked unavailable with a message telling the user how to install it. Everything else keeps working. The Home Assistant add-on image installs `yt-dlp` (with EJS), Deno, and the BgUtils PO token provider as ordinary PATH tools and self-updates `yt-dlp` on start. The Linux `.deb` ships Deno and the provider; yt-dlp remains a recommended host package. The Go binary embeds the bgutil plugin and copies it under `data/yt-dlp-plugins/bgutil/` so Homebrew yt-dlp can load it.
2. **Resolved URLs are time-limited and IP-bound.** The returned URL carries `expire`, `ip`, and `sig` parameters, so resolution is a periodically-refreshed operation and the result is never stored in the database. The worker re-resolves on every reconnect, and additionally recycles an otherwise-healthy session after 30 minutes — well inside the expiry window — so the URL is refreshed before it dies. The tile holds its last frame across the reopen.
3. **Resolution has a timeout and a circuit breaker.** `yt-dlp` and `ffprobe` always run under a deadline, including when the caller supplies a context that has none — a decode worker's context is only cancelled at shutdown, so without this a hung subprocess would wedge the worker for the life of the process. On Unix the subprocess is started in its own process group and that group is killed when the deadline fires, so Python/Deno children (EJS, the POT plugin, `--cookies-from-browser`) do not survive as orphans. After five consecutive resolution failures the source trips a circuit breaker: it reports `failed` rather than `reconnecting`, and retries at the backoff ceiling instead of hammering the subprocess. A single success clears it. `GET /api/v1/system/info` reports `yt_dlp` and `yt_dlp_version` (`yt-dlp --version`) so the Stream Sources card can show which binary is on PATH.

YouTube's Terms of Service prohibit accessing content by means other than the Service's own interface. Treating `yt-dlp` as a user-supplied external tool is both the practical and the appropriate posture; this is documented in the UI where YouTube sources are added.

### RTSP

Handed to libavformat. **Transport defaults to TCP** (`rtsp_transport=tcp`): interleaved TCP is dramatically more reliable than UDP on a congested or wireless network, and the added latency is negligible for a wall. UDP is available per source for users who need the last few milliseconds.

Live RTSP/RTSPS opens use ~20s socket timeouts (`stimeout` / `timeout` / `rw_timeout`) and **do not** set `fflags nobuffer` or `flags low_delay`. Those low-latency flags make sparse IP-camera GOPs look idle; FFmpeg then tears down the TLS session and prints `[tls @ …] Unknown error`. File sources still use nobuffer/low_delay.

Workers for a given NVR are staggered (~150ms apart) so four simultaneous RTSPS handshakes do not collide.

Credentials go in the source record and are injected into the URL at open time so they never appear in logs.

### HLS, DASH, MPEG-TS, SRT, RTMP

Passed straight to libavformat with a sensible timeout and reconnect options. DASH is the weakest area in pure Go, which is another reason to let libavformat own it.

HLS (including YouTube) is segmented: libavformat reads a whole MPEG-TS chunk, then the decoder can emit two seconds of frames in a few milliseconds. The frame slot is only six frames deep, so dumping a segment into it looks like a stall then a burst. Segmented sources therefore:

1. **Preroll at the demuxer.** `options.buffer_ms` (default **4000** for YouTube/HLS/DASH, **0** for RTSP) maps to FFmpeg `live_start_index` (4000 ms → `-3`, the FFmpeg default). That is a few seconds of compressed segments, not a decoded-frame queue. `0` is live-edge (`-1`) plus `fflags nobuffer`. `http_multiple=1` overlaps the next segment fetch. HLS-private keys are not set on RTSP or DASH.
2. **Pace only segmented sources.** After each decode, the worker waits until that frame's PTS is due, *then* publishes. RTSP is already a realtime clock and is not paced. Lateness up to `buffer_ms` is absorbed; beyond that the origin snaps instead of dumping. File sources are paced so a looped idle card plays at the right speed.
3. **Codec low-delay only on software when the buffer is 0.** `AV_CODEC_FLAG_LOW_DELAY` belongs on the codec context, not the format `flags` dict. Hardware opens never set it: VideoToolbox has its own reorder buffer, and combining the two yields `vt decoder cb: output image buffer is null`.

`options.force_software` skips hardware decode for that source even when the host would otherwise use it. Changing buffer, force-software, URL, or credentials restarts the worker.

### Files

Uploaded through `POST /api/v1/uploads` into the data directory. Played on loop by seeking to the start on EOF. Useful for idle cards and offline fallbacks.

## Decode Worker

One worker per **needed** source — every enabled source referenced by any screen in the current strategy, not only the tiles on screen right now. Pre-rolling the rest of the wall means Goto, tour, and playlist cuts already have a decoded frame instead of a black tile while RTSPS reconnects.

Each worker is pinned to its own OS thread with `runtime.LockOSThread()` because libav calls are cgo and long-running — leaving them on a shared goroutine scheduler thread starves the rest of the runtime.

```
resolve URL (kind-specific)
open input                              libavformat
find best video stream
  discard audio and subtitle streams    ← no audio in scope
select decoder + hwaccel                see capability probe below
open codec context
loop:
    read packet
    if packet.stream != video: unref, continue
    if hardware decode and no keyframe yet: unref, continue
    send packet to decoder
    receive frames:
        if hardware-backed: transfer to system memory as NV12
        if already NV12 or YUV420P: keep native layout; else swscale to NV12
        wait until the frame's PTS is due          ← segmented sources only
        publish into the source's frame slot
        unref
on EOF:   loop (files) or treat as disconnect (live)
on error: classify, back off, reconnect
```

### Audio Is Discarded at the Demuxer

Audio packets are dropped before they reach a decoder. This is not merely a feature omission — it removes A/V synchronisation from the system entirely, which is what allows the renderer to be display-clock driven and to drop stale frames freely. See [Display Pipeline](display-pipeline.md#presentation-and-pacing).

### Hardware Acceleration Selection

At startup the capability prober inspects the platform once and caches the result:

- DRM render nodes under `/dev/dri`
- V4L2 M2M devices (for example `/dev/video10` and neighbours on a Raspberry Pi 4)
- V4L2 stateless request devices (for example `/dev/video19` + `/dev/media*`, `rpivid` on Pi)
- Desktop hwaccels where present (VA-API, VideoToolbox, …)
- Which FFmpeg hwaccels and decoders the linked libav actually offers
- VA-API on development machines

Per source, the worker then picks the best available path for the stream's codec, and records the decision on the source's probe record so the UI can show it. If no hardware path exists, it falls back to software decode and says so — silently dropping frames while the user believes they have acceleration is the failure mode this design most wants to avoid.

**VideoToolbox (macOS) does not conceal missing references.** Joining an RTSP camera mid-GOP and feeding P-frames to the hardware decoder produces a burst of `hardware accelerator failed to decode picture` until the next IDR. Hardware sessions wait for a keyframe before `SendPacket`, do not set `AV_CODEC_FLAG_LOW_DELAY` (that flag fights VideoToolbox's own reorder buffer), and pass `hwaccel_flags=+allow_profile_mismatch+ignore_level`. IP-camera RTSP still stays on software unless a probe actually produced a hardware frame.

A configurable cap limits concurrent hardware decoder instances, because the Pi's decoder blocks are a finite resource and exceeding them fails in confusing ways.

Details, including the Pi 4 versus Pi 5 divergence, are in [Hardware Decode](hardware-decode.md).

### Frame Conversion

Hardware-decoded frames on some SBCs are not linear NV12 — on Raspberry Pi they are Broadcom **SAND** tiled. The baseline path transfers them to system memory and detiles to linear NV12 (`hwdownload,format=nv12` in filter terms). The cost is one copy plus a detile per frame per stream; at 1080p that is manageable on modern Cortex-A cores.

Software-decoded 4:2:0 stays planar I420 and uploads as `SDL_PIXELFORMAT_IYUV`. Converting it to NV12 would rewrite every luma and chroma sample in system memory before the GPU sees it — the common path on a Pi 5, where H.264 is software-only. Other pixel formats (P010, 4:2:2, …) still go through `swscale` to NV12. Hardware download remains NV12 because that is what `hwdownload` produces after SAND detile.

Packing uses `av_image_copy_to_buffer` with align 1 so GPU pitches equal width. Decoder `linesize` is typically 32-byte padded; uploading the padded rows would spend memory bandwidth on bytes the panel never displays.

## Frame Handoff

The decoder and the renderer are connected by a **frame slot**: a bounded single-producer/single-consumer presentation queue, six frames deep over a pool of eight buffers.

- The decoder **writes packed pixels into a reserved pool buffer** (`Prepare` / `Commit`) and appends that buffer to the queue. There is no staging buffer and no second copy.
- The renderer takes the frame due at the next refresh, judged by PTS against that source's presentation clock, and discards anything older. If nothing new is due it reuses the texture it already uploaded.
- The pool outsizes the queue by the buffer being filled and the one the consumer holds, so the producer always has somewhere to write without waiting.

**The queue is bounded and evicts its oldest entry, and that is deliberate.** An unbounded queue lets the renderer fall behind and never catch up, and converts a transient decode stall into permanent added latency. Dropping old frames is the correct behaviour for a viewer: freshness beats completeness. What the depth buys is the ability to hold a frame that has arrived but is not yet due — a newest-only slot silently loses any frame whose successor arrives before the refresh it belonged to. The renderer keeps its non-blocking guarantee: the slot's mutex covers index bookkeeping and is never held across a copy.

See [Presentation timing](display-pipeline.md#presentation-timing) for how the renderer chooses.

**The publish path allocates nothing in steady state.** The producer reserves a pool buffer, packs into it, and queues the same buffer. `Slot.Publish` still copies, and exists for tests; ingest does not use it. An earlier version packed into a session-owned staging buffer and then copied each plane into the pool, which at 1080p across a handful of cameras meant hundreds of megabytes per second of extra traffic on hardware chosen for being cheap.

`AVFrame` and `AVPacket` objects are explicitly unreferenced. Note that the unref must be deferred *before* the operation that can fail — a failed `av_hwframe_transfer_data` can still leave buffers attached to the destination frame, which is reused for the whole session.

See [Concurrent State Pattern](../patterns/concurrent-state-pattern.md).

## Failure Handling

Failures are per-source and never propagate. A dead camera must not disturb the other eight tiles.

| Condition | Response |
|-----------|----------|
| Connect fails | Exponential backoff with jitter, capped at 30s. The attempt counter is unbounded for a permanently dead source, so the doubling is bounded iteration, not a shift — overflowing it produced a negative delay and a panic after about 25 minutes. |
| Stream drops mid-play | Same backoff, **reset after a session that published frames**; YouTube sources re-resolve their URL first. The last uploaded texture stays on the tile until a new frame arrives. |
| Decode error on a packet | Skip the packet, count it; repeated errors within a window trigger a decoder reset. Hardware sessions also drop packets until the first keyframe so VideoToolbox is not fed mid-GOP P-frames. |
| Hardware decoder unavailable at open | Fall back to software, record the downgrade on the source status |
| No frames while the source is still needed | Hold the last uploaded frame (placeholder only if this source has never decoded) |
| Resolution or format change | Publish the new geometry with the frame; renderer recreates its texture |
| File EOF | Seek to start (loop) |
| Repeated resolution failure | After five consecutive failures the source reports `failed` and retries at the backoff ceiling; cleared by one success |
| Panic in a decode session | Recovered and reported as an ordinary session error, so the reconnect loop handles it. One hostile bitstream must not end the process. |

Every state change is published over Server-Sent Events (`GET /api/v1/events`) so the Preview page reflects reality without polling.

## Probing

`POST /api/v1/sources/:id/probe` opens the source, reads enough to determine codec, resolution, frame rate, and pixel format, decodes one frame for a thumbnail, and closes. The result is cached on the source record.

Probing is what makes the Settings UI honest: it can tell the user, before they build a 3×3 grid, that six of their cameras are H.264 and this host has no hardware path for that codec.

## References

- [`asticode/go-astiav`](https://github.com/asticode/go-astiav)
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp)
- [`bgutil-ytdlp-pot-provider`](https://github.com/Brainicism/bgutil-ytdlp-pot-provider) — PO token provider for the YouTube bot check
- [`bluenviron/gortsplib`](https://github.com/bluenviron/gortsplib) — reference for RTSP semantics
- [`bluenviron/mediamtx`](https://github.com/bluenviron/mediamtx) — optional external normalisation layer
- [Frigate birdseye rawvideo pipe PR](https://github.com/blakeblackshear/frigate/pull/4761) — prior art for the approach we are avoiding
