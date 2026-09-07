export function formatDwellSeconds(ms) {
  if (ms == null || Number.isNaN(Number(ms))) return ''
  return String(Math.round(Number(ms) / 1000))
}

export function dwellMsFromSeconds(value) {
  const n = Number(value)
  if (!Number.isFinite(n) || n <= 0) return 1000
  return Math.round(n * 1000)
}

/** Fit modes for a tile or playlist item. */
export const FIT_ITEMS = [
  { title: 'Contain', value: 'contain' },
  { title: 'Cover', value: 'cover' },
  { title: 'Fill', value: 'fill' },
]

/** Maps named entities (sources, screens) to `v-select` items. */
export function selectItems(list) {
  return (list || []).map((x) => ({ title: x.name, value: x.id }))
}

/** Maps named entities to an id → name lookup for captions and labels. */
export function nameByID(list) {
  const map = {}
  for (const x of list || []) map[x.id] = x.name
  return map
}

export function formatTransitionSummary(t) {
  if (!t || !t.type || t.type === 'cut') return 'Cut'
  const subtype = t.subtype ? ` · ${t.subtype}` : ''
  const dur = t.duration_ms ? ` · ${(t.duration_ms / 1000).toFixed(1)}s` : ''
  return `${t.type}${subtype}${dur}`
}

export function formatResolution(probe) {
  if (!probe || !probe.width || !probe.height) return ''
  return `${probe.width}×${probe.height}`
}

export function formatProbeSummary(probe) {
  if (!probe || !probe.status) return 'Not probed'
  if (probe.status === 'unavailable') return probe.message || 'Probe unavailable'
  if (probe.status === 'error') return probe.message || 'Probe failed'
  const bits = [probe.codec, formatResolution(probe)]
  if (probe.fps) bits.push(`${Number(probe.fps).toFixed(0)} fps`)
  return bits.filter(Boolean).join(' · ') || 'OK'
}

export function kindLabel(kind) {
  const map = {
    youtube: 'YouTube',
    rtsp: 'RTSP',
    hls: 'HLS',
    dash: 'DASH',
    http: 'HTTP',
    srt: 'SRT',
    rtmp: 'RTMP',
    file: 'File',
  }
  return map[kind] || kind
}
