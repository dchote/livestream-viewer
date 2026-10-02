# 0005: Orange Pi 4 Pro

## Status: Documented

Panel output on this board already works, and the probe correctly reports software decode. A Cedar spike on kernel `6.6.98-8wi-os` produced linear H.264 YUV and one interrupt per frame. HEVC aborted, so the gate failed and no decoder was added.

## Summary

The Orange Pi 4 Pro is an Allwinner A733 (`sun60iw2`), not a Rockchip board. On 8WI OS the Home Assistant add-on already composites to HDMI through the existing KMSDRM path, and the capability probe correctly reports software decode for H.264 and HEVC. The Video Engine is present in the kernel (`sunxi_ve`, `/dev/cedar_dev`) but nothing in the add-on image can drive it: there is no V4L2 decoder, no VA-API driver, and no `libvdecoder`.

**Done when:** the architecture, product, glossary, add-on, and troubleshooting docs name this host and its software-decode limit, and the Cedar spike result is recorded. Both are done. HEVC did not produce frames, so the decode-method follow-up stays closed.

## What was measured

Checked over SSH on `172.20.9.84` (8WI OS 1.0.1-dev, kernel `6.6.98-8wi-os`, add-on image 0.1.6):

- CPU: 2× Cortex-A76 at 2.0 GHz and 6× Cortex-A55 at 1.8 GHz. This unit has 3.8 GiB RAM (4 GB SKU).
- Device-tree model is `sun60iw2`. Compatible string is `xunlong,orangepi-4-pro`. The friendly name is in host `/etc/os-release` (`CATALOG_BOARD_ID=orangepi_4_pro`, `SUPERVISOR_MACHINE=orangepi-a733`).
- Display: `/dev/dri/card0` is `sunxi-drm` with HDMI-A-1 connected (25 modes). `/dev/dri/card1` is Imagination PowerVR (`pvrsrvkm`) with no connectors. The add-on logs card0 as the usable output. Preview has served MJPEG, so compositing is already working. Do not pin `SDL_KMSDRM_DEVICE_INDEX`.
- Decode device: `sunxi_ve` exposes `/dev/cedar_dev` (decode) and `/dev/cedar_dev_ve2` (encode), both mode 600 root. No `/dev/video*`, no `/dev/media*`. The add-on does not receive `cedar_dev`. Its FFmpeg 8 hwaccels are `vaapi` and `drm`, plus `*_v4l2m2m` wrappers with no device behind them. Probe: `board="" h264_hw=false hevc_hw=false`.
- The container's `/proc/device-tree` symlink targets `/sys/firmware/devicetree/base`, which is not mounted, so `board_model` stays empty. That is a missing mount, not a failed probe. Do not invent a model string.

## Scope Boundaries

**In this pass**

- Document the board, the working display path, and why FFmpeg stays on software.
- Name Cedar / Video Engine in the glossary.
- State capacity guidance: software H.264 on two A76 cores and 4 GB is a small number of 1080p streams, not a 3×3 wall. Prefer camera substreams. HEVC in software is heavier.
- Record the Cedar spike in [hardware-decode.md](../architecture/hardware-decode.md). H.264 produced linear YUV. HEVC aborted, so no decoder was added.

**Explicitly out**

- Claiming `drm` or `v4l2m2m` hardware because `/dev/dri` exists.
- `cedarzcdec ! kmssink` or any single KMS plane. A wall composites many tiles in SDL.
- OMX (`omxh264dec`). On A733 its DMA-BUF negotiation is broken.
- Vendoring Allwinner `.so` files into the public image before license and ABI are confirmed.
- Passing `/dev/cedar_dev` into the add-on before a userspace decoder exists.
- Changing probe selection, OpenAPI, the display strategy, or the layout catalogue in this pass.

## Cedar spike (gate)

Throwaway `vdecoderdemo` from Radxa `libcedarc-dev_2.0.0_arm64.deb` (version 1.0.7), not an ingest backend. The libraries were not committed. Detail is in [Hardware Decode](../architecture/hardware-decode.md#cedar-spike).

| Check | Result |
|-------|--------|
| H.264 linear YUV | Pass. 59 frames, YUV420P, 1920×1088, byte size matches planar 4:2:0. |
| HEVC frames | Fail. glibc `sysmalloc` abort (exit 133) on two runs. Partial output, not a frame. |
| `cedar_dev` IRQ | Pass for H.264: +59, one per displayed frame. HEVC: +2 per attempt. Encoder IRQ stayed 0. |
| Sessions | 2, 4, and 8 concurrent H.264 processes all finished. No wedge at 8. Eight-wide took about 3 s for 20 frames. |
| CPU | Same synthetic 1080p clip: Cedar about 1 s for 59 frames; FFmpeg 8 software 0.974 s wall for 60 frames. The clip is `testsrc`, not a camera. |

The gate required both codecs. HEVC failed, so there is no `cedar` decode method, no OpenAPI change, and no `/dev/cedar_dev` in the add-on.

## Cross-Cutting Contracts

| Contract | Action in this change |
|----------|------------------------|
| `api/openapi.yaml` | Unchanged. The spike did not pass, so no `cedar` path string. |
| `docs/reference/glossary.md` | Add Cedar / Video Engine |
| Display strategy / layout catalogue | Unchanged |
| Password fields | Unchanged |

## Related

- [Hardware Decode](../architecture/hardware-decode.md)
- [0004: Ingest and Display Engine](0004-ingest-and-display-engine.md)
