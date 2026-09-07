/**
 * The single-cell full-frame layout. Mirrors `layout.FullBleedID` in Go and is
 * the only one-cell entry in the catalogue — it is what a new grid screen and a
 * transition screen's Preview both fall back to.
 *
 * Geometry always comes from `GET /api/v1/layouts`; only the id lives here.
 */
export const FULL_BLEED_ID = 'full'
