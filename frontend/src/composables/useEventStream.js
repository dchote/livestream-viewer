import { onMounted, onUnmounted, reactive } from 'vue'
import { getIngressBase } from '@/utils/ingress'
import { getToken } from '@/utils/api'

let sharedSource = null
let subscribers = 0
let reconnectTimer = null
let backoff = 1000
const listeners = new Set()

function notify(patch) {
  for (const fn of listeners) fn(patch)
}

function connect() {
  if (sharedSource) return
  const token = getToken()
  const params = token ? `?access_token=${encodeURIComponent(token)}` : ''
  const url = `${getIngressBase()}api/v1/events${params}`
  try {
    sharedSource = new EventSource(url)
    sharedSource.addEventListener('display.state', (ev) => {
      try {
        notify({ last: JSON.parse(ev.data), connected: true })
      } catch {
        notify({ last: ev.data, connected: true })
      }
    })
    sharedSource.onopen = () => {
      backoff = 1000
      notify({ connected: true })
    }
    sharedSource.onerror = () => {
      notify({ connected: false })
      if (sharedSource) {
        sharedSource.close()
        sharedSource = null
      }
      if (subscribers > 0 && !reconnectTimer) {
        reconnectTimer = setTimeout(() => {
          reconnectTimer = null
          connect()
        }, backoff)
        backoff = Math.min(backoff * 2, 15000)
      }
    }
  } catch (err) {
    console.log('[useEventStream] connect error:', err)
  }
}

export function useEventStream() {
  const events = reactive({ last: null, connected: false })

  function apply(patch) {
    if (patch.last !== undefined) events.last = patch.last
    if (patch.connected !== undefined) events.connected = patch.connected
  }

  onMounted(() => {
    subscribers += 1
    listeners.add(apply)
    connect()
  })
  onUnmounted(() => {
    listeners.delete(apply)
    subscribers -= 1
    if (subscribers <= 0) {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      if (sharedSource) {
        sharedSource.close()
        sharedSource = null
      }
      subscribers = 0
    }
  })

  return events
}
