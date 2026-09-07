# Frontend Styling Requirements

**Reference**: For full styling patterns, typography, spacing, and component conventions, see `@docs/patterns/ui-style-guidelines.md`. For component structure, routing, and live state, see `@docs/patterns/frontend-guide.md`.

## Form Input Properties

**MANDATORY** for form inputs:

- `variant="outlined"`
- `density="compact"` (login/register may use `comfortable` for accessibility)
- `hide-details="auto"`
- `autocomplete="off"` on every non-auth text field
- `class="mr-2"` for spacing between inputs
- `style="max-width: 320px;"` for fixed-width inputs (adjust as needed)

## Button Spacing

**CRITICAL**: Adjacent buttons MUST have spacing. Add `class="mr-2"` to the first/left button. Never use `gap`.

## Feedback and Confirmations

- **Success/error**: Use `v-alert` on the parent page with `density="compact"` and `class="mb-4"`
- **Confirmations**: Use `StandardDialog` — never `confirm()`, never raw `v-dialog`

## Project-Specific

- **Layout geometry comes from the API.** Render layout pickers and tile editors from the normalised rects in `GET /api/v1/layouts`. Never hard-code cell positions in CSS or component state.
- **Live state comes from the SSE stream** via `useEventStream()`. Never poll.
- **Build every URL through `getIngressBase()`**, including the MJPEG preview `<img src>`, which is easy to miss because it does not go through the API client.
- **Never distort the preview.** Render it in the output's aspect ratio with letterboxing.
- **Software decode is a `warning` chip, not `success`.** On a Pi 5 it is the difference between a wall that works and one that does not.

## Summary

**DO:**
- Use explicit margins (`mr-2`, `mb-2`) — never `gap`
- Use `density="compact"` for form inputs
- Use `hide-details="auto"` on all inputs
- Add `mr-2` between adjacent buttons

**DON'T:**
- Use `gap` or `gap-*` classes
- Use `alert()` or `confirm()`
- Place adjacent buttons without spacing
- Hard-code layout geometry
- Poll for display state
