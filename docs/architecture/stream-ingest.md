# Stream Ingest

> **Status:** Design. Not yet implemented.

This document covers the path from a configured source to a frame sitting in a slot ready for the renderer: URL resolution, demuxing, decoding, the frame handoff, and failure handling.

## Library Choice

### In-Process libav, Not an FFmpeg Subprocess

Almost every Go project in this space shells out to the `ffmpeg` binary. [go2rtc](https://github.com/AlexxIT/go2rtc) is pure Go for protocol handling and spawns `ffmpeg` for anything needing a codec. [MediaMTX](https://github.com/bluenviron/mediamtx) deliberately does not decode at all. [Frigate](https://github.com/blakeblackshear/frigate)'s birdseye multi-camera view — the closest functional analogue to this project — composites in Python and pushes rawvideo through a named pipe.

The subprocess approach buys real things: crash isolation, trivial upgrades, no cgo, and the ability to swap in the Raspberry Pi FFmpeg fork without recompiling. It is genuinely the right call for many projects.

It is the wrong call here, for one arithmetic reason. Raw NV12 at 1080p30 is approximately **93 MB/s per stream** through a pipe, and every byte is a decoder→kernel→process copy. A 3×3 wall is roughly 840 MB/s of pure copying, which saturates Raspberry Pi memory bandwidth before a single pixel is composited. Frigate's birdseye CPU cost is a well-known complaint and is good evidence for the alternative. It also adds at least a frame of buffering per hop, and it permanently forecloses zero-copy.

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
yt-dlp -g --no-warnings -f "best[protocol*=m3u8]" "<url>"
```

Three rules:

1. **`yt-dlp` is discovered on `PATH`, never bundled.** It releases constantly, and shipping it inside our binary or container is neither practical nor appropriate. If it is absent, YouTube sources are marked unavailable with a message telling the user how to install it. Everything else keeps working.
2. **Resolved URLs are time-limited and IP-bound.** The returned URL carries `expire`, `ip`, and `sig` parameters. Treat resolution as a periodically-refreshed operation, not a value stored in the database. Re-resolve on every reconnect and on a timer well inside the expiry window.
3. **Resolution has a timeout and a circuit breaker.** A hung `yt-dlp` must not wedge the source worker. Cap the subprocess with a context deadline and back off after repeated failures.

YouTube's Terms of Service prohibit accessing content by means other than the Service's own interface. Treating `yt-dlp` as a user-supplied external tool is both the practical and the appropriate posture; this is documented in the UI where YouTube sources are added.

### RTSP

Handed to libavformat. **Transport defaults to TCP** (`rtsp_transport=tcp`): interleaved TCP is dramatically more reliable than UDP on a congested or wireless network, and the added latency is negligible for a wall. UDP is available per source for users who need the last few milliseconds.

Credentials go in the source record and are injected into the URL at open time so they never appear in logs.

### HLS, DASH, MPEG-TS, SRT, RTMP

Passed straight to libavformat with a sensible timeout and reconnect options. DASH is the weakest area in pure Go, which is another reason to let libavformat own it.

### Files

Uploaded through `POST /api/v1/uploads` into the data directory. Played on loop by seeking to the start on EOF. Useful for idle cards and offline fallbacks.

## Decode Worker

One worker per active source, each pinned to its own OS thread with `runtime.LockOSThread()` because libav calls are cgo and long-running — leaving them on a shared goroutine scheduler thread starves the rest of the runtime.

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
    send packet to decoder
    receive frames:
        if hardware-backed: transfer to system memory as NV12
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
- V4L2 M2M devices (`/dev/video10` and neighbours on a Pi 4)
- V4L2 stateless request devices (`/dev/video19` + `/dev/media2`, `rpivid`)
- Which FFmpeg hwaccels and decoders the linked libav actually offers
- VA-API on development machines

Per source, the worker then picks the best available path for the stream's codec, and records the decision on the source's probe record so the UI can show it. If no hardware path exists, it falls back to software decode and says so — silently dropping frames while the user believes they have acceleration is the failure mode this design most wants to avoid.

A configurable cap limits concurrent hardware decoder instances, because the Pi's decoder blocks are a finite resource and exceeding them fails in confusing ways.

Details, including the Pi 4 versus Pi 5 divergence, are in [Hardware Decode](hardware-decode.md).

### Frame Conversion

Hardware-decoded frames on the Pi are not linear NV12 — they are Broadcom **SAND** tiled. The baseline path transfers them to system memory and detiles to linear NV12 (`hwdownload,format=nv12` in filter terms), which is exactly what other Pi 5 projects do. The cost is one copy plus a detile per frame per stream; at 1080p that is manageable on A76 cores.

Software-decoded frames arrive as YUV420P and are converted to NV12 so the renderer has one upload path. If profiling shows this conversion mattering, the renderer can grow an IYUV path via `SDL_UpdateYUVTexture`; the frame interface already carries the plane count and format.

## Frame Handoff

The decoder and the renderer are connected by a **frame slot**: a triple-buffered, lock-free, single-producer/single-consumer handoff.

- The decoder writes into whichever of the three buffers is free, then atomically publishes it as "newest".
- The renderer atomically takes "newest" at each vsync. If nothing new has arrived, it reuses the texture it already uploaded.
- The third buffer guarantees the producer always has somewhere to write without waiting for the consumer.

**There is no queue, and that is deliberate.** A queue lets the renderer fall behind and never catch up, and it converts a transient decode stall into permanent added latency. Dropping old frames is the correct behaviour for a viewer: freshness beats completeness. The slot also gives the renderer a hard non-blocking guarantee — it can never be stalled by a decoder.

Frame buffers are pooled per source and sized on first frame. `AVFrame` and `AVPacket` objects must be explicitly unreferenced; every allocation site pairs with a `defer` and the pool is the only owner of long-lived buffers.

See [Concurrent State Pattern](../patterns/concurrent-state-pattern.md).

## Failure Handling

Failures are per-source and never propagate. A dead camera must not disturb the other eight tiles.

| Condition | Response |
|-----------|----------|
| Connect fails | Exponential backoff with jitter, capped at a configurable ceiling |
| Stream drops mid-play | Same backoff; YouTube sources re-resolve their URL first |
| Decode error on a packet | Skip the packet, count it; repeated errors within a window trigger a decoder reset |
| Hardware decoder unavailable at open | Fall back to software, record the downgrade on the source status |
| No frames for the grace period | Renderer swaps the tile to an offline placeholder; worker keeps retrying |
| Resolution or format change | Publish the new geometry with the frame; renderer recreates its texture |
| File EOF | Seek to start (loop) |
| Repeated resolution failure | Circuit-break, mark the source failed, surface the reason in the UI and over SSE |

Every state change is published over Server-Sent Events (`GET /api/v1/events`) so the Preview page reflects reality without polling.

## Probing

`POST /api/v1/sources/:id/probe` opens the source, reads enough to determine codec, resolution, frame rate, and pixel format, decodes one frame for a thumbnail, and closes. The result is cached on the source record.

Probing is what makes the Settings UI honest: it can tell the user, before they build a 3×3 grid, that six of their cameras are H.264 and this is a Pi 5.

## References

- [`asticode/go-astiav`](https://github.com/asticode/go-astiav)
- [`yt-dlp`](https://github.com/yt-dlp/yt-dlp)
- [`bluenviron/gortsplib`](https://github.com/bluenviron/gortsplib) — reference for RTSP semantics
- [`bluenviron/mediamtx`](https://github.com/bluenviron/mediamtx) — optional external normalisation layer
- [Frigate birdseye rawvideo pipe PR](https://github.com/blakeblackshear/frigate/pull/4761) — prior art for the approach we are avoiding
