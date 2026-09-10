# Hardware Decode

> **Status:** Implemented for VideoToolbox (macOS: keyframe wait, no `LOW_DELAY` on hardware, `hwaccel_flags` for profile/level) and best-effort VA-API/DRM/V4L2 enumeration on Linux. The Home Assistant add-on is confirmed working on Raspberry Pi. Pi capacity numbers remain qualitative.

livestream-viewer is platform-agnostic: it probes the host at startup and reports what each codec can do. **This document is the optimisation and capacity-planning guide for constrained Linux hosts**, with Raspberry Pi 4/5 as the primary worked example because their V4L2 surface is unusually sharp-edged. Other SBCs and desktop GPUs follow the same probe → report → fall back pattern; only the device nodes and hwaccel names change.

## Why this matters on low-cost hardware

On a workstation with VA-API or VideoToolbox, a 3×3 grid of 1080p H.264 is rarely a crisis. On a Raspberry Pi, Rockchip board, or similar ARM SBC, decode is often the binding constraint. The application must never silently fall back to software decode and drop frames — it probes, records, and surfaces capacity so the UI can warn before the wall is built.

## Raspberry Pi — headline facts

The Pi is the most constraint-laden optimisation target today, and the constraints changed significantly between the Pi 4 and the Pi 5:

1. **The Raspberry Pi 5 has no H.264 hardware decoder.** The block was removed from BCM2712. Only HEVC remains.
2. **H.264 is what most real sources use** — YouTube, most IP cameras, most web streams. So on a Pi 5, the common case is software decode.
3. **Upstream FFmpeg cannot produce zero-copy DMA-BUF frames from V4L2 on the Pi.** That requires an out-of-tree fork, and upstreaming is considered unlikely by the people closest to it.
4. **The Pi's decoder does not emit linear NV12.** It emits Broadcom SAND, a tiled format that must be detiled or handled with DRM format modifiers.

None of these is fatal. All of them need to be in the design rather than discovered during a demo. Other embedded platforms have their own equivalents (Rockchip MPP, Allwinner Cedar, AMD/Intel VA-API quirks); the Pi notes below are the template for documenting those as they are validated.

## Per-model capability (Raspberry Pi)

| | Pi 4 (BCM2711) | Pi 5 (BCM2712) |
|---|---|---|
| H.264 decode | Hardware, 1080p-class | **None** — software on 4× Cortex-A76 |
| HEVC decode | Hardware (`rpivid`) | Hardware, 4K60 |
| VP9 / AV1 | Software | Software |
| H.264 interface | Stateful V4L2 M2M, `/dev/video10` (`bcm2835-codec`) | n/a |
| HEVC interface | Stateless V4L2 request, `/dev/video19` + `/dev/media2`, needs `dtoverlay=rpivid-v4l2` | Stateless V4L2 request, `/dev/video19` |

Raspberry Pi engineers defend the removal on the grounds that the A76 cores decode H.264 in software faster than the Pi 4's hardware block could, and that the block "was in the Pi 1 and was designed over 15 years ago" ([forum thread](https://forums.raspberrypi.com/viewtopic.php?t=391283)). Jeff Geerling's [testing](https://www.jeffgeerling.com/blog/2024/can-raspberry-pi-5-handle-4k/) corroborates: 4K60 HEVC is "butter smooth," 4K60 H.264 is "watchable but with small stutters."

For a single stream that is fine. For a 3×3 grid of 1080p H.264 cameras it is not, and the UI needs to say so before the user builds it.

**Practical guidance we surface to users on Pi hardware:** if you control the encoder, use HEVC. It is the only codec with hardware decode on the Pi 5 and it works on the Pi 4 as well.

## The Two Kernel APIs (V4L2 on Linux SBCs)

This is the most common source of confusion in Pi video work, and it produces a specific recurring bug report. Similar stateful/stateless splits appear on other V4L2 platforms.

- **Stateful V4L2 M2M** — The decoder maintains bitstream state. FFmpeg talks to this with the `h264_v4l2m2m` decoder. Pi 4 H.264 only.
- **Stateless V4L2 request API** — The kernel driver is given per-frame controls and reference lists; the client maintains state. Used by `rpivid` for HEVC on both models. FFmpeg reaches this through the `drm` hwaccel, not through a `*_v4l2m2m` decoder.

**`hevc_v4l2m2m` cannot work on the Pi.** A Raspberry Pi engineer states directly that "there is NOT a simple mapping between the two" ([forum](https://forums.raspberrypi.com/viewtopic.php?start=25&t=283301)). A large fraction of "hardware decode is using 200% CPU" bug reports across projects are exactly this mistake.

Correct invocation for HEVC on a Pi 5 is:

```
-hwaccel drm -hwaccel_output_format drm_prime
```

The capability prober must therefore distinguish the two interfaces explicitly rather than pattern-matching on decoder names.

## FFmpeg: Which Build

Upstream FFmpeg has **no V4L2-request (stateless) support at all**, and its `v4l2m2m` support copies frames to system memory rather than exporting DMA-BUFs. The out-of-tree fork is [`jc-kynesim/rpi-ffmpeg`](https://github.com/jc-kynesim/rpi-ffmpeg), configured with `--enable-sand --enable-v4l2-request --enable-libdrm`.

Two things follow:

- **Plan around the fork permanently.** mpv's maintainers judge upstreaming ["unlikely… we're likely to remain in this situation for the indefinite future"](https://github.com/mpv-player/mpv/wiki/V4L2-drmprime-support).
- **Pin versions.** The fork tracks kernel ABI changes, so kernel/FFmpeg skew produces breakage like [`Failed to find a V4L2 device for H265`](https://github.com/raspberrypi/linux/issues/6718).

One mitigating fact: the FFmpeg packaged in current Raspberry Pi OS reportedly already carries the hwaccel patches, so building the fork may not be necessary on the standard image. The capability prober checks what the linked libav actually offers rather than assuming.

## The SAND Problem

The Pi's decoder emits **SAND** (also seen as "NC12"), a 128-byte-column-tiled format. FFmpeg's fork exposes it as the pixel formats `rpi4_8`, `rpi4_10`, and `P030`, carrying `DRM_FORMAT_MOD_BROADCOM_SAND128` modifiers.

This is what makes zero-copy hard, in three separate ways:

1. **SDL3 has no DMA-BUF import API.** There is nothing in SDL's public surface analogous to `EGL_EXT_image_dma_buf_import`.
2. **Vulkan on the Pi cannot import it.** mpv 0.40 switched to Vulkan by default and Pi zero-copy broke with `DRM modifier 0x07 0xca804 not available for format r8`; the fix is forcing OpenGL ([mpv#16136](https://github.com/mpv-player/mpv/issues/16136)). Since SDL_GPU on Linux *is* Vulkan, SDL_GPU and Pi zero-copy are mutually exclusive. This is a primary reason we use `SDL_Renderer` — see [Display Pipeline](display-pipeline.md#why-sdl_renderer-and-not-sdl_gpu).
3. **DMA-BUF imports land on `GL_TEXTURE_EXTERNAL_OES`**, while SDL's GLES2 renderer expects `GL_TEXTURE_2D`. Bridging that requires importing the Y and UV planes as separate DMA-BUF planes and handling the external-OES sampler.

There *is* a hook: `SDL_CreateTextureWithProperties` accepts `SDL_PROP_TEXTURE_CREATE_OPENGLES2_TEXTURE_NUMBER` and `..._TEXTURE_UV_NUMBER`, which wrap existing GL texture names including the UV plane of an NV12 texture. So doing the EGL import ourselves and handing SDL the resulting texture names is plausible. It is not, however, something confirmed working in the wild, and it should be prototyped before anything is built on it.

## The Baseline Design

**Copy frames to system memory. Ship that. Optimise later.**

```
decode with hwaccel drm → DRM PRIME frame (SAND tiled)
  → hwdownload + format=nv12   (detile + copy to system memory)
  → SDL_UpdateNVTexture on an SDL_PIXELFORMAT_NV12 streaming texture

software H.264 (Pi 5)
  → YUV420P in system memory
  → pack into the frame-slot pool as I420
  → SDL_UpdateYUVTexture on an SDL_PIXELFORMAT_IYUV streaming texture
```

This is exactly what other Pi 5 projects do for hardware decode — the [homebridge-unifi-protect Pi 5 work](https://github.com/hjdhjd/homebridge-unifi-protect/issues/1318) uses the same `hwdownload,format=nv12` step. The cost is one memcpy plus a SAND→linear detile per frame per stream. Software H.264 skips the NV12 conversion and uploads planar 4:2:0. At 1080p on a Pi 5 this is very manageable, and it keeps us inside plain `SDL_Renderer` with no EGL code at all.

Critically, this decision is **reversible**. The frame delivery interface in `internal/frame` describes a frame as either system-memory planes or an opaque hardware handle, and the upload step in `internal/display/texture` is written against that interface. Zero-copy becomes a second implementation behind an existing seam rather than a rewrite.

## If We Pursue Zero-Copy

The reference implementation to read is SDL's own [`test/testffmpeg.c`](https://github.com/libsdl-org/SDL/blob/main/test/testffmpeg.c). It:

- Handles `AV_PIX_FMT_DRM_PRIME` and `AV_PIX_FMT_VAAPI`
- Builds an `EGLImage` via `EGL_LINUX_DMA_BUF_EXT` from the `AVDRMFrameDescriptor`
- Binds it with `glEGLImageTargetTexture2DOES`
- Passes DRM format modifiers when `EGL_EXT_image_dma_buf_import_modifiers` is available ([commit discussion](https://discourse.libsdl.org/t/sdl-testffmpeg-use-egl-ext-image-dma-buf-import-modifiers-extension/49566))
- Falls back to `GL_TEXTURE_EXTERNAL_OES` for tiled formats ([commit discussion](https://discourse.libsdl.org/t/sdl-testffmpeg-added-support-for-egl-oes-frame-formats/49780))

Steal its lesson from [SDL#14908](https://github.com/libsdl-org/SDL/issues/14908) too: a missing `eglDestroyImage` per frame leaked roughly 112 MB/s.

Reaching the DMA-BUF file descriptors from Go requires a small cgo shim casting `frame.Data()[0]` to `AVDRMFrameDescriptor`, since go-astiav does not wrap that struct.

## Capability Probing

At startup, once, the prober records:

| Probe | Method |
|-------|--------|
| Board model | `/proc/device-tree/model` |
| DRM devices | Enumerate `/dev/dri/card*`, `/dev/dri/renderD*` |
| V4L2 M2M devices | Enumerate `/dev/video*`, query `VIDIOC_QUERYCAP` for M2M capability |
| V4L2 request devices | Presence of `/dev/media*` paired with a stateless decoder |
| FFmpeg hwaccels | `av_hwdevice_iterate_types` via astiav |
| Decoders per codec | `avcodec_find_decoder_by_name` for the relevant candidates |
| VA-API | Development machines only |

The result is exposed at `GET /api/v1/system/info` and rendered in the UI, so a user can see "Pi 5 · HEVC: hardware · H.264: software" before designing a nine-tile wall.

## macOS / VideoToolbox

On a Mac the generic H.264 decoder plus a VideoToolbox device context is the hardware path (`h264_videotoolbox` is an encoder name). It works for files and for many livestreams. It is brittle on live RTSP:

1. **Join mid-GOP.** VideoToolbox cannot start on a P-frame. Feeding those pictures used to log `hardware accelerator failed to decode picture` until the next IDR. The decode worker (and the hardware probe) now wait for a keyframe before `SendPacket`.
2. **`AV_CODEC_FLAG_LOW_DELAY`.** That flag is for software live decode. Combined with VideoToolbox it yields `vt decoder cb: output image buffer is null` (`kVTVideoDecoderMalfunctionErr`). Hardware opens do not set it.
3. **Profile / level.** IP cameras often advertise High@L5.1 or Constrained Baseline that does not match the SPS. Opens pass `hwaccel_flags=+allow_profile_mismatch+ignore_level`.
4. **UniFi Protect and similar.** Many of those bitstreams still fail after a keyframe (changing PPS, SEI, long GOPs). Probe records `hw_decode` only if a hardware frame was actually produced; RTSP stays on software unless that probe succeeded.

Residual VT picture-failure logs are demoted so they do not drown the process log; a session that never publishes still falls back to software (`errHWUnusable`).

## Capacity Guidance

Rough expectations to communicate in the UI, to be replaced with measured numbers once there is something to measure:

| Scenario | Expectation |
|----------|-------------|
| Pi 5, HEVC 1080p, hardware | Many streams; decode is not the bottleneck |
| Pi 5, H.264 1080p, software | A small number of streams; scales with core count |
| Pi 5, 4K60 HEVC | One stream, comfortably |
| Pi 4, H.264 1080p, hardware | A few streams; the block is old and 1080p-class |
| Any model, upload bandwidth | The per-frame copy is proportional to resolution × frame rate × stream count |

Two levers reduce load substantially and should be offered: request a lower sub-stream from cameras that provide one (nearly all IP cameras do), and prefer HEVC where the source can be configured.

## References

- [Pi 5 H.264 decoder removal — Raspberry Pi forums](https://forums.raspberrypi.com/viewtopic.php?t=391283)
- [Stateful vs stateless V4L2 — Raspberry Pi forums](https://forums.raspberrypi.com/viewtopic.php?start=25&t=283301)
- [`jc-kynesim/rpi-ffmpeg`](https://github.com/jc-kynesim/rpi-ffmpeg)
- [mpv V4L2 DRM PRIME support notes](https://github.com/mpv-player/mpv/wiki/V4L2-drmprime-support)
- [mpv#16136 — Vulkan breaks Pi zero-copy](https://github.com/mpv-player/mpv/issues/16136)
- [SDL `test/testffmpeg.c`](https://github.com/libsdl-org/SDL/blob/main/test/testffmpeg.c)
- [Can the Raspberry Pi 5 handle 4K? — Jeff Geerling](https://www.jeffgeerling.com/blog/2024/can-raspberry-pi-5-handle-4k/)
