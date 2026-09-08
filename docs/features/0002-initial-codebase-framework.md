# 0002: Initial Codebase Framework

## Status: Implemented

## Summary

Carve a runnable **control-plane + management UI shell** out of the [0001 roadmap](0001-project-scaffold.md) so the project builds and runs on a developer workstation today (including macOS with `-display=false`). The Go server serves the embedded SPA and Swagger; the frontend ships the dark Vuetify theme, nav, and page layouts from the UI guidelines. Display engine, FFmpeg ingest, and hardware decode stay out of scope (covered by 0001 Stages 1–4).

**Done when:** on this Mac, `./scripts/build.sh` produces a binary; `./build/livestream-viewer -display=false` serves `/`, `/docs`, and `/health`; Vite `yarn dev` proxies to the API; add-on files exist and `docker build` for the add-on image succeeds for the control-plane binary.

## Scope Boundaries

**In this pass**

- Go module, package layout, bootstrap config, slog, SQLite/GORM
- HTTP server: SPA embed, Swagger UI, OpenAPI, health, system info, layout/transition catalogues, stub list/CRUD shapes for sources/screens/tour/display/config
- Vue 3 + Vuetify 3 shell: theme, AuthenticatedLayout/DefaultLayout, nav, Preview / Stream Sources / Display Strategy pages (structured placeholders, not full editors)
- Common components: StandardCard, StandardDialog, BackButton
- Build scripts, Makefile, CI, HA add-on scaffold + `repository.yaml`
- Seeded JWT auth so AuthenticatedLayout is real (login page + default admin)

**Explicitly deferred** (still tracked in 0001)

- SDL3 render loop, go-astiav decode, frame slots, preview MJPEG, real source probing
- Full LayoutPicker/TileEditor/PlaylistEditor/TourEditor behaviour (pages show section structure + empty states only)
- DRM verification on HA OS

**macOS default:** `-display=false`. Do not link `go-sdl3` or `go-astiav` in this pass so Homebrew FFmpeg major version cannot block the framework build. CGO is still used for `mattn/go-sqlite3`.

## Cross-Cutting Contracts

| Contract | Action in this change |
|----------|------------------------|
| `api/openapi.yaml` | Create hand-maintained OpenAPI 3.0 covering every shipped handler (including stubs) |
| `docs/reference/glossary.md` | No new terms expected; only touch if a label invents vocabulary |
| Display strategy / layout catalogue | Implement catalogue as data in `internal/display/layout` and serve via `GET /api/v1/layouts` |
| Transition catalogue | Same pattern in `internal/display/transition` catalogue data + `GET /api/v1/transitions` |

## Default Credentials

First-run seed (you are prompted to change this password immediately after login):

- Username: `admin`
- Password: `admin`

Administrators manage other accounts under **Settings → Users**. Created users also must change their password on first login. Roles are `admin` (full write access) and `user` (read-only preview and settings).

## Related

- [0001: Project Scaffold and Implementation Roadmap](0001-project-scaffold.md)
- [Technical Overview](../technical-overview.md)
- [Build and Test](../build-and-test.md)
