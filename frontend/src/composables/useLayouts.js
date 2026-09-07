import { onMounted, reactive } from 'vue'
import { api } from '@/utils/api'

const state = reactive({
  layouts: [],
  loaded: false,
  error: null,
})

let inflight = null

export function useLayouts() {
  async function load() {
    if (state.loaded || inflight) return inflight
    inflight = api.get('api/v1/layouts')
      .then((data) => {
        state.layouts = data.layouts || []
        state.loaded = true
        state.error = null
      })
      .catch((err) => {
        console.log('[useLayouts] API error:', err)
        state.error = err.message || 'Failed to load layouts'
      })
      .finally(() => {
        inflight = null
      })
    return inflight
  }

  onMounted(load)
  return { state, load }
}
