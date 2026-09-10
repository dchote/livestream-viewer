# 0004: Ingest and Display Engine

## Status: Implemented

## Summary

Finish Stages 1–4 of the roadmap as **production code** (not throwaway spikes): in-process libav ingest, lock-free frame slots, and an SDL3 windowed renderer on macOS, wired to the existing control plane. Iterate against the local UniFi sources until the wall and Preview MJPEG actually work.

**Done when:** `go run ./cmd/livestream-viewer -display=true` opens a windowed SDL3 surface; the configured `1+3` grid, transition playlist, and full-bleed screens play live; decoder health is live over SSE; Preview shows MJPEG of the composited output when the engine is running; probe writes codec/resolution/fps, `hw_decode`, and a JPEG thumbnail; unit tests pass without opening network streams.

## Scope Boundaries

**In this pass**

- `internal/frame` triple-buffered lock-free slots
- `internal/ingest` decode workers (go-astiav), capability probe, reconnect, YouTube re-resolve
- VideoToolbox: wait for a keyframe before `SendPacket`; never `AV_CODEC_FLAG_LOW_DELAY` on hardware opens; RTSP stays on software unless probe produced a hardware frame
- YouTube: supervised PO token provider (`auto` uses a managed Deno server when present, otherwise a sidecar on `:4416`), **embedded** bgutil yt-dlp plugin copied to `data/yt-dlp-plugins/bgutil/`, cookies when the bot check remains; yt-dlp and the managed provider run in their own process groups so a timeout reaps children. The Linux `.deb` ships Deno and the BgUtils server.
- SDL3 windowed output (`Zyko0/go-sdl3`, system library, never `binsdl`)
- Layout fit, transition `Draw()`, compositor scenes, engine loop wrapping `schedule.Runtime`
- MJPEG preview of composited output; `GET /api/v1/system/info` capabilities
- Preview page uses MJPEG when `display_running`, layout diagram otherwise
- Long-run hardening: panic recovery on every long-lived goroutine, bounded backoff, one
  texture per source destroyed on geometry change, packed pixels written into the
  reserved slot-pool buffer (no staging copy), and no database query at render-loop frequency
- Control-plane guards: real `/health` readiness, login throttling, probe concurrency cap,
  streaming-client caps, request body limits, PATCH range checks, and SQLite WAL with a
  busy timeout — see [Health and resource guards](../technical-overview.md#health-and-resource-guards)
- Withdrawal of the masked wipes (`irisWipe`, `ellipseWipe`, `clockWipe`), which were
  advertised but rendered as a plain crossfade, with a startup migration to `fade`

**Explicitly deferred**

- Audio, multi-output, zero-copy DMA-BUF
- Pi capacity numbers (qualitative guidance only; see [hardware-decode.md](../architecture/hardware-decode.md))

KMSDRM full-screen panel output and Home Assistant DRM-on-panel access are confirmed working on Raspberry Pi as a Home Assistant add-on.

## Cross-Cutting Contracts

| Contract | Action in this change |
|----------|------------------------|
| `api/openapi.yaml` | Typed capabilities/displays; DisplayState decoder health, degradations, fps; preview 200 vs 503; `Health` schema; 429 on throttled endpoints; masked wipes removed from the transition enum; `offline_grace_ms` and `output_rotation` removed |
| `docs/reference/glossary.md` | No new terms expected |
| Display strategy / layout catalogue | Geometry unchanged; engine consumes `strategy.Snapshot` |
| Password fields | Unchanged |

## Host toolchain

cgo links **FFmpeg 8.x** (`go-astiav` is n8.0 only). On this Mac, Homebrew `ffmpeg`/`ffmpeg-full` is 9.x — use keg-only `ffmpeg@8` via `PKG_CONFIG_PATH`. SDL3 3.4+ is loaded at runtime from the system library.

## Related

- [0001: Project Scaffold and Implementation Roadmap](0001-project-scaffold.md)
- [0003: Stream Sources and Display Strategy](0003-stream-sources-and-display-strategy.md)
- [Display Pipeline](../architecture/display-pipeline.md)
- [Stream Ingest](../architecture/stream-ingest.md)
- [Render Loop Pattern](../patterns/render-loop-pattern.md)
