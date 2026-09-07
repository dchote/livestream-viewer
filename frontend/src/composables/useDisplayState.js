import { onMounted, reactive } from 'vue'
import { api } from '@/utils/api'

export function useDisplayState() {
  const display = reactive({
    display_running: false,
    paused: true,
    pinned: false,
    active_screen: null,
    next_screen: null,
    dwell_remaining_ms: 0,
    fps: 0,
    tiles: [],
    error: null,
  })

  async function seed() {
    try {
      const data = await api.get('api/v1/display/state')
      Object.assign(display, data)
    } catch (err) {
      console.log('[useDisplayState] API error:', err)
      display.error = err.message
    }
  }

  onMounted(seed)
  return { display, seed }
}
