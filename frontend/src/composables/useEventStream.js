import { onMounted, onUnmounted, reactive } from 'vue'
import { getIngressBase } from '@/utils/ingress'
import { getToken } from '@/utils/api'

let sharedSource = null
let subscribers = 0

export function useEventStream() {
  const events = reactive({ last: null, connected: false })

  function connect() {
    if (sharedSource) return
    const token = getToken()
    const params = token ? `?access_token=${encodeURIComponent(token)}` : ''
    const url = `${getIngressBase()}api/v1/events${params}`
    try {
      sharedSource = new EventSource(url)
      sharedSource.addEventListener('display.state', (ev) => {
        try {
          events.last = JSON.parse(ev.data)
        } catch {
          events.last = ev.data
        }
      })
      sharedSource.onopen = () => { events.connected = true }
      sharedSource.onerror = () => { events.connected = false }
    } catch (err) {
      console.log('[useEventStream] connect error:', err)
    }
  }

  onMounted(() => {
    subscribers += 1
    connect()
  })
  onUnmounted(() => {
    subscribers -= 1
    if (subscribers <= 0 && sharedSource) {
      sharedSource.close()
      sharedSource = null
      subscribers = 0
    }
  })

  return events
}
