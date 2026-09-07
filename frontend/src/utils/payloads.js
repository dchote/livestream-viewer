/**
 * Canonical write shapes for the display strategy.
 *
 * The dirty check and the request body must be built by the same function.
 * When they drift, Save enables on one shape and sends another.
 */

export const DEFAULT_CUT = { type: 'cut', duration_ms: 0 }

export function screenPayload(screen) {
  return {
    name: screen.name,
    kind: screen.kind,
    layout: screen.layout ?? null,
    loop: !!screen.loop,
    transition: screen.transition || { ...DEFAULT_CUT },
    tiles: screen.tiles || [],
    items: screen.items || [],
  }
}

export function tourPayload(tour) {
  return {
    enabled: !!tour.enabled,
    loop: !!tour.loop,
    entries: (tour.entries || []).map((e, i) => ({
      position: i,
      screen_id: e.screen_id,
      dwell_ms: e.dwell_ms,
      transition: e.transition || { ...DEFAULT_CUT },
    })),
  }
}

/** True when the draft differs from the saved baseline in its write shape. */
export function isDirty(draft, baseline, shape) {
  if (!draft || !baseline) return false
  return JSON.stringify(shape(draft)) !== JSON.stringify(shape(baseline))
}
