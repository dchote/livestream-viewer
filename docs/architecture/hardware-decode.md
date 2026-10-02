# Hardware Decode

> **Status:** Implemented for VideoToolbox (macOS: keyframe wait, no `LOW_DELAY` on hardware, `hwaccel_flags` for profile/level) and best-effort VA-API/DRM/V4L2 enumeration on Linux. The Home Assistant add-on is confirmed working on Raspberry Pi, and on Orange Pi 4 Pro (Allwinner A733) for panel output with software decode. Pi capacity numbers remain qualitative. Cedar hardware decode on A733 is not available through FFmpeg.

livestream-viewer is platform-agnostic: it probes the host at startup and reports what each codec can do. **This document is the optimisation and capacity-planning guide for constrained Linux hosts**, with Raspberry Pi 4/5 as the primary worked example because their V4L2 surface is unusually sharp-edged. Other SBCs and desktop GPUs follow the same probe → report → fall back pattern; only the device nodes and hwaccel names change.

## Why this matters on low-cost hardware

On a workstation with VA-API or VideoToolbox, a 3×3 grid of 1080p H.264 is rarely a crisis. On a Raspberry Pi, Rockchip board, or similar ARM SBC, decode is often the binding constraint. The application must never silently fall back to software decode and drop frames — it probes, records, and surfaces capacity so the UI can warn before the wall is built.

## Raspberry Pi — headline facts

The Pi is the most constraint-laden optimisation target today, and the constraints changed significantly between the Pi 4 and the Pi 5:

1. **The Raspberry Pi 5 has no H.264 hardware decoder.** The block was removed from BCM2712. Only HEVC remains.
2. **H.264 is what most real sources use** — YouTube, most IP cameras, most web streams. So on a Pi 5, the common case is software decode.
3. **Upstream FFmpeg cannot produce zero-copy DMA-BUF frames from V4L2 on the Pi.** That requires an out-of-tree fork, and upstreaming is considered unlikely by the people closest to it.
4. **The Pi's decoder does not emit linear NV12.** It emits Broadcom SAND, a tiled format that must be detiled or handled with DRM format modifiers.

None of these is fatal. All of them need to be in the design rather than discovered during a demo. Other embedded platforms have their own equivalents (Rockchip MPP, Allwinner Cedar, AMD/Intel VA-API quirks); the Pi notes below are the template for documenting those as they are validated. The Orange Pi 4 Pro section records one of those equivalents: the Video Engine is in the kernel, and the userspace that can drive it is not.

## Per-model capability (Raspberry Pi)

| | Pi 4 / CM4 (BCM2711) | Pi 5 (BCM2712) |
|---|---|---|
| H.264 decode | Hardware, 1080p-class (`h264_v4l2m2m`) | **None** — software on 4× Cortex-A76 |
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
H.264 on Pi 4/CM4 (h264_v4l2m2m, no DRM hwdevice)
  → YUV420P in system memory
  → SDL_UpdateYUVTexture on an SDL_PIXELFORMAT_IYUV streaming texture

HEVC (hwaccel drm) → DRM PRIME frame (SAND tiled)
  → hwdownload + format=nv12   (detile + copy to system memory)
  → SDL_UpdateNVTexture on an SDL_PIXELFORMAT_NV12 streaming texture

software H.264 (Pi 5, or any host without an H.264 path)
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
| V4L2 nodes | Enumerate `/dev/video*` |
| V4L2 M2M | `VIDIOC_QUERYCAP` for `V4L2_CAP_VIDEO_M2M(_MPLANE)`; sysfs name fallback (`bcm2835-codec-decode`, …) |
| Media controller | Enumerate `/dev/media*` (Pi HEVC / drm) |
| FFmpeg hwdevices | Create and free VideoToolbox / VA-API / DRM contexts (failures logged at debug) |
| Per-codec path | Choose `videotoolbox`, `vaapi`, `v4l2m2m`, or `drm` — never claim H.264 from DRM alone |

H.264 on Pi 4/CM4 uses **`h264_v4l2m2m`** with no DRM hwdevice context (system-memory YUV). HEVC uses the generic decoder plus a DRM device context when media/HEVC nodes are present. A Pi 5 has no H.264 block: `h264_v4l2m2m` may still be linked in FFmpeg, but without an M2M node the probe reports software.

The result is exposed at `GET /api/v1/system/info` (`h264_hw`, `hevc_hw`, `h264_path`, `hevc_path`, `v4l2_m2m`, …) and rendered in the UI.

**Operator override:** source option `force_software` skips hardware. Hardware is preferred whenever the host has a path for the codec; a stale `hw_decode=false` probe no longer locks V4L2/VA-API onto software. `force_hardware` still forces a VideoToolbox RTSP retry after a failed probe. The two force flags are mutually exclusive. Capability probing refreshes when `/dev/video*` (etc.) change.

**Concurrency:** Pi 4 / CM4 H.264 M2M is a single shared block. The ingest manager caps concurrent V4L2 M2M hardware sessions at one regardless of `max_hw_decoders`; remaining sources decode in software. `Session.Close` also bounds `Codec.Free` so a wedged driver cannot stall worker teardown indefinitely.

## macOS / VideoToolbox

On a Mac the generic H.264 decoder plus a VideoToolbox device context is the hardware path (`h264_videotoolbox` is an encoder name). It works for files and for many livestreams. It is brittle on live RTSP:

1. **Join mid-GOP.** VideoToolbox cannot start on a P-frame. Feeding those pictures used to log `hardware accelerator failed to decode picture` until the next IDR. The decode worker (and the hardware probe) now wait for a keyframe before `SendPacket`.
2. **`AV_CODEC_FLAG_LOW_DELAY`.** That flag is for software live decode. Combined with VideoToolbox it yields `vt decoder cb: output image buffer is null` (`kVTVideoDecoderMalfunctionErr`). Hardware opens do not set it.
3. **Profile / level.** IP cameras often advertise High@L5.1 or Constrained Baseline that does not match the SPS. Opens pass `hwaccel_flags=+allow_profile_mismatch+ignore_level`.
4. **UniFi Protect and similar.** Many of those bitstreams still fail after a keyframe (changing PPS, SEI, long GOPs). Probe records `hw_decode` only if a hardware frame was actually produced; RTSP stays on software unless that probe succeeded.

Residual VT picture-failure logs are demoted so they do not drown the process log; a session that never publishes still falls back to software (`errHWUnusable`).

## Orange Pi 4 Pro (Allwinner A733)

Measured on 8WI OS 1.0.1-dev, kernel `6.6.98-8wi-os`, add-on image 0.1.6. This board is not Rockchip. The SoC is Allwinner A733 (`sun60iw2`); the device-tree compatible string is `xunlong,orangepi-4-pro`. The device-tree model is only `sun60iw2`. The friendly name is in the host os-release (`CATALOG_BOARD_ID=orangepi_4_pro`).

| | Orange Pi 4 Pro (A733) |
|---|---|
| CPU | 2× Cortex-A76 @ 2.0 GHz + 6× Cortex-A55 @ 1.8 GHz |
| RAM on the unit checked | 3.8 GiB (4 GB SKU) |
| H.264 / HEVC decode | **Software.** FFmpeg has no device to open. |
| Display | `sunxi-drm` on `/dev/dri/card0`, HDMI-A-1. KMSDRM compositing works. |
| GPU node | Imagination PowerVR (`pvrsrvkm`) on `/dev/dri/card1`, no connectors |

SDL must keep scanning for a card with a connected panel. Pinning `SDL_KMSDRM_DEVICE_INDEX` to card1 would select the GPU node and fail the same way pinning card0 fails on a Pi. Card0 is the panel here, so the existing scan is the right behaviour.

### Why the probe stays on software

`chooseH264` / `chooseHEVC` only select VideoToolbox, VA-API, `h264_v4l2m2m`, or Pi-style DRM. This host has none of those. The kernel does have a Video Engine:

- Module `sunxi_ve` (GPL, "User mode CEDAR device interface"), vermagic matches this kernel.
- `/dev/cedar_dev` is the decoder. `/dev/cedar_dev_ve2` is the encoder. Both are mode 600, root. The add-on is not given `cedar_dev`.
- No `/dev/video*`, no `/dev/media*`, no VA-API driver. FFmpeg 8 in the add-on lists `vaapi`, `drm`, and `*_v4l2m2m` wrappers, and none of them have a device.

Presence of `/dev/dri` must not be reported as H.264 hardware. The probe already refuses that, and it must stay that way. Inside the add-on, `/proc/device-tree` points at `/sys/firmware/devicetree/base`, which is not mounted, so `board_model` is empty. That is a missing mount, not a failed codec probe. Do not invent a model string until the OS bind-mounts the device-tree or passes the machine name in.

Mainline Cedrus and libva-v4l2-request do not support A733. The userspace that can open `/dev/cedar_dev` is vendor libcedarc / `libvdecoder`. It is not on 8WI OS, not in the add-on image, and not an FFmpeg hwaccel. The package changelog marks part of that library closed source, so it is not committed here.

### What not to build

- A single KMS plane (`cedarzcdec ! kmssink` and the same idea). A wall composites many tiles in SDL. One scanout plane cannot.
- OMX (`omxh264dec`). On A733 its DMA-BUF negotiation is broken and the CPU fallback is the expensive path.
- Shipping the Allwinner `.so` files. H.264 did open this kernel's device (see the spike), and the package still marks libraries closed source. They stay out of the image and the repository.
- Mapping `/dev/cedar_dev` into the add-on while decode stays on FFmpeg. `devices: /dev/dri` plus `video: true` is enough for the panel.

A later hardware path, if both codecs can produce linear YUV, would keep libavformat for demux, call `libvdecoder` for the bitstream, and copy linear YUV into the frame slot. That is the same baseline as Pi 4 H.264 (system memory, not zero-copy). `cedar_dev_ve2` is the encoder and is irrelevant here.

### Cedar spike

Run on this board with Radxa `libcedarc-dev_2.0.0_arm64.deb` (package version 1.0.7, the t736/A733 build) and that package's `vdecoderdemo`. The container was given `/dev/cedar_dev`, `/dev/sunxi_soc_info`, and `/dev/dma_heap/system` plus `reserved`. The clip was two seconds of 1080p30 `testsrc`: a synthetic bitstream, easier than a camera, so the timings are not wall capacity.

**H.264 passed.** 59 displayed frames, pixel format YUV420P, geometry 1920×1088 (macroblock padding). The file was 184,872,960 bytes, which is 59 × 1920 × 1088 × 3/2. That is linear planar YUV, not LBC. The `cedar_dev` interrupt count rose by 59, one per displayed frame. `cedar_dev_ve2` stayed at 0. The library reported IC version `0x3331000021320` through this kernel's `sunxi_ve`, so the ioctl ABI matched for H.264. One session cost about 1 second for those 59 frames. Two, four, and eight processes at once all exited 0. At eight-wide, 20 frames took about 3 seconds each: the engine slowed and did not wedge. No failure count was found at or below eight, so this is not a Pi-style cap of one.

**HEVC did not.** The same tool, codec format 2, aborted twice with a glibc `sysmalloc` assertion during init (exit 133). The output file was a partial write, not a whole frame. The decoder interrupt rose by 2 on each attempt, not once per frame.

FFmpeg 8 in the add-on image decoded the same H.264 clip in software in 0.974 seconds of wall time (`utime` 1.46 seconds), about 2× realtime. That only shows the synthetic clip is cheap. It does not say how many camera streams the 4 GB board can hold.

The gate required both codecs to emit linear frames. HEVC did not, so there is no Cedar decode method, no new OpenAPI path string, and no `/dev/cedar_dev` in the add-on. Shipping decode stays on software.

### Capacity

Software H.264 on two A76 cores and 4 GB is a small number of 1080p streams, not a 3×3 wall. Use camera substreams. HEVC in software is the heavier case, and the hardware HEVC attempt above did not produce frames. The synthetic-clip timings are not a substitute for a camera measurement.

## Capacity Guidance

Rough expectations to communicate in the UI, to be replaced with measured numbers once there is something to measure:

| Scenario | Expectation |
|----------|-------------|
| Pi 5, HEVC 1080p, hardware | Many streams; decode is not the bottleneck |
| Pi 5, H.264 1080p, software | A small number of streams; scales with core count |
| Pi 5, 4K60 HEVC | One stream, comfortably |
| Pi 4, H.264 1080p, hardware | A few streams; the block is old and 1080p-class |
| Orange Pi 4 Pro, H.264 or HEVC 1080p, software | A small number of H.264 streams on 2× A76 and 4 GB; not a 3×3 wall. HEVC is heavier. Use substreams. |
| Any model, upload bandwidth | The per-frame copy is proportional to resolution × frame rate × stream count |

Two levers reduce load substantially and should be offered: request a lower sub-stream from cameras that provide one (nearly all IP cameras do), and prefer HEVC where the host has an HEVC hardware path (Pi 4 and Pi 5). On the Orange Pi 4 Pro, HEVC is software and is the heavier choice.

## References

- [Pi 5 H.264 decoder removal — Raspberry Pi forums](https://forums.raspberrypi.com/viewtopic.php?t=391283)
- [Stateful vs stateless V4L2 — Raspberry Pi forums](https://forums.raspberrypi.com/viewtopic.php?start=25&t=283301)
- [`jc-kynesim/rpi-ffmpeg`](https://github.com/jc-kynesim/rpi-ffmpeg)
- [mpv V4L2 DRM PRIME support notes](https://github.com/mpv-player/mpv/wiki/V4L2-drmprime-support)
- [mpv#16136 — Vulkan breaks Pi zero-copy](https://github.com/mpv-player/mpv/issues/16136)
- [SDL `test/testffmpeg.c`](https://github.com/libsdl-org/SDL/blob/main/test/testffmpeg.c)
- [Can the Raspberry Pi 5 handle 4K? — Jeff Geerling](https://www.jeffgeerling.com/blog/2024/can-raspberry-pi-5-handle-4k/)
- [A733 zero-copy hardware decoding — Armbian forums](https://forum.armbian.com/topic/61323-a733-zero-copy-hardware-decoding/)
- [Cedrus — linux-sunxi.org](https://linux-sunxi.org/Cedrus)
- [libcedarc 1.0.7 package used for the spike — radxa/allwinner-debian](https://github.com/radxa/allwinner-debian/tree/main/packages/arm64/libcedarc)
