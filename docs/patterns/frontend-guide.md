# Frontend Patterns Guide

> **Status:** Shell implemented (layouts, auth, users, settings placeholders). Full editors remain design until their feature stages.

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
│   ├── common/         # StandardCard, StandardDialog, BackButton, ListToolbar (detail pages)
│   ├── admin/          # EditUserRoleDialog, CreateUserDialog
│   ├── preview/        # PreviewCanvas, TileStatusList, DecoderHealth
│   ├── sources/        # SourceForm, SourceList, SourceProbeChip, UploadDropzone
│   └── display/        # LayoutPicker, TileEditor, PlaylistEditor, TransitionEditor, TourEditor
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

- **PreviewCanvas** — The MJPEG stream from `GET /api/v1/preview/stream`, in the output's aspect ratio. When the display engine is not running, it falls back to a client-side diagram drawn from `GET /api/v1/layouts` plus per-source thumbnails, with a clear "engine not running" notice. The Settings pages must remain fully usable in that state.
- **Active screen indicator** — Which screen is showing, what is next, and where the tour is in its dwell.
- **Tile status** — Per tile: source name, decoder state, resolution, whether it is on hardware or software decode.
- **Manual controls** — Previous, Next, Pause/Resume, and jump-to-screen. These map to the `POST /api/v1/display/*` endpoints.

Live data comes from the SSE stream, never from polling. See `useEventStream`.

### Stream Sources (`/settings/sources`)

A list of sources with an "Add Source" button right-aligned in the section header. Each row shows name, kind, probe summary (codec, resolution, frame rate), decode path, and status.

- Adding a source opens a StandardDialog whose fields change with the selected `kind`. YouTube shows a notice if `yt-dlp` is unavailable, read from `GET /api/v1/system/info`.
- **Probe** is an explicit action per source, and its result is what makes the UI honest about capacity. Show the decode path (hardware or software) prominently — this is the number a user needs before building a nine-tile grid.
- File uploads use a dedicated dropzone posting to `POST /api/v1/uploads`, with progress.
- Deleting a source that is in use surfaces the API's list of referencing screens in the confirmation dialog rather than a bare error.

### Display Strategy (`/settings/display`)

The substantial page. Three sections, each with its own header and right-aligned action button:

1. **Screens** — List of defined screens with kind, layout or item count, and a preview thumbnail. "Add Screen" in the header.
2. **Screen editor** — Opens on selecting a screen.
   - *Grid*: **LayoutPicker** shows the catalogue grouped by family (Equal, Hotspot, Vertical, Panoramic), rendering each option from the same normalised geometry the engine uses. Selecting a layout renders **TileEditor**, a clickable diagram where each cell opens a source picker and a fit selector. A cell can hold a single source or a sequence.
   - *Transition*: **PlaylistEditor**, a drag-reorderable list of sources with per-item dwell time and fit, plus a **TransitionEditor** for the transition between items.
3. **Tour** — **TourEditor**, a drag-reorderable list of screens with dwell time and the transition played when entering each one.

**TransitionEditor** reads `GET /api/v1/transitions` for valid type/subtype combinations and never offers a transition the platform reports as unavailable. Easing is chosen from the named presets with an option to enter explicit cubic Bézier control points, and the curve is drawn.

## Components vs Pages

Pages stay thin and compose shared components. Never duplicate card or dialog layout in a page.

- **StandardCard** (`components/common/StandardCard.vue`) — Page layout card with title and header, toolbar, content, and actions slots.
- **StandardDialog** (`components/common/StandardDialog.vue`) — Modal with header, content, and actions. Used for every dialog, including confirmations. Never use raw `v-dialog`.
- **BackButton** (`components/common/BackButton.vue`) — Icon-only back button for card headers, with a `fallback` route.

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
- `autocomplete` is **required on every text field**: `autocomplete="off"` on all non-auth fields; `username`, `current-password`, or `new-password` on auth forms only
- `class="mr-2"` between inputs
- `style="max-width: 320px;"` for fixed-width inputs

## Form Layout

```vue
<v-form @submit.prevent="handleSubmit">
  <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
  <div class="mb-4">
    <v-text-field v-model="name" label="Name" variant="outlined" density="compact" hide-details="auto" autocomplete="off" />
  </div>
  <div class="d-flex justify-end mt-4">
    <v-btn variant="text" class="mr-2" @click="cancel">Cancel</v-btn>
    <v-btn type="submit" color="primary" variant="elevated">Save</v-btn>
  </div>
</v-form>
```

## Feedback and Confirmations

Never use `alert()` or `confirm()`. Use `v-alert` for feedback and StandardDialog for confirmations.

```vue
<StandardDialog v-model="showDeleteDialog" title="Delete source?" max-width="400" :fullscreen="mobile" @close="sourceToDelete = null">
  <p>Are you sure?</p>
  <template #actions>
    <v-spacer />
    <v-btn variant="text" class="mr-2" @click="showDeleteDialog = false">Cancel</v-btn>
    <v-btn color="error" variant="elevated" @click="confirmDelete">Delete</v-btn>
  </template>
</StandardDialog>
```

Pass `:fullscreen="mobile"` from `useDisplay()` on small screens.

## Spacing Rules

- Between form fields: `mb-4`
- Before action buttons: `mt-4`
- Between buttons: `mr-2` on the first button
- Between chips: `mr-2 mb-2` on each chip
- **Never use `gap`** — use explicit margins

See [UI Style Guidelines](ui-style-guidelines.md) for the full visual specification.
