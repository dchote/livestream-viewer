# Display Pipeline

> **Status:** Implemented. The display engine composites `strategy.Snapshot` onto an SDL3 window (or KMSDRM when that driver is selected). Linux panel verification is still deferred.

This document covers everything from a decoded frame to a lit pixel: SDL3 initialisation, the Linux KMS/DRM path for headless panels, windowed output on desktop hosts, texture management, compositing, transitions, and presentation. Raspberry Pi and other embedded boards are called out where their DRM or Mesa behaviour differs from a generic Linux workstation.

## SDL3 Binding Choice

`veandco/go-sdl2` is SDL2-only and will remain so — the maintainer [declined SDL3 support](https://github.com/veandco/go-sdl2/issues/618) and the README now redirects elsewhere. The two viable SDL3 bindings are:

| Binding | Approach | License | Notes |
|---------|----------|---------|-------|
| [`Zyko0/go-sdl3`](https://github.com/Zyko0/go-sdl3) | purego | MIT | Idiomatic Go (`error` returns, methods on `*Renderer`/`*Texture`), tracks SDL 3.4, also wraps SDL3_ttf/image/mixer, has a per-function `COVERAGE.md` |
| [`jupiterrider/purego-sdl3`](https://github.com/JupiterRider/purego-sdl3) | purego | Unlicense | Mirrors the C API closely, so SDL docs and C examples translate directly |

**We use `Zyko0/go-sdl3`.** It is the more actively maintained of the two and its idiomatic error handling fits the rest of the codebase. Both cover the required surface: `CreateTexture`, `UpdateNVTexture`, `UpdateYUVTexture`, `SetRenderTarget`, `SetTextureBlendMode`, `SetTextureAlphaMod`, and the NV12/IYUV pixel formats.

Two caveats to design around:

1. **Purego means no compile-time symbol checking.** A missing or renamed SDL function is a runtime load failure, not a build error. Probe the SDL version at startup and fail loudly with a clear message rather than crashing mid-render.
2. **Do not use the binding's embedded `libSDL3.so` blob.** `binsdl.Load()` embeds a prebuilt library that is not guaranteed to have KMSDRM compiled in. Use `sdl.LoadLibrary()` against the system SDL3, and verify at startup that the expected video driver (KMSDRM on headless Linux, or the platform default on a desktop) is present.

Neither binding is 1.0. Wrap SDL access behind `internal/display/output` so the binding is replaceable.

## Why SDL_Renderer and not SDL_GPU

SDL3 offers a modern `SDL_GPU` API alongside the traditional 2D `SDL_Renderer`. For this project the 2D renderer is the correct choice, for two independent reasons:

- **SDL_GPU on Linux is Vulkan-only.** There is [no OpenGL backend and there will not be one](https://github.com/libsdl-org/SDL/issues/13292). On Raspberry Pi that means going through Mesa's V3DV driver, which has been made to work under KMSDRM but requires hand-built Mesa and libdrm.
- **Some embedded Vulkan drivers cannot import the decoder's frame format.** mpv 0.40 switched to Vulkan by default and Pi zero-copy immediately broke with `DRM modifier 0x07 0xca804 not available for format r8`; the workaround is forcing `--gpu-api=opengl` ([mpv#16136](https://github.com/mpv-player/mpv/issues/16136)). SDL_GPU and Pi-style zero-copy are mutually exclusive today, so choosing SDL_GPU would foreclose the optimisation we most want to keep available on constrained boards.

SDL_Renderer is not a compromise here. As of **SDL 3.4.0** the 2D renderer gained real shader access through `SDL_CreateGPURenderer()`, `SDL_CreateGPURenderState()`, and `SDL_SetGPURenderStateFragmentUniforms()`, plus YUV texture and HDR colorspace support in that path. That is our escape hatch for masked transitions, used only where a transition genuinely needs a shader.

**Minimum SDL version: 3.4.0.** Earlier versions lack the GPU render state API.

## Output Initialisation

### Headless Linux (KMS/DRM)

Running without a desktop session means SDL talks directly to the kernel's DRM/KMS interface. This is the path used on Raspberry Pi OS Lite and other headless Linux installs with an attached panel.

```
SDL_VIDEO_DRIVER=kmsdrm
```

Note the name: SDL3 renamed SDL2's `SDL_VIDEODRIVER` to **`SDL_VIDEO_DRIVER`**. Using the old name silently produces a black screen — this caught a real user and required an SDL developer to diagnose ([SDL#12418](https://github.com/libsdl-org/SDL/issues/12418)).

Requirements on the device:

- SDL3 built with `-DSDL_KMSDRM=ON`, which needs libdrm and libgbm development packages present at configure time.
- `dtoverlay=vc4-kms-v3d` in `/boot/firmware/config.txt`. Add `,cma-512` for 4K output.
- `/dev/dri/card*` and `/dev/dri/renderD*` accessible — the process user must be in the `video` and `render` groups.
- **DRM master.** Nothing else may own the display. `SDL_HINT_KMSDRM_REQUIRE_DRM_MASTER` defaults to requiring it; relaxing the hint yields input handling but no rendering. In practice this means no desktop session, no other DRM client, and on Home Assistant OS it means the add-on must be the only thing touching the display.

Relevant hints exposed by the binding: `HintKmsDrmDeviceIndex` (select among multiple DRM devices), `HintKmsDrmRequireDrmMaster`, and `HintKmsDrmAtomic` (new in 3.4).

Two operational notes from the field: the KMSDRM driver requires the application to pump the SDL event queue or the screen stays black, and some Pi configurations need the `opengles` renderer forced over `opengl` because the Pi's desktop GL only advertises GLSL 1.30.

### Development Workstation

On macOS and desktop Linux, SDL selects `cocoa`, `wayland`, or `x11` automatically and opens a normal resizable window. The engine treats this identically — the only difference is that the window is not fullscreen and its size can change, which the layout solver already handles because it recomputes geometry from the current output size each frame.

`display.driver` in the bootstrap config overrides driver selection for testing.

### Startup Sequence

```
runtime.LockOSThread()          // main thread, held for the process lifetime
sdl.LoadLibrary()               // system SDL3, not the embedded blob
verify SDL version >= 3.4.0
apply hints (video driver, KMSDRM options, render vsync)
sdl.Init(sdl.InitVideo)
enumerate displays and modes → report through /api/v1/system/info
CreateWindowAndRenderer(...)    // fullscreen on Pi, windowed in development
verify renderer name and available texture formats
publish "display ready" state
enter render loop
```

If any step fails, the display plane reports the failure through the engine state snapshot and the process continues serving the API and UI. A Pi with a bad `config.txt` should still be reachable so the user can see why.

## Frame Presentation

### Texture Format

Decoded frames arrive as **NV12** (hardware download and remaining conversions) or **I420** (software 4:2:0). Each source owns one streaming texture in the matching SDL format (`SDL_PIXELFORMAT_NV12` or `SDL_PIXELFORMAT_IYUV`):

```
SDL_CreateTexture(renderer, format, SDL_TEXTUREACCESS_STREAMING, w, h)
```

Upload with **`SDL_UpdateNVTexture`** or **`SDL_UpdateYUVTexture`**, which take planes with independent pitches. Decoder `linesize[]` almost never equals width, so ingest packs to stride=width before the slot; the GPU then sees tight rows. Plain `SDL_UpdateTexture` cannot be used for either layout. The [SDL wiki](https://wiki.libsdl.org/SDL3/SDL_UpdateNVTexture) is explicit about this.

**Colorspace must be set explicitly.** YUV textures default to `SDL_COLORSPACE_JPEG` (full-range BT.601). Typical camera and streaming video is limited-range BT.709, so create the texture with `SDL_CreateTextureWithProperties` and pass `SDL_PROP_TEXTURE_CREATE_COLORSPACE_NUMBER` derived from the stream's reported colorspace. Getting this wrong produces washed-out or over-saturated video that is easy to miss and hard to diagnose later.

**Texture uploads are main-thread only.** This is the constraint that shapes the whole concurrency design — decode happens on worker threads, but the upload does not. See [Concurrent State Pattern](../patterns/concurrent-state-pattern.md).

### Texture Lifecycle

The cache holds **exactly one texture per source**, tagged with the geometry and colorspace it was created for. A resolution or colorspace change — which happens on adaptive HLS streams — destroys the old texture and creates a replacement under the same source ID.

That "one per source" rule is the important part. An earlier version keyed the map on `(source_id, width, height, format, colorspace, range)`, so every geometry a stream ever used left a texture behind for the life of the process, and the lookup returned whichever entry map iteration reached first — which after a resolution change could be the stale one, drawn at the wrong size.

Textures stay bound for every pre-rolled source, not only the tiles on the active screen. They are released on Unbind when the source is no longer in the strategy (or is disabled). Reconnects keep the last uploaded frame so the tile does not go black while libav re-opens the stream.

## Compositing

### Scene Assembly

Each frame the engine builds a **scene**: an ordered list of draw operations, resolved before any SDL call is made. A scene entry is:

```
{ texture, source_rect, dest_rect, alpha, clip_rect, blend_mode }
```

Building the scene as data first means the compositor is testable without a GPU, and the Preview page's client-side fallback can render the same geometry from the same layout numbers.

### Geometry

The layout solver produces normalised cell rectangles in the unit square. For each visible tile:

1. Scale the normalised cell to the current output resolution.
2. Inset by the configured gutter.
3. Fit the source's video rectangle inside the cell according to the tile's `fit`:
   - `contain` — Scale to fit, preserve aspect ratio, letterbox or pillarbox the remainder with the background colour.
   - `cover` — Scale to fill, preserve aspect ratio, crop the overflow via `source_rect`.
   - `fill` — Stretch to the cell, ignore aspect ratio.
4. Round to whole pixels. Sub-pixel tile edges produce shimmering seams on a static wall.

### Render Targets

Transitions and multi-layer composition use offscreen render targets. `SDL_SetRenderTarget` takes a texture created with `SDL_TEXTUREACCESS_TARGET`; passing `nil` returns to the window.

A useful property for a layered compositor: **viewport, clip rect, scale, and logical presentation are per-target and persist across target switches**. Each layer keeps its own state rather than needing re-setup on every switch.

Render targets are allocated once at output resolution and reused. There are at most three: `screenA`, `screenB`, and `preview`.

## Transitions

The key simplification: **a screen is a layer, whatever its kind.** A 3×3 grid and a single full-screen source both composite into a render target. A transition therefore operates on two textures and does not care what produced them. Transitioning between grid screens, between full-screen sources, and between a grid and a full-screen source are all the same code path.

```
compose(outgoing_screen) → screenA
compose(incoming_screen) → screenB
transition.Draw(renderer, screenA, screenB, easedProgress)
present
```

### Implementation Families

**Alpha** — `fade`. `SDL_SetTextureBlendMode(BLENDMODE_BLEND)` plus `SDL_SetTextureAlphaModFloat` on the incoming layer. `fadeToColor` dips through a solid colour in two halves, showing the outgoing screen on the way down and the incoming on the way up. `fadeFromColor` is the distinct SMIL variant: the frame starts on the colour and the incoming rises out of it, with the outgoing screen never shown.

**Geometric** — `pushWipe`, `slideWipe`, `barWipe`, `boxWipe`, `barnDoorWipe`. Pure destination-rect and clip-rect arithmetic; no shader, no mask. `push` translates both layers (the outgoing one is shoved off-screen); `slide` translates only the incoming layer over a stationary outgoing one. These are frequently confused by vendors, so the distinction is defined explicitly in the [Glossary](../reference/glossary.md) and enforced by the implementation.

**Masked** — `irisWipe`, `ellipseWipe`, `clockWipe`. **Withdrawn from the catalogue; not implemented.** These need a per-pixel alpha mask that geometric clipping cannot express, which means a fragment shader through `SDL_CreateGPURenderer` / `SDL_SetGPURenderState`. That was never written: the code advertised them, then drew a plain crossfade. Rather than keep offering effects that do not exist, they are gone from `GET /api/v1/transitions` and rejected by validation. Screens saved with them are migrated to `fade` on startup, and until they are, they render as a crossfade and report `masked_wipe_fade` in the engine's degradation list.

Reinstating them is a self-contained piece of work: add the shader, put the three types back in `Catalogue()`, and the existing tests will start requiring real implementations for each subtype.

### Progress and Easing

Progress is derived from the wall clock, never from a frame counter:

```
raw = clamp((now - transitionStart) / duration, 0, 1)
t   = easing(raw)
```

A frame-counted transition changes speed when the render loop drops frames, which is exactly when it is most visible. Easing is a cubic Bézier evaluated with Newton–Raphson, matching the CSS `cubic-bezier()` definition, with the named presets expanding to their standard control points.

Every transition implementation is a pure function of `t` and is unit-tested by asserting the scene it produces at `t = 0`, `0.5`, and `1` — no GPU required.

## Presentation and Pacing

`SDL_RenderPresent` with vsync enabled paces the loop. There is no separate frame timer and no sleep loop.

### Presentation timing

The loop runs on the display's clock, but *which* frame it shows is decided on the stream's. Each source carries a **presentation clock** that anchors its media timeline (PTS) to the wall clock. At each pass the renderer asks that clock which queued frame belongs on screen at the *next* refresh, discards anything older, and reuses the existing texture when nothing new is due.

The alternative — show whatever is newest at each vsync — was what shipped first, and it is wrong in a way that takes a while to notice. The decoder's clock and the panel's clock are unrelated, so the interval between "frame becomes newest" and "renderer looks" drifts continuously. A 30 fps source on a 60 Hz output holds for two refreshes, then three, then two, and loses a frame entirely whenever two publishes land inside one refresh. On a fixed camera this is genuinely invisible: a repeated or missing frame is the same pixels. Point the camera at something moving and it reads as judder, which is why it surfaced on a YouTube livestream and never on the RTSP wall.

With media time driving the choice, the mapping is linear and the cadence follows from the ratio:

- 30 fps on 60 Hz holds every frame for exactly two refreshes.
- 25 fps on 60 Hz alternates two and three in a fixed repeating pattern rather than wandering.
- 60 fps on 60 Hz presents one for one.
- A stalled or reconnecting stream holds its last frame for as long as the source is still needed. The placeholder appears only when there has never been a frame, or after the source is unbound.

Different sources at different frame rates coexist with no special handling, because each tile has its own clock.

Three details make it hold up:

- **Lead.** The clock anchors 50 ms behind the arriving stream. Without that the renderer presents each frame the moment it is decoded, the queue never has anything in it, and the decode thread's timer jitter lands straight on the screen.
- **Slew.** Encoder and panel clocks differ by tens of parts per million, which left alone accumulates into a dropped or starved frame. The anchor is nudged by a millisecond per refresh when the queue strays from its working depth, spreading the correction below the threshold of visibility.
- **Resync.** A reconnect, a file loop, or a stall long enough that the anchor no longer describes the stream drains the queue and re-anchors at the newest frame, rather than replaying a backlog at once.

The refresh interval comes from `SDL_GetCurrentDisplayMode`, re-read once a second so that dragging the window to a different panel is picked up. Timing the render loop instead would fold our own overruns into the estimate, and an engine that fell behind would conclude the panel had slowed down and start scheduling against a refresh that does not exist. Loop timing is kept only as a fallback for drivers that report no mode, and the rational `RefreshRateNumerator/Denominator` form is preferred so 59.94 Hz does not accumulate error.

### A grid of mixed frame rates

The clocks are per source and deliberately independent. There is no common timebase between a YouTube stream and an RTSP camera — separate encoders, separate network paths, separate drift — and genlocking them would mean dropping or repeating frames on every source but one. Each tile therefore runs on its own clock, and the display is the master only for *presentation*. A 60 Hz wall showing 60, 30, 25 and 15 fps tiles gives each of them its own stable cadence: 1:1, 2:2, a fixed 2/3 pattern, and 4:4.

What the sources *do* share is the render pass, and that is where a grid can bite:

- **Convergent uploads.** Nothing stops several sources falling due on the same refresh. Nine 1080p tiles landing together is ~28 MB of texture upload in one iteration and near-zero on the next; if the heavy iteration overruns, the loop misses a vblank and *every* tile hitches at once — far more noticeable than any single stream's timing. Each source is therefore anchored one refresh further along than the last, cycling every four, which spreads the work without any source having to wait.
- **Per-frame cost.** Steady state draws straight to the backbuffer. Composing through two offscreen render targets — needed only to blend a transition, or to give the preview something to downscale — costs three full-screen writes per frame instead of one, and that headroom is what decides whether a full wall holds the refresh.
- **Missed vblanks are reported.** A loop period more than 1.5× the refresh interval counts as a dropped frame in the display state, so a wall that is GPU-bound shows up as a number rather than as a vague complaint about smoothness.

### Tearing

Tearing is prevented structurally, not by timing. `SDL_RenderPresent` with vsync flips at vblank, and every texture upload for a pass happens before any draw call in that pass, so no tile can be presented half-old and half-new. The one real risk is vsync being silently refused — `SDL_SetRenderVSync` can fail on some drivers — so its result is checked at startup and surfaced as a `vsync_unavailable` degradation rather than left to be discovered as an unexplained visual fault.

## Failure Modes

| Failure | Behaviour |
|---------|-----------|
| SDL fails to initialise | Display plane reports the error; API and UI continue serving so the user can diagnose |
| No DRM master available | Explicit error naming the likely cause (another DRM client, missing `vc4-kms-v3d`, group membership) |
| A withdrawn masked transition is still stored | Renders as `fade`; reported as `masked_wipe_fade` in engine state and migrated on next startup |
| Texture allocation fails | Tile shows a placeholder card; other tiles continue |
| Source has no new frames | Hold last frame while the source is still needed |
| Source resolution changes | Destroy and recreate that source's single texture |
| Render loop panics | Recovered; the error is published to the control plane and the process shuts down deliberately rather than crashing |
| Render loop overruns vsync | Frames drop naturally; transition progress stays wall-clock correct |

Nothing in this list stops the render loop. A wall that freezes is worse than a wall showing one placeholder tile.

## Zero-Copy: The Future Path

The baseline copies decoded frames into system memory. Should that prove insufficient, the upgrade path is known and the frame interface is designed to accommodate it — but it is deliberately not the starting point. The analysis, including why SDL has no DMA-BUF import API and what the SAND tiled format costs, is in [Hardware Decode](hardware-decode.md).

SDL's own [`test/testffmpeg.c`](https://github.com/libsdl-org/SDL/blob/main/test/testffmpeg.c) is effectively a reference implementation: it handles `AV_PIX_FMT_DRM_PRIME`, builds an `EGLImage` via `EGL_LINUX_DMA_BUF_EXT` from the `AVDRMFrameDescriptor`, binds it with `glEGLImageTargetTexture2DOES`, and hands the resulting GL texture name to SDL through `SDL_PROP_TEXTURE_CREATE_OPENGLES2_TEXTURE_NUMBER`. If we go there, that is the code to read first — including its lesson from [SDL#14908](https://github.com/libsdl-org/SDL/issues/14908), where a missing `eglDestroyImage` per frame leaked roughly 112 MB/s.

## References

- [SDL3 wiki](https://wiki.libsdl.org/SDL3/)
- [`SDL_UpdateNVTexture`](https://wiki.libsdl.org/SDL3/SDL_UpdateNVTexture)
- [`SDL_UpdateYUVTexture`](https://wiki.libsdl.org/SDL3/SDL_UpdateYUVTexture)
- [`SDL_SetRenderTarget`](https://wiki.libsdl.org/SDL3/SDL_SetRenderTarget)
- [`SDL_HINT_KMSDRM_REQUIRE_DRM_MASTER`](https://wiki.libsdl.org/SDL3/SDL_HINT_KMSDRM_REQUIRE_DRM_MASTER)
- [SDL 3.4.0 release notes](https://github.com/libsdl-org/SDL/releases/tag/release-3.4.0)
- [W3C SMIL 3.0 Transition Effects Module](https://www.w3.org/TR/smil/smil-transitions.html)
