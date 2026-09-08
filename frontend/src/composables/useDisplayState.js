import { onMounted, reactive, watch } from 'vue'
import { api } from '@/utils/api'
import { useEventStream } from '@/composables/useEventStream'

export function useDisplayState() {
  const display = reactive({
    display_running: false,
    display_enabled: false,
    paused: true,
    pinned: false,
    active_screen: null,
    active_screen_name: '',
    next_screen: null,
    dwell_remaining_ms: 0,
    fps: 0,
    dropped_frames: 0,
    degradations: [],
    tiles: [],
    decoders: [],
    error: null,
  })

  const events = useEventStream()

  async function seed() {
    try {
      const data = await api.get('api/v1/display/state')
      Object.assign(display, data)
      display.error = null
    } catch (err) {
      console.log('[useDisplayState] API error:', err)
      display.error = err.message
    }
  }

  watch(() => events.last, (payload) => {
    if (payload && typeof payload === 'object') {
      Object.assign(display, payload)
    }
  })

  onMounted(seed)
  return { display, seed, events }
}
