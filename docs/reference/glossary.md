# Glossary

This project deliberately reuses the vocabulary of CCTV video management systems, digital signage, and broadcast production rather than inventing its own. The terms below already mean something specific to the people who will use this software, and adopting them costs nothing while making the UI and API immediately legible.

Where a term is ambiguous across vendors, this document picks one meaning and the implementation enforces it.

## Layout Terms

| Term | Meaning |
|------|---------|
| **Layout** | An arrangement of tiles across the output. Identified by name (`2x2`, `1+5`) and defined as normalised rectangles in the unit square. Some products call this a *view* (Milestone) or *monitor profile*. |
| **Tile** | One cell of a layout, holding at most one source at a time. Also called a *pane* or *cell*. Genetec and Milestone both use "tile". |
| **Cell index** | The tile's position within its layout. Index 0 is always the primary or hotspot tile where the layout has one. |
| **Hotspot** | The designated large tile in a `1+N` layout. Switching between hotspot layouts preserves the hotspot's source — standard VMS behaviour that users expect. |
| **Gutter** | The uniform spacing between tiles. |
| **Fit** | How a source's video rectangle is placed inside its tile: `contain`, `cover`, or `fill`. |
| **Mosaic** | A composed multi-source output. Broadcast term; FFmpeg also uses it for a tiled grid. Roughly synonymous with our *grid screen*. |
| **Multiviewer** | Broadcast hardware or software that composes many sources onto one display. This project is, in effect, a small software multiviewer. |
| **PIP** (picture-in-picture) | A small overlay inset over a full-screen source. Not in the initial scope; a `1+N` layout covers most of the same need. |
| **Salvo** | A group of cameras switched onto a bank of monitors simultaneously. Older matrix-switcher terminology, still found in NVR documentation. Out of scope — this project drives one display. |

### Layout Families

The grouping used by the layout picker, following [Vivotek VAST](https://www.vivotek.com/en-US/resource/download_center/product/download/13606) and comparable products:

| Family | Examples | Notes |
|--------|----------|-------|
| **Full bleed** | `full` | One source filling the output. The only single-cell layout. |
| **Equal** | `2x2`, `3x3`, `4x4`, `2x1`, `1x2` | Uniform grid. `2x2` is often called a *quad*. |
| **Focus** / **Hotspot** / **1+N** | `1+3`, `1+5`, `1+7`, `1+12` | One large tile plus smaller ones. |
| **Vertical** | `3v`, `1v+6` | For portrait displays or portrait sources. |
| **Panoramic** | `2p`, `1p+6` | For wide or stitched sources. |

Fisheye dewarp modes (`1O`, `1R`, `1P`, `4R`) are standard in VMS products but out of scope here.

## Scheduling Terms

Three distinct concepts that VMS products keep separate, and so do we. Conflating them is the most common source of confusion in this domain.

| Term | Scope | In this project |
|------|-------|-----------------|
| **Sequence** (or **carousel**) | Sources cycling **within a single tile** | `tile.sequence` on a grid screen |
| **Playlist** | Sources cycling **as the full output** | `items[]` on a transition screen |
| **Tour** | Whole **layouts/screens** cycling | `tour.entries[]` |

Genetec defines a [camera sequence](https://techdocs.genetec.com/r/en-US/Security-Center-Administrator-Guide-5.12/About-camera-sequences) as "a list of cameras displayed one after another in a rotating fashion within a single tile" — exactly our tile sequence. FLIR's United VMS documents "layout tours" for the screen-level equivalent. "Tour" is also used for PTZ preset patrols in some products, which is unrelated.

| Term | Meaning |
|------|---------|
| **Dwell time** | How long each step is displayed before advancing. The universal term across every vendor — use it rather than "hold duration" or "interval". In this project, dwell time **excludes** transition duration. |
| **Pin** | Hold on one screen and suspend the tour. `POST /api/v1/display/goto/:screenId` pins. |
| **Take** | Execute a transition immediately, manually. From vision-mixer terminology. |
| **Auto-transition** | Execute a transition over its configured duration. The normal case. |

## Transition Terms

Transition names follow **SMPTE 258M** as codified by the [W3C SMIL 3.0 Transition Effects Module](https://www.w3.org/TR/smil/smil-transitions.html), which defines a two-level type/subtype taxonomy. The same vocabulary appears in [GStreamer's `smpte` element](https://gstreamer.freedesktop.org/documentation/smpte/smpte.html), GStreamer Editing Services, and [DirectShow's SMPTE wipe table](https://learn.microsoft.com/en-us/windows/win32/directshow/smpte-wipe-transition).

| Type | Subtypes | SMPTE codes |
|------|----------|-------------|
| `barWipe` | `leftToRight`, `topToBottom` | 1, 2 |
| `boxWipe` | `topLeft`, `topRight`, `bottomRight`, `bottomLeft` | 3–6 |
| `barnDoorWipe` | `vertical`, `horizontal` | 21, 22 |
| `pushWipe` / `slideWipe` | directional | 301+ |
| `fade` | `crossfade`, `fadeToColor`, `fadeFromColor` | — |

The masked wipes — `irisWipe` (101), `ellipseWipe` (119–121), and `clockWipe` (201–204) — are part of the vocabulary but are **not implemented and not offered**. They require a fragment shader; see [Display Pipeline](../architecture/display-pipeline.md). The names are reserved so they can be reinstated without a vocabulary change.

### Definitions We Enforce

| Term | Meaning |
|------|---------|
| **Cut** | Instantaneous, zero-duration switch. The default and the cheapest. |
| **Dissolve** / **crossfade** | Both sources are simultaneously visible during the transition. Film and video editing prefer *dissolve*; audio and web prefer *crossfade*. We use `fade` / `crossfade`. |
| **Fade** (proper) | To or from a solid colour — *fade to black* (FTB) being the common case. Distinct from a crossfade, though vendors often blur the two. Our `fadeToColor` and `fadeFromColor` cover this. |
| **Wipe** | A moving geometric boundary reveals the incoming source. |
| **Push** | The outgoing image is **shoved off-screen** by the incoming one. Both layers move. |
| **Slide** | The incoming image **moves over a stationary** outgoing one. Only the incoming layer moves. |
| **DVE** (Digital Video Effect) | Broadcast term for scale, position, and rotate moves. A tile-to-fullscreen animation is technically a DVE. |

Push and slide are frequently confused by vendors, which is why they are defined explicitly here and implemented distinctly.

### Easing

Follows the [CSS easing function](https://developer.mozilla.org/en-US/docs/Web/CSS/easing-function) standard: `linear`, `ease`, `ease-in`, `ease-out`, `ease-in-out`, and explicit `cubic-bezier(x1, y1, x2, y2)`. Named presets are stored as their control points so the model stays portable and the UI can draw the same curve the engine evaluates.

## Video Pipeline Terms

| Term | Meaning |
|------|---------|
| **Source** | Anything that produces video frames: a stream URL, a camera, or an uploaded file. |
| **Demux** | Splitting a container into elementary streams. Handled by libavformat. |
| **Depacketise** | Reassembling RTP packets into access units. Handled inside libavformat for RTSP. |
| **NV12** | Packed 4:2:0 we upload after hardware download: one full-resolution Y plane plus one half-resolution interleaved UV plane. |
| **I420** / **IYUV** | Planar 4:2:0 (Y, U, V). Software H.264 usually decodes to this; we upload it with `SDL_UpdateYUVTexture` instead of converting to NV12. |
| **SAND** | Broadcom's 128-byte-column-tiled frame format, emitted by the Raspberry Pi hardware decoder. Also seen as "NC12". Must be detiled or handled with DRM format modifiers. See [Hardware Decode](../architecture/hardware-decode.md). |
| **DMA-BUF** / **DRM PRIME** | Kernel mechanism for sharing GPU buffers between devices without copying. The basis of any zero-copy path. |
| **Zero-copy** | Keeping a decoded frame in GPU memory from decoder to display, never touching system memory. Possible on the Pi but not through SDL's public API. |
| **Stateful V4L2 M2M** | Kernel decode API where the driver maintains bitstream state. Pi 4 H.264. FFmpeg's `h264_v4l2m2m`. |
| **Stateless V4L2 request** | Kernel decode API where the client supplies per-frame controls and reference lists. Pi HEVC (`rpivid`). Reached through FFmpeg's `drm` hwaccel, **not** through `hevc_v4l2m2m`, which cannot work. |
| **KMSDRM** | Kernel Mode Setting / Direct Rendering Manager. Lets an application drive the display with no X11, Wayland, or compositor. |
| **DRM master** | Exclusive control of a display device. Required for KMSDRM output, so nothing else may own the display. |
| **Frame slot** | Our bounded single-producer/single-consumer handoff between a decoder and the renderer. Queues a few frames so the renderer can schedule presentation, and evicts the oldest when full. |
| **Presentation clock** | Per-source mapping from a stream's media time (PTS) to the wall clock. Decides which queued frame belongs on screen at the next refresh, which is what keeps cadence stable on moving content. |
| **Buffer** | How far behind live a segmented source (YouTube, HLS, DASH) starts, in seconds (`options.buffer_ms`). A jitter buffer: more delay, smoother playback. RTSP defaults to 0. |
| **VideoToolbox** | Apple's hardware decode/encode API. On macOS, H.264 decode uses the generic libav decoder plus a VideoToolbox device context (`h264_videotoolbox` is an encoder name). Cannot start mid-GOP; livestream-viewer waits for a keyframe. See [Hardware Decode](../architecture/hardware-decode.md#macos--videotoolbox). |
| **YouTube cookies** | A Netscape cookie jar uploaded on Stream Sources (or exported via `--cookies-from-browser` / `./scripts/export-youtube-cookies.sh`) and passed to `yt-dlp --cookies`. Needed when YouTube still treats the host as a bot after a PO token, and for private, members-only, or age-restricted videos. Stored at `data/secrets/youtube.cookies`, never returned by the API. |
| **PO token provider** | Local BgUtils HTTP server that mints YouTube proof-of-origin tokens for yt-dlp. Used for the public-livestream bot check. `auto` starts a managed Deno server when present; otherwise it keeps pinging a sidecar on `127.0.0.1:4416`. A running provider is a health check, not a guarantee. |
| **bgutil plugin** | yt-dlp PO token plugin **embedded in the Go binary** and copied to `data/yt-dlp-plugins/bgutil/` at startup. Homebrew yt-dlp does not ship it; without `--plugin-dirs` a running HTTP provider is ignored. GPL-3, separate from the MIT Go binary. |
| **Vsync** | Synchronising presentation to the display's refresh. Paces the render loop. |

## Terms We Avoid

| Avoid | Use instead | Why |
|-------|-------------|-----|
| "Slot" for a layout cell | **Tile** | "Slot" is taken by the frame slot in the video pipeline |
| "Scene" for a screen | **Screen** | "Scene" is the compositor's per-frame draw list |
| "Rotation" for a tour | **Tour** | "Rotation" is display orientation |
| "Interval" or "hold time" | **Dwell time** | The vendor-standard term |
| "Transition" for a screen kind and for the effect | Context distinguishes them, but prefer **transition screen** for the kind | The effect is what `transition` means everywhere else |
| "Channel" | **Source** | "Channel" means something specific and different in NVR products |
