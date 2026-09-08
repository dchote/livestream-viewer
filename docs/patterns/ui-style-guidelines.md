# UI Style and Guidelines

> **Status:** Implemented for the management UI, including layout picker, tile/playlist/tour editors, source forms, and PasswordField.

This document defines the visual design standards for the livestream-viewer management frontend. All UI follows these guidelines for consistency.

## Design Philosophy

Material Design 3 palette, matching [go-mumble-server](https://github.com/dchote/go-mumble-server). This is an operations tool that may be left open on a second monitor. Card chrome uses the shared blue header gradient; video content is still the only thing that should dominate the Preview canvas.

Page shell density (`v-container.page-content`) follows the 8wi interior-page pattern: modest top/bottom padding, no stacked `py-8`.

## Layout and Structure Rules

### Headers

- **Vertical alignment**: All header content must be vertically centre-aligned. Use `d-flex align-center` on header rows.
- **Action buttons in headers**: Action buttons for tabular or list data (Add, Create, Edit) live in that section's header, right-aligned. Use `v-spacer` before them.
- **Section headers**: Each data section (Sources, Screens, Tour) has a header row with the title on the left and its primary action button right-aligned.

### Navigation Pattern

- **Sources**: a `v-data-table` whose rows open a **StandardDialog** form. Row actions are right-aligned icon buttons that never wrap (see `table-row-actions`).
- **Screens and tour entries**: **`v-expansion-panels`** (`multiple`, `flat`, `variant="accordion"`, start collapsed) inside their StandardCard, with the editor in the expanded panel. Dense nested editors (e.g. TransitionEditor) live inside that panel.

There is no separate detail *page* for any entity, and therefore no "Edit in detail header" pattern — editing happens in a dialog (Sources, Users) or in an expansion panel (Screens, Tour).

### Forms

- Use `mb-4` between stacked form elements (gap on the upper field).
- Side-by-side fields: the **`field-row`** theme class (see Spacing and Layout) — never `v-row`/`v-col`.
- Switch / chip / toolbar clusters: buttons/chips use `style="gap: 8px"`; boolean settings use **`v-switch inset`** in a `switch-cluster` row (16px gap).
- **Inline / card editors** (expansion panels, StandardCard `#actions`): primary action label is **`Save`** — never “Save tour”, “Save screen”, etc. Enable Save only when the form is dirty; do not pair it with a Cancel that only reverts. Discard unsaved work by collapsing the panel or leaving without saving.
- **Dialogs** still use Cancel + primary (Save / Delete / …) because Cancel dismisses the modal.

### Dialogs

- All dialogs use **StandardDialog** with title, content slot, and actions slot. Never raw `v-dialog`.
- Actions slot: `v-spacer`, then Cancel and the primary action. `mr-2` on the first button.
- **Mobile**: pass `:fullscreen="mobile"` from `useDisplay()` so form and confirm dialogs fill the viewport below the mobile breakpoint.
- Validation and API errors for a dialog belong **inside** that dialog (`v-alert`), not on the parent page behind it.
- Prefer local form state in the dialog component; emit a single `save` payload on success path.

## Color Palette

Defined in `frontend/src/styles/theme.scss` and `frontend/src/plugins/vuetify.js`.

- **Primary**: Material Blue (#1976D2)
- **Secondary**: Grey (#424242)
- **Accent/Info**: Blue (#2196F3)
- **Success**: Green (#4CAF50)
- **Warning**: Orange (#FF9800)
- **Error**: Red (#F44336)
- **Background/Surface**: Theme-driven (light grey / dark)

**Dark theme is the default.** Light and dark palettes match go-mumble-server: dark-mode primary is `#42A5F5`, light-mode primary is `#1976D2`.

### Card chrome

StandardCard and StandardDialog use the go-mumble-server chrome:

- Sharp corners (`border-radius: 0`)
- 3px Material Blue top border
- Header band: light-mode gradient `#1976D2 → #1565C0 → #2196F3`; dark-mode `#0D47A1 → #1565C0 → #1976D2`
- White title text in light mode; `on-surface` in dark mode

Do not use `variant="outlined"` on StandardCard — the theme supplies the border.

## Typography

- **Font family**: System stack — `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif`
- **No decorative or custom fonts**
- **Card headers**: `text-h5` (1.5rem) — avoid `text-h4`, which is too large
- **Card titles**: `font-weight: 600`
- Use Vuetify typography classes: `text-h5`, `text-h6`, `text-body-1`, `text-caption`

## Spacing and Layout

Aligned with the 8wi design-guide spacing model (4px Vuetify grid). Prefer utilities over custom CSS.

### 4px grid

| Class | Typical px |
|-------|------------|
| `*-2` | 8px |
| `*-3` | 12px |
| `*-4` | 16px |
| `*-5` | 20px |

### Prefer Vuetify utilities

Use `pa-*`, `ma-*`, `mb-*`, etc. in templates. Do **not** add `theme.scss` rules that only restate Vuetify spacing. Reserve theme CSS for brand chrome and structural shells (`.page-content`, StandardCard).

### Vertical rhythm: `mb-*`, not `mt-*`

Put the gap on the **upper** element. Avoid `mt-*` for section spacing (double-spacing bugs).

| Context | Spacing |
|---------|---------|
| Stacked form fields | `mb-4` |
| Section label → content | `mb-2` / `mb-3` on the label |
| Cards / alerts on a page | `mb-4` |
| Divider before a detail block | `mb-4` on the divider |

### Control clusters: inline `gap`

Switches, chips, toolbar buttons, and wrap rows:

```vue
<div class="d-flex align-center flex-wrap" style="gap: 8px">
```

| Cluster | Spacing |
|---------|---------|
| Buttons / chips in a row | `style="gap: 8px"` |
| Switches in a row | class `switch-cluster` (16px) |
| Side-by-side form fields | class `field-row` (16px, both axes) |

#### `field-row`

```vue
<div class="field-row mb-4">
  <v-select label="Screen" … />
  <v-text-field label="Dwell time (seconds)" style="max-width: 200px;" … />
  <div class="field-row__actions d-flex align-center" style="gap: 4px">…</div>
</div>
```

Children split the row evenly (`flex: 1 1 0`); cap a field that should stay narrow with
`style="max-width: …px"`, and give a trailing icon cluster `field-row__actions` so it sits at
its natural width. Below `sm` the row stacks and the same 16px gap applies vertically, so
children need **no** margin of their own — adding `mb-4 mb-sm-0` is the double-spacing bug
this class exists to prevent.

Centred single-card pages (login, change password) use `page-narrow` on the `v-container`.

**`v-row` / `v-col` are not used in this codebase.** Column gutters stack with `mb-4` and
leave auto-width siblings flush. Do **not** rely on Vuetify `ga-*` / `gap-*` utilities
(unreliable across versions); use inline `style="gap: …"` or a theme cluster class.

Simple Cancel + Save pairs may keep `class="mr-2"` on the first button.

### Page Layout

- **Main content**: Layouts must **not** add padding around the slot. Use `v-main` with the slot as a direct child.
- **Page padding**: Each page uses `v-container` with class `page-content`. That is the single source of page margins — do not compound it with layout-level padding or `py-8`.

### Content Padding

- Card/dialog body padding comes from StandardCard / StandardDialog theme classes — do not add extra `pa-4` on `v-card-text`.
- For one-off dense regions outside those shells, prefer `pa-3 pa-sm-4` over inventing custom padding.

### Tables and Lists

- **Primary lists** (sources, users): `density="comfortable"` on `v-data-table`.
- **Screens and tour entries**: `v-expansion-panels` with `multiple`, `flat`, and `variant="accordion"` — never elevated panels inside a StandardCard.
- **Nested or auxiliary tables** (inside dialogs, secondary sections): `density="compact"`.
- **v-data-table**: Hide pagination and footer by default with an empty `#bottom` slot. Use `:items-per-page="50"`.
- **Actions column**: `align: 'end'` on the header; cell content in `d-flex justify-end align-center flex-nowrap` with `style="gap: 4px"` — **never wrap** row actions onto a second line. Use **icon buttons** for Edit / Delete (with `aria-label`); keep a short **text** button for uncommon verbs like Probe. Users may use a ⋮ overflow menu when actions need tooltips/guards. Drop secondary columns (e.g. Probe summary) below the mobile breakpoint rather than crushing the row. Wrap the table for horizontal scroll when needed.

### Expansion panels

- Always **`flat`** — elevated/shadowed panels look wrong nested in StandardCard and fight the flat card chrome.
- Prefer `variant="accordion"` for flush stacked rows.
- Use `multiple` when several items may be edited at once; start with all collapsed (`v-model` empty array). Do not set `mandatory`.
- Put reorder/delete controls in the title with `@click.stop`; leave the default expand chevron alone (do not replace `#actions` unless you re-render the expand icon).
- **Body inset** (theme): `.v-expansion-panel-text__wrapper` uses `16px` top/side/bottom (Vuetify’s `8px` top is too tight). When the body contains a `.v-field`, top becomes `20px` so outlined floating labels clear the title. Do not compensate with one-off `pt-*` on each panel.
- **Body wash** (theme): `.v-expansion-panel-text` gets a very light `on-surface` tint (`0.015` light / `0.025` dark) so expanded content reads as an inset region without elevation or a hard border.

### Border Radius

- **Standard cards** (StandardCard, StandardPageCard): `0` (flat, edge-to-edge)
- **Metric cards** (decoder health, fps): `8px`
- **Dialogs**: `0`

## Component Patterns

### App Header

- **Background**: Primary colour, solid or gradient.
- **User menu**: `density="comfortable"` on the dropdown `v-list`. `px-3` on the activator button, `mr-3` between avatar and text, `ml-2` before the chevron.
- **App bar icon buttons**: `px-2` for consistent touch targets.

### Cards

- `v-card` with the StandardCard chrome (gradient header). Nested cards use `brand-section-card`.
- Add a divider under `v-card-title` when using a raw `v-card`.
- **`v-card-text` has built-in padding** — do not add `pa-4` or similar.

### Buttons

- **Primary actions**: `variant="elevated"`, `color="primary"`
- **Secondary actions**: `variant="outlined"` or `variant="text"`
- **Button / control clusters — REQUIRED**: never place adjacent controls flush. Prefer `d-flex` + `style="gap: 8px"`; Cancel/Save pairs may use `mr-2` on the first button. Do not use Vuetify `ga-*`.
- **Form action buttons**:
  - Inline / expansion-panel / card editors: **`Save` only**, `:disabled` when clean. Label is always `Save`.
  - Dialogs: Cancel + primary; Cancel dismisses the dialog.

### Form Inputs

- **All inputs**: `variant="outlined"`, `density="compact"`, `hide-details="auto"`
- **Login/register**: may use `density="comfortable"` for accessibility
- **`autocomplete` — REQUIRED** (HTML / Vuetify tokens only — no readonly or decoy hacks):
  - Non-credential text fields: `autocomplete="off"`
  - **Login only:** `autocomplete="username"` and `autocomplete="current-password"`
  - **Create user / source credentials / other non-login secrets:** username `autocomplete="off"`; password `autocomplete="new-password"` (browsers ignore `"off"` on password fields)
  - **Change password:** current `autocomplete="current-password"`; new + confirm `autocomplete="new-password"`
- **Fixed-width inputs**: `style="max-width: 320px;"` (or 200/400 as appropriate) on page forms; dialog fields typically fill the dialog width
- **Duration inputs**: always labelled with their unit and stored in milliseconds. Display seconds where that reads better, but never leave the unit ambiguous.
- **Password inputs**: always `PasswordField` (`components/common/PasswordField.vue`) with a show/hide toggle. Never a raw `type="password"` field. Hidden by default; `aria-label` is `Show password` / `Hide password`.

### Checkboxes and switches

- Prefer **`v-switch` with `inset`** for boolean settings (Enabled, Loop, etc.). Props: `color="primary"`, `inset`, `density="compact"`, `hide-details="auto"`.
- Switch clusters: wrap in `d-flex align-center flex-wrap switch-cluster` (theme sets **16px** gap and pins each `.v-switch` to `flex: 0 0 auto` so spacing is visible). Do not rely on bare `style="gap: 8px"` around switches — Vuetify’s input flex shrink fights it.
- Checkboxes remain for multi-select lists only; use `density="compact"`.

### Section Spacing

- Between form fields: `mb-4`
- Between major sections: `mb-4` on the upper block (prefer over `mt-*`)
- Control clusters: buttons/chips `style="gap: 8px"`; **switch rows** use class `switch-cluster` (16px gap)
- Side-by-side fields: class `field-row`
- Simple two-button pairs: `mr-2` on the first **or** parent `gap: 8px`

### Feedback and Confirmations

**Never use `alert()` or `confirm()`.**

- **Success/error**: `v-alert` on the parent page, `density="compact"`, `class="mb-4"`, **above** the
  content it describes. Drive them from `useFeedback()`, which makes success transient and
  mutually exclusive with error — a success banner that nothing resets will sit there
  contradicting the next failure.
- **Confirmations**: `ConfirmDeleteDialog` for deletes; StandardDialog with Cancel + primary
  for anything else. An error inside a confirm dialog goes above the question, like every
  other alert.

```vue
<v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>

<ConfirmDeleteDialog
  v-model="showConfirm"
  title="Delete screen?"
  :name="screenToDelete?.name"
  :error="deleteError"
  :loading="deleting"
  @close="screenToDelete = null"
  @confirm="doDelete"
/>
```

#### Dialog primary-action verb

The primary button names the outcome: **Create** in a create dialog, **Save** when editing an
existing record, **Delete** in a confirm-delete. Do not label a create action `Save`.

### Links

- All links use the primary colour; never leave unstyled blue.

## Domain-Specific Components

These are specific to this application and have their own conventions.

### Layout Picker

- Renders each layout from the **normalised geometry returned by `GET /api/v1/layouts`** — never from hand-drawn CSS. If the picker and the engine ever disagree about what `1+5` means, that is a bug the shared geometry exists to prevent.
- Group options by family. The family list is **derived from the API response**, not hard-coded, so a family added server-side cannot silently hide its layouts. `full` (Full bleed) is the only single-cell layout — the old `1x1` was retired as a geometric duplicate.
- The selected layout gets a primary-coloured border, not a background fill; a fill fights the cell diagram.

### Tile Editor

- The layout diagram is the control. Cells are clickable and show the assigned source's thumbnail and name.
- Empty cells show a dashed border and an add icon — clearly a slot, not an error.
- Maintain the true aspect ratio of the output so the editor matches the wall.
- Per-cell fit mode is a small select inside the cell's popover, not a separate form section.

### Preview Canvas

- Fills the content column (no pixel max-width). Height follows the configured output aspect ratio.
- Renders in the output's aspect ratio with letterboxing, on a black surface.
- Shows an explicit notice when the display engine is not running.
- Never distort the preview to fill height independently of width. A stretched wall is worse than a correctly proportioned one.

The canvas shows the MJPEG stream when the engine is running, and falls back to the
scheduler's layout geometry with per-tile source names when it is not. Show a
`v-progress-circular` only while a stream that will start is connecting; when
`/api/v1/preview/stream` returns `engine_not_running`, switch to the layout diagram rather
than spinning forever. The endpoint serves at most four viewers and answers 429 with
`too_many_clients` beyond that, which the UI should surface as a message, not a retry loop.

### Status Chips

Decoder and source state use consistent colours everywhere they appear:

| State | Colour | Label |
|-------|--------|-------|
| Playing, hardware decode | `success` | `Hardware` |
| Playing, software decode | `warning` | `Software` |
| Connecting or reconnecting | `info` | `Connecting` |
| Offline or failed | `error` | `Offline` |
| Disabled | `grey` | `Disabled` |

Software decode is `warning` rather than `success` deliberately: on constrained hosts it is often the difference between a wall that works and one that does not, and it should be visible at a glance.

## Mobile Responsiveness

- **Breakpoints**: 0–599 (xs), 600 (sm), 960 (md), 1280 (lg)
- Prefer `pa-3 pa-sm-4` over fixed `pa-6` when adding padding outside StandardCard
- Use `useDisplay()` for programmatic breakpoint checks
- Minimum 44px touch targets
- The tile editor collapses to a vertical cell list below `sm`; a nine-cell diagram is not usable on a phone

## Theme and Global CSS

- **Avoid padding and margin overrides** in `theme.scss`. Use Vuetify's default component spacing and utility classes in templates. Overriding component padding globally fights the design system.
- Do not add padding to `v-card-text`; Vuetify provides it.

## Do Not

- **Use text fields without an explicit `autocomplete` attribute**
- **Place Add/Create buttons below tables or lists** — they belong in the section header, right-aligned
- **Use raw `v-dialog`** — use StandardDialog
- **Use elevated / non-`flat` expansion panels** inside StandardCard
- **Place adjacent buttons/switches without spacing** — use `style="gap: 8px"` or `mr-2`
- **Use `v-row`/`v-col`** anywhere — use `field-row`, `page-narrow`, or plain flex + `gap`
- **Rely on Vuetify `ga-*` utilities** for critical layout — use inline `style="gap: …"`
- **Use `alert()` or `confirm()`**
- **Hard-code layout geometry in the frontend** — always use the API's normalised rects
- **Poll for display state** — use the SSE stream
- **Use a raw `type="password"` field** — use `PasswordField` with show/hide
- Use raw HTML for forms, buttons, inputs, or cards — always Vuetify components
- Put form action buttons in the page header
- Use `hide-details` without `="auto"`
- **Set a success message that nothing clears** — use `useFeedback()`
- **Re-implement an empty state or a delete confirmation** — use `EmptyState` / `ConfirmDeleteDialog`
- Use decorative or custom fonts
- Use unstyled blue links
