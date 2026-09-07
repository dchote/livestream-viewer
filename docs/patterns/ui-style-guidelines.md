# UI Style and Guidelines

> **Status:** Design. Not yet implemented.

This document defines the visual design standards for the livestream-viewer management frontend. All UI follows these guidelines for consistency.

## Design Philosophy

Material Design 3 palette, minimal and professional. This is an operations tool that may be left open on a second monitor, so it avoids flashy gradients and uses subtle depth where appropriate. Video content is the only thing on screen that should draw the eye.

## Layout and Structure Rules

### Headers

- **Vertical alignment**: All header content must be vertically centre-aligned. Use `d-flex align-center` on header rows.
- **Action buttons in headers**: Action buttons for tabular or list data (Add, Create, Edit) live in that section's header, right-aligned. Use `v-spacer` before them.
- **Section headers**: Each data section (Sources, Screens, Tour) has a header row with the title on the left and its primary action button right-aligned.

### Navigation Pattern

- **List → Detail**: Primary entities (sources, screens) are listed on their settings page. Selecting one opens its editor.
- **Edit in detail header**: A detail view's header carries a right-aligned Edit button or action menu.

### Forms

- Use `mb-4` between form elements.
- Form actions (Save, Cancel) go at the bottom of the form, right-aligned: `d-flex justify-end mt-4`. Use `mr-2` on the first button.

### Dialogs

- All dialogs use **StandardDialog** with title, content slot, and actions slot. Never raw `v-dialog`.
- Actions slot: `v-spacer`, then Cancel and the primary action. `mr-2` on the first button.

## Color Palette

Defined in `frontend/src/styles/theme.scss` and `frontend/src/plugins/vuetify.js`.

- **Primary**: Material Blue (#1976D2)
- **Secondary**: Grey (#424242)
- **Accent/Info**: Blue (#2196F3)
- **Success**: Green (#4CAF50)
- **Warning**: Orange (#FF9800)
- **Error**: Red (#F44336)
- **Background/Surface**: Theme-driven (light grey / dark)

**Dark theme is the default.** This application is normally used to configure a screen in a room, often a dim one, and it displays video thumbnails that read better against a dark surface.

## Typography

- **Font family**: System stack — `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif`
- **No decorative or custom fonts**
- **Card headers**: `text-h5` (1.5rem) — avoid `text-h4`, which is too large
- **Card titles**: `font-weight: 600`
- Use Vuetify typography classes: `text-h5`, `text-h6`, `text-body-1`, `text-caption`

## Spacing and Layout

### Page Layout

- **Main content**: Layouts must **not** add padding around the slot. Use `v-main` with the slot as a direct child.
- **Page padding**: Each page uses `v-container`, which provides responsive horizontal padding and max width. This is the single source of page margins — do not compound it with layout-level padding.

### Content Padding

- Use responsive padding: `pa-3 pa-sm-6` or `pa-3 pa-sm-4`.

### Tables and Lists

- **Primary lists** (sources, screens, tour entries): `density="comfortable"`.
- **Nested or auxiliary tables** (inside dialogs, secondary sections): `density="compact"`.
- **v-data-table**: Hide pagination and footer by default with an empty `#bottom` slot. Use `:items-per-page="50"`.

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

- `v-card` with `variant="outlined"` or `variant="flat"`.
- Add a divider under `v-card-title` when using a raw `v-card`.
- **`v-card-text` has built-in padding** — do not add `pa-4` or similar.

### Buttons

- **Primary actions**: `variant="elevated"`, `color="primary"`
- **Secondary actions**: `variant="outlined"` or `variant="text"`
- **Button groups — REQUIRED**: **NEVER** place adjacent buttons without spacing. Add `class="mr-2"` to the first/left button. Do NOT use `gap`.
- **Form action buttons**: bottom of the form, right-aligned, in a `d-flex justify-end mt-4` wrapper.

### Form Inputs

- **All inputs**: `variant="outlined"`, `density="compact"`, `hide-details="auto"`
- **Login/register**: may use `density="comfortable"` for accessibility
- **`autocomplete` — REQUIRED**:
  - Non-auth text fields and textareas: always `autocomplete="off"`
  - Auth forms only: `autocomplete="username"`, `"current-password"`, or `"new-password"`
- **Fixed-width inputs**: `style="max-width: 320px;"` (or 200/400 as appropriate)
- **Duration inputs**: always labelled with their unit and stored in milliseconds. Display seconds where that reads better, but never leave the unit ambiguous.

### Checkboxes

- Always `density="compact"`
- Add `class="checkbox-compact"` for minimal padding

### Section Spacing

- Between form fields: `mb-4`
- Above button groups: `mt-4`
- Between buttons: `mr-2` on the first
- Between chips: `mr-2 mb-2` on each

### Feedback and Confirmations

**Never use `alert()` or `confirm()`.**

- **Success/error**: `v-alert` on the parent page, `density="compact"`, `class="mb-4"`
- **Confirmations**: StandardDialog with Cancel and the primary action

```vue
<v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>

<StandardDialog v-model="showConfirm" title="Delete screen?" max-width="400" @close="screenToDelete = null">
  <p>Are you sure?</p>
  <template #actions>
    <v-spacer />
    <v-btn variant="text" class="mr-2" @click="showConfirm = false">Cancel</v-btn>
    <v-btn color="error" variant="elevated" @click="doDelete">Delete</v-btn>
  </template>
</StandardDialog>
```

### Links

- All links use the primary colour; never leave unstyled blue.

## Domain-Specific Components

These are specific to this application and have their own conventions.

### Layout Picker

- Renders each layout from the **normalised geometry returned by `GET /api/v1/layouts`** — never from hand-drawn CSS. If the picker and the engine ever disagree about what `1+5` means, that is a bug the shared geometry exists to prevent.
- Group options by family (Equal, Hotspot, Vertical, Panoramic) with a `v-item-group`.
- The selected layout gets a primary-coloured border, not a background fill; a fill fights the cell diagram.

### Tile Editor

- The layout diagram is the control. Cells are clickable and show the assigned source's thumbnail and name.
- Empty cells show a dashed border and an add icon — clearly a slot, not an error.
- Maintain the true aspect ratio of the output so the editor matches the wall.
- Per-cell fit mode is a small select inside the cell's popover, not a separate form section.

### Preview Canvas

- Renders in the output's aspect ratio with letterboxing, on a black surface.
- Shows a `v-progress-circular` while the first frame is loading, and an explicit notice if the display engine is not running.
- Never stretch the preview to fill its container. A distorted preview of a wall is worse than a small accurate one.

### Status Chips

Decoder and source state use consistent colours everywhere they appear:

| State | Colour | Label |
|-------|--------|-------|
| Playing, hardware decode | `success` | `Hardware` |
| Playing, software decode | `warning` | `Software` |
| Connecting or reconnecting | `info` | `Connecting` |
| Offline or failed | `error` | `Offline` |
| Disabled | `grey` | `Disabled` |

Software decode is `warning` rather than `success` deliberately: on a Pi 5 it is the difference between a wall that works and one that does not, and it should be visible at a glance.

## Mobile Responsiveness

- **Breakpoints**: 0–599 (xs), 600 (sm), 960 (md), 1280 (lg)
- Prefer `pa-3 pa-sm-6` over fixed `pa-6`
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
- **Use `gap` or `gap-*`** — use explicit margins (`mr-2`, `mb-2`)
- **Place adjacent buttons without spacing** — `mr-2` on the first
- **Use `alert()` or `confirm()`**
- **Hard-code layout geometry in the frontend** — always use the API's normalised rects
- **Poll for display state** — use the SSE stream
- Use raw HTML for forms, buttons, inputs, or cards — always Vuetify components
- Put form action buttons in the page header
- Use `hide-details` without `="auto"`
- Use decorative or custom fonts
- Use unstyled blue links
