# 0003: Stream Sources and Display Strategy

## Status: Implemented

## Summary

Complete Stream Sources and Display Strategy as a **control-plane** feature: real CRUD, validation, uploads, PATH-based `yt-dlp`/`ffprobe` resolution, Vue editors, and a headless wall-clock scheduler that drives SSE and display commands. SDL/FFmpeg ingest and composited MJPEG stay a follow-on (0001 Stages 1–4).

**Done when:** with `-display=false`, an admin can add sources (including file uploads), probe them when `ffprobe`/`yt-dlp` are on `PATH`, build grid and transition screens with the layout catalogue, order a tour, and see the scheduler advance over SSE. Preview uses the client-side layout fallback. `yarn lint` / `yarn build` and `CGO_ENABLED=1 go test -timeout=30s ./...` pass. No `go-sdl3` or `go-astiav`.

## Scope Boundaries

**In this pass**

- Typed JSON for source options/probe, screen/tour transitions, and runtime `default_transition`
- Source and upload CRUD with 409 reference details
- Screen and tour CRUD, sequence preload, hotspot-preserving layout changes, first-screen tour seed
- Optional `yt-dlp` / `ffprobe` discovered on `PATH` (injectable in tests; no libav link). Thumbnail JPEG extraction is deferred with media ingest; `GET .../thumbnail` returns 404 until then.
- Headless `schedule.Runtime`: tour, playlist, and tile-sequence timers; display next/previous/goto/pause/resume
- SSE hub with reconnecting frontend client
- Stream Sources and Display Strategy editors; shared `PasswordField` on every password input
- Preview page bound to display state with a client-side layout diagram

**Explicitly deferred**

- SDL render loop, go-astiav decode, frame slots, MJPEG preview of composited output
- Hardware-capability probe beyond PATH checks
- `GET /api/v1/preview/stream` and `GET /api/v1/preview/frame` remain `503 engine_not_running`

## Cross-Cutting Contracts

| Contract | Action in this change |
|----------|------------------------|
| `api/openapi.yaml` | Replace 501 stubs with real responses; options/probe/transition as objects; `Tile.sequence`; 409 `details` |
| `docs/reference/glossary.md` | No new terms expected |
| Display strategy / layout catalogue | Snapshot geometry and LayoutPicker both use `GET /api/v1/layouts` / `layout.All()` |
| Password fields | Shared `PasswordField` with show/hide; no raw `type="password"` |

## Related

- [0001: Project Scaffold and Implementation Roadmap](0001-project-scaffold.md)
- [0002: Initial Codebase Framework](0002-initial-codebase-framework.md)
- [Display Strategy Pattern](../patterns/display-strategy-pattern.md)
- [Technical Overview](../technical-overview.md)
