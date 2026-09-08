# Display Strategy Pattern

> **Status:** Implemented. The engine consumes `strategy.Snapshot`; geometry is unchanged.

The display strategy is the user-facing configuration model: what is shown, where, and for how long. This document defines the model precisely, because it is the shared contract between the database schema, the REST API, the display engine, and the Settings UI.

Terminology follows CCTV/VMS and broadcast convention. See the [Glossary](../reference/glossary.md) for definitions and provenance.

## The Three Levels

```
Tour            ordered screens, each with a dwell time and a transition
 └── Screen     one complete composition of the output
      ├── grid        ── tiles, one source (or sequence) per cell
      └── transition  ── an ordered playlist of full-screen sources
```

A tour with one entry and `loop: false` is a static wall. That is the expected starting configuration and everything else layers on top of it, so the model must make that case trivial rather than requiring the user to understand tours before they can show one camera.

## Layouts

A layout is **data, not code**: a named list of normalised rectangles in the unit square.

```json
{
  "id": "1+5",
  "name": "1 + 5 hotspot",
  "cells": 6,
  "rects": [
    { "x": 0.0,    "y": 0.0,   "w": 0.6667, "h": 0.6667 },
    { "x": 0.6667, "y": 0.0,   "w": 0.3333, "h": 0.3333 },
    { "x": 0.6667, "y": 0.3333,"w": 0.3333, "h": 0.3333 },
    { "x": 0.0,    "y": 0.6667,"w": 0.3333, "h": 0.3333 },
    { "x": 0.3333, "y": 0.6667,"w": 0.3333, "h": 0.3333 },
    { "x": 0.6667, "y": 0.6667,"w": 0.3333, "h": 0.3333 }
  ]
}
```

Keeping layouts as data means the same numbers drive the engine, the `GET /api/v1/layouts` response, the Settings layout picker, and the Preview page's client-side fallback. Adding a layout is a data change, not a code change, and there is no way for the UI's idea of a layout to drift from the engine's.

### Catalogue

Grouped the way VMS products group them, so the picker is familiar:

| Family | Layouts |
|--------|---------|
| **Full bleed** | `full` |
| **Equal** | `2x1`, `1x2`, `2x2`, `3x3`, `4x4` |
| **Hotspot** (1+N) | `1+3`, `1+5`, `1+7`, `1+12` |
| **Vertical** | `3v`, `1v+6` |
| **Panoramic** | `2p`, `1p+6` |

`full` is the only single-cell layout. An earlier `1x1` in the Equal family had identical
geometry; keeping both meant two ways to express the same wall and two code paths that had
to agree. It was retired, and `database.migrateRetiredLayouts` rewrites existing screens to
`full` on startup.

Cell index 0 is always the primary or hotspot tile where the layout has one. When switching between hotspot layouts, the source assigned to cell 0 is preserved — this is standard VMS behaviour and users expect it.

### Geometry Resolution

The solver turns normalised rects into pixels:

1. Scale each rect by the current output resolution.
2. Inset by the configured gutter (uniform, in pixels).
3. Fit the source's video rect inside the cell per the tile's `fit` mode.
4. **Round to whole pixels.** Sub-pixel tile edges shimmer, and on a static wall that is the first thing anyone notices.

### Fit Modes

| Mode | Behaviour |
|------|-----------|
| `contain` | Scale to fit, preserve aspect ratio, letterbox or pillarbox with the background colour. The safe default. |
| `cover` | Scale to fill, preserve aspect ratio, crop the overflow via the source rect. Good for cameras where edges do not matter. |
| `fill` | Stretch to the cell, ignore aspect ratio. Available, but never the default. |

Fit is per tile, not per screen. A 16:9 camera and a 4:3 camera in the same grid usually want different treatment.

## Screens

### Grid Screen

```json
{
  "id": 3,
  "name": "Cameras",
  "kind": "grid",
  "layout": "1+5",
  "tiles": [
    { "index": 0, "source_id": 12, "fit": "contain" },
    { "index": 1, "source_id": 13, "fit": "cover" },
    { "index": 2, "sequence": { "source_ids": [14, 15, 16], "dwell_ms": 8000 } },
    { "index": 3, "source_id": null }
  ]
}
```

Two things worth noting:

- **Tiles may be empty.** `source_id: null` renders the background. A user should be able to pick a `2x2` and fill three cells.
- **A tile may hold a sequence** instead of a single source: an ordered list of sources rotating within that one cell on its own dwell timer. This is the standard VMS "camera sequence" and it is what makes a `1+5` hotspot layout genuinely useful — six cells can cover twelve cameras.

Sequences rotate independently per tile, on their own timers. They are not synchronised with each other or with the tour, because synchronising them makes the wall pulse.

### Transition Screen

```json
{
  "id": 4,
  "name": "Feature rotation",
  "kind": "transition",
  "loop": true,
  "transition": {
    "type": "slideWipe",
    "subtype": "leftToRight",
    "duration_ms": 800,
    "easing": "ease-in-out"
  },
  "items": [
    { "position": 0, "source_id": 20, "dwell_ms": 15000, "fit": "cover" },
    { "position": 1, "source_id": 21, "dwell_ms": 15000, "fit": "contain" }
  ]
}
```

One full-screen source at a time, stepping through the playlist. The transition applies between consecutive items.

## Tour

```json
{
  "enabled": true,
  "loop": true,
  "entries": [
    { "position": 0, "screen_id": 3, "dwell_ms": 30000,
      "transition": { "type": "fade", "subtype": "crossfade", "duration_ms": 600, "easing": "ease-in-out" } },
    { "position": 1, "screen_id": 4, "dwell_ms": 60000,
      "transition": { "type": "cut", "duration_ms": 0 } }
  ]
}
```

The transition on an entry is the one played **when moving to that entry**, which is the convention that makes "give this screen a dramatic entrance" express naturally.

When `enabled` is false, the display holds whichever screen is pinned. `POST /api/v1/display/goto/:screenId` pins a screen and implicitly pauses the tour; `POST /api/v1/display/resume` releases it.

## Transitions

```json
{
  "type": "pushWipe",
  "subtype": "leftToRight",
  "duration_ms": 700,
  "easing": "cubic-bezier(0.4, 0.0, 0.2, 1.0)",
  "color": null
}
```

The `type` / `subtype` split comes from the [W3C SMIL 3.0 Transition Effects Module](https://www.w3.org/TR/smil/smil-transitions.html), which codifies the SMPTE 258M wipe catalogue. Adopting it verbatim is nearly free and instantly legible to anyone with a broadcast background — the same enum appears in GStreamer's `smpte` element and in DirectShow.

| Type | Subtypes | Implementation family |
|------|----------|----------------------|
| `cut` | — | Immediate |
| `fade` | `crossfade`, `fadeToColor`, `fadeFromColor` | Alpha |
| `barWipe` | `leftToRight`, `topToBottom` | Geometric |
| `boxWipe` | `topLeft`, `topRight`, `bottomRight`, `bottomLeft` | Geometric |
| `barnDoorWipe` | `vertical`, `horizontal` | Geometric |
| `pushWipe` | `fromLeft`, `fromRight`, `fromTop`, `fromBottom` | Geometric |
| `slideWipe` | `fromLeft`, `fromRight`, `fromTop`, `fromBottom` | Geometric |

`GET /api/v1/transitions` returns this catalogue with each type's valid subtypes, so the UI never offers a transition that will silently degrade. Every advertised type and subtype reaches a distinct implementation, and tests enforce both properties: one asserts nothing in the catalogue renders as a degradation, another asserts no two subtypes of the same type produce identical draw instructions.

**The masked wipes are not in the catalogue.** `irisWipe` (SMPTE 101), `ellipseWipe` (119–121), and `clockWipe` (201–204) need a per-pixel alpha mask, which means a fragment shader. That is unimplemented, and while they were advertised they simply drew a crossfade. They are withdrawn rather than left as decoration; stored configurations are migrated to `fade` on startup. The SMPTE names are reserved for whenever the shader lands.

**Push versus slide** is defined explicitly because vendors are sloppy about it: in a *push*, the outgoing image is shoved off-screen by the incoming one, so both move. In a *slide*, the incoming image moves over a stationary outgoing one.

### Easing

Named presets expand to CSS-standard cubic Bézier control points:

| Preset | Control points |
|--------|----------------|
| `linear` | `(0, 0, 1, 1)` |
| `ease` | `(0.25, 0.1, 0.25, 1)` |
| `ease-in` | `(0.42, 0, 1, 1)` |
| `ease-out` | `(0, 0, 0.58, 1)` |
| `ease-in-out` | `(0.42, 0, 0.58, 1)` |

Explicit `cubic-bezier(x1, y1, x2, y2)` is accepted anywhere a preset is. Storing easing as control points keeps the model portable and lets the UI show the same curve the engine evaluates.

## Timing Semantics

Three distinct concepts, which VMS products keep separate and so do we:

| Concept | Scope | Field |
|---------|-------|-------|
| **Sequence** | Sources rotating within one tile | `tile.sequence.dwell_ms` |
| **Playlist** | Sources stepping in a transition screen | `item.dwell_ms` |
| **Tour** | Screens stepping across the whole output | `entry.dwell_ms` |

**Dwell time excludes transition duration.** A screen with `dwell_ms: 30000` and an incoming 600 ms transition is fully visible for 30 seconds; the transition is additional. This is the interpretation users expect and it makes the arithmetic of "how long is one full loop" predictable.

All timers are wall-clock, not frame-counted, so dropped frames do not slow the schedule.

## Validation

Enforced by the API, not just the UI:

- A tile's `index` must be within the layout's cell count.
- A source referenced by any tile, sequence, or playlist item must **exist**. Sources may be disabled while still referenced; the scheduler treats those tiles as empty/offline until re-enabled.
- Deleting a source that is referenced is refused; the response lists the referencing screens so the UI can offer to clear them.
- Layout IDs must refer to entries in the layout catalogue. Transition screens use the dedicated `full` (full-bleed) layout for Preview geometry when they have no grid layout of their own.
- A transition screen needs at least one item; an enabled tour needs at least one entry.
- `duration_ms` must be zero for `cut` and greater than zero otherwise.
- A `subtype` must be valid for its `type`.
- Dwell times have a sane floor (one second) to prevent a configuration that thrashes the decoders.

## Engine Handoff

The engine never reads the database. When configuration changes, the control plane resolves the full strategy — screens, tiles, sources, resolved layout geometry — into an **immutable snapshot** and sends it over the command channel. The engine swaps to the new snapshot at a frame boundary.

This means a configuration edit can never tear a frame, can never block the render loop on SQLite, and can never leave the engine holding a half-applied change. It also makes the engine trivially testable: hand it a snapshot, ask for a scene.

See [Concurrent State Pattern](concurrent-state-pattern.md).
