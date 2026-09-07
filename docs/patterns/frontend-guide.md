# Frontend Patterns Guide

> **Status:** Implemented for the management UI, including Stream Sources and Display Strategy editors. Preview MJPEG of composited output waits on the display engine; the Preview page uses a client-side layout diagram until then.

This document defines patterns and conventions for the Vue 3 + Vuetify 3 management frontend. All frontend code follows these conventions.

## Directory Structure

```
frontend/src/
├── layouts/            # DefaultLayout, AuthenticatedLayout
├── pages/              # Route components (file-based routing)
│   ├── index.vue                    → /            (Preview)
│   ├── login.vue                    → /login
│   ├── change-password.vue          → /change-password
│   ├── admin/
│   │   └── users.vue                → /admin/users
│   └── settings/
│       ├── sources.vue              → /settings/sources
│       └── display.vue              → /settings/display
├── components/
│   ├── common/         # StandardCard, StandardDialog, PasswordField (ListToolbar/BackButton optional stubs)
│   ├── admin/          # EditUserRoleDialog, CreateUserDialog
│   ├── sources/        # SourceForm, SourceList, SourceProbeChip, UploadDropzone
│   └── display/        # LayoutPicker, TileEditor, PlaylistEditor, TransitionEditor, ScreensEditor, TourEditor, LayoutDiagram
├── stores/             # Pinia stores
├── composables/        # useDisplayState, useEventStream, useLayouts
├── utils/              # api.js, ingress.js, roles.js, formatters
├── plugins/            # vuetify, router, pinia
├── styles/             # theme.scss
└── App.vue
```

## Navigation

Deliberately shallow — Preview, Settings, and an admin-only Users item.

```
Preview                  /
Settings
 ├── Stream Sources      /settings/sources
 ├── Display Strategy    /settings/display
 └── Users (admin)       /admin/users
```

Rendered as a `v-navigation-drawer`. Settings is a labelled section, not a third top-level product area. Users sits with the other Settings items and is hidden unless `auth.isAdmin`.

A first login (seeded admin, or any account an administrator just created) lands on `/change-password` until `must_change_password` is cleared. That page uses DefaultLayout so the rest of the app is not reachable until the password is changed.

## Page Responsibilities

### Preview (`/`)

A monitoring view, not a second renderer. It shows what the physical display is actually doing:

- **Layout diagram** — Until the display engine runs, Preview draws a client-side diagram from `GET /api/v1/layouts` plus scheduler tile names, with a clear "engine not running" notice. `GET /api/v1/preview/stream` stays `503 engine_not_running`. The Settings pages remain fully usable in that state. MJPEG of composited output is a later stage.
- **Active screen indicator** — Which screen is showing, what is next, and where the tour is in its dwell.
- **Tile status** — Per tile: source name, decoder state, resolution, whether it is on hardware or software decode.
- **Manual controls** — Previous, Next, Pause/Resume, and jump-to-screen. These map to the `POST /api/v1/display/*` endpoints.

Live data comes from the SSE stream, never from polling. See `useEventStream`.

### Stream Sources (`/settings/sources`)

A list of sources with an "Add Source" button right-aligned in the section header. Each row shows name, kind, probe summary (codec, resolution, frame rate), decode path, and status. Row actions are right-aligned on one line: **Probe** as text, Edit/Delete as icon buttons. On mobile the Probe summary column is hidden.

- Adding a source opens a StandardDialog whose fields change with the selected `kind`. YouTube shows a notice if `yt-dlp` is unavailable, read from `GET /api/v1/system/info`.
- **Probe** is an explicit action per source, and its result is what makes the UI honest about capacity. Show the decode path (hardware or software) prominently — this is the number a user needs before building a nine-tile grid.
- File uploads use a dedicated dropzone posting to `POST /api/v1/uploads`, with progress.
- Deleting a source that is in use surfaces the API's list of referencing screens in the confirmation dialog rather than a bare error.

### Display Strategy (`/settings/display`)

The substantial page. Two sections, each with its own header and right-aligned action button:

1. **Screens** — **ScreensEditor**. Empty state when none exist. Otherwise one **`v-expansion-panel` per screen** (`multiple`, `flat`, `variant="accordion"`, all collapsed by default). Titles summarize name, kind, and layout/item count; expand to edit. Grid uses LayoutPicker + TileEditor; transition uses PlaylistEditor + TransitionEditor. Per-panel **Save** (enabled only when dirty); collapsing discards unsaved edits. Delete on the title. Add Screen creates a new collapsed panel.
2. **Tour** — **TourEditor**. If there are no screens yet, the card only shows a prerequisite empty state (no Add/Save). Otherwise: tour-level Enabled/Loop, then one **`v-expansion-panel` per entry** (`multiple`, `flat`, `variant="accordion"`, all collapsed by default). Panel titles summarize screen, dwell, and transition; expand to edit those fields and the incoming transition. Add Entry appends a new collapsed panel. Card footer **Save** is enabled only when the tour is dirty.

**TransitionEditor** reads `GET /api/v1/transitions` for valid type/subtype combinations and never offers a transition the platform reports as unavailable. Easing is chosen from the named presets with an option to enter explicit cubic Bézier control points, and the curve is drawn.

## Components vs Pages

Pages stay thin and compose shared components. Never duplicate card or dialog layout in a page.

- **StandardCard** (`components/common/StandardCard.vue`) — Page layout card with title and header, toolbar, content, and actions slots.
- **StandardDialog** (`components/common/StandardDialog.vue`) — Modal with header, content, and actions. Used for every dialog, including confirmations. Never use raw `v-dialog`.
- **PasswordField** (`components/common/PasswordField.vue`) — Show/hide password input used everywhere credentials are collected.

## Header Rules

- All header content is vertically centred: `d-flex align-center`.
- Action buttons for tabular or list data live in that section's header, right-aligned, with `v-spacer` before them.
- Form action buttons (Save, Cancel) go at the **bottom** of the form, right-aligned — never in the page header.

## Layout Structure

- **AuthenticatedLayout** when authenticated and the password does not need changing: app bar, navigation drawer, user menu.
- **DefaultLayout** for guests and for the forced password-change screen.
- `App.vue` switches between them on auth state (`isAuthenticated && !mustChangePassword`).
- **Layouts must not add padding around the slot.** Each page uses `v-container` with class `page-content`, which provides the page margins. Wrapping the slot in `pa-4` compounds with `v-container` and produces excessive outer margins.

## File-Based Routing

Routes are generated from `src/pages/` by `unplugin-vue-router`:

- `src/pages/index.vue` → `/`
- `src/pages/login.vue` → `/login`
- `src/pages/change-password.vue` → `/change-password`
- `src/pages/settings/sources.vue` → `/settings/sources`
- `src/pages/settings/display.vue` → `/settings/display`
- `src/pages/admin/users.vue` → `/admin/users`
- Dynamic routes: `[id].vue` → `/:id`

The Vite config uses `importMode: 'sync'` so route components are statically imported. This produces a single JS bundle and avoids chunk 404s when the frontend is embedded and served by the Go SPA handler.

## Live State

Two composables own all live data. Components consume them; components do not open their own connections.

- **`useEventStream()`** — Single shared `EventSource` against `GET /api/v1/events`, with automatic reconnect and backoff. Fans out to subscribers. Exactly one connection per browser tab.
- **`useDisplayState()`** — Reads from the event stream, seeded by an initial `GET /api/v1/display/state` so the page renders immediately rather than waiting for the first event.

Never poll. If a value is not on the event stream and it should be, add it there rather than adding a timer.

## API and Error Handling

- Centralised client in `utils/api.js`, JWT in the `Authorization` header.
- On 401: clear auth and redirect to login.
- On 403 `password_change_required`: redirect to `/change-password` without logging out.
- Other 403 responses (insufficient role) stay on the page; surface them with `v-alert`.
- Catch errors in components and surface them with `v-alert` on the parent page. Never `alert()`.
- Log with a component prefix: `console.log('[SourceForm] API error:', error)`.

## Home Assistant Ingress

Under ingress the app is served from `/api/hassio_ingress/<token>/`, so nothing may assume it lives at the site root.

- `utils/ingress.js` exports `getIngressBase()`, the single source of truth for the path prefix.
- Both the API client base and the Vue Router base derive from it.
- `vite.config.js` sets `base: './'` for relative asset paths, and the Go server injects a `<base href>` when serving `index.html` so assets resolve on direct navigation and reload.
- The MJPEG preview URL must also be built through `getIngressBase()`. It is easy to miss because it is set as an `<img src>` rather than fetched through the API client.

## Form Input Properties

- `variant="outlined"`
- `density="compact"` (login and register may use `comfortable` for accessibility)
- `hide-details="auto"`
- `autocomplete` is **required on every text field** using HTML tokens only (no readonly/decoy hacks):
  - Non-credential fields: `off`
  - Login: `username` / `current-password`
  - Create user & source credentials: username `off`, password `new-password`
  - Change password: current `current-password`, new/confirm `new-password`
- Password fields always use `PasswordField` (show/hide toggle). Never a raw `type="password"` input
- `style="max-width: 320px;"` for fixed-width inputs on pages; dialogs usually let fields fill the content width
- Side-by-side fields: the `field-row` theme class — never `v-row`/`v-col`

## Form Layout

```vue
<v-form @submit.prevent="handleSubmit">
  <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
  <div class="mb-4">
    <v-text-field v-model="name" label="Name" variant="outlined" density="compact" hide-details="auto" autocomplete="off" />
  </div>
  <div class="d-flex justify-end" style="gap: 8px">
    <v-btn variant="text" @click="cancel">Cancel</v-btn>
    <v-btn type="submit" color="primary" variant="elevated">Save</v-btn>
  </div>
</v-form>
```

## Feedback and Confirmations

Never use `alert()` or `confirm()`.

Page-level alerts come from `useFeedback()`, which owns both `error` and `success`. Success is
transient and auto-clears, and setting one clears the other — a success banner that nothing
resets ends up sitting next to the failure that came after it.

```js
const { error, success, showError, showSuccess, clear: clearFeedback } = useFeedback()
```

Deletes use `ConfirmDeleteDialog`; it handles the question, the error alert placement, the
loading state, and `:fullscreen` on mobile. The default slot takes extra detail (for example
what still references the record).

```vue
<ConfirmDeleteDialog
  v-model="showDelete"
  title="Delete source?"
  :name="toDelete?.name"
  :error="deleteError"
  :loading="deleting"
  @close="toDelete = null"
  @confirm="confirmDelete"
/>
```

Anything else that needs a modal uses StandardDialog directly, with `:fullscreen="mobile"` from
`useDisplay()`. The primary button names the outcome: **Create** in a create dialog, **Save**
when editing, **Delete** in a confirm.

## Shared primitives

| Need | Use |
|------|-----|
| Nothing-here placeholder | `EmptyState` |
| Delete confirmation | `ConfirmDeleteDialog` |
| Any other modal | `StandardDialog` |
| Password input | `PasswordField` |
| Page alerts | `useFeedback()` |
| Screen / tour write body | `screenPayload`, `tourPayload` (`@/utils/payloads`) |
| Fit modes, select items, name lookup, transition summary | `@/utils/formatters` |
| Password policy | `@/utils/passwords` |
| Full-bleed layout id | `FULL_BLEED_ID` (`@/utils/layouts`) |

The Save-enabled check and the request body must both come from `screenPayload` /
`tourPayload`. Computing them separately lets Save light up on one shape and send another.

## Spacing Rules

- Between form fields: `mb-4` on the upper field (prefer `mb-*` over `mt-*` for section rhythm)
- Control clusters (buttons, chips, switches): `d-flex` + `style="gap: 8px"`
- Switch rows: class `switch-cluster` (16px), not bare `gap: 8px`
- Side-by-side form fields: class `field-row` (16px both axes; no per-child margin)
- Centred single-card pages: class `page-narrow` on the `v-container`
- Inline editors: **Save** only, disabled when clean; dialogs keep Cancel + primary
- `v-row`/`v-col` are not used anywhere; do not rely on Vuetify `ga-*`

See [UI Style Guidelines](ui-style-guidelines.md) for the full visual specification.
