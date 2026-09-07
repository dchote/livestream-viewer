import { onUnmounted, ref } from 'vue'

const SUCCESS_TIMEOUT_MS = 4000

/**
 * Page-level error / success alerts.
 *
 * Success is transient: it clears itself, so a stale confirmation can never sit
 * next to a later failure. The two are mutually exclusive for the same reason.
 */
export function useFeedback() {
  const error = ref('')
  const success = ref('')
  let timer = null

  function clearTimer() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  function showError(message) {
    clearTimer()
    success.value = ''
    error.value = message
  }

  function showSuccess(message) {
    clearTimer()
    error.value = ''
    success.value = message
    timer = setTimeout(() => {
      success.value = ''
      timer = null
    }, SUCCESS_TIMEOUT_MS)
  }

  function clear() {
    clearTimer()
    error.value = ''
    success.value = ''
  }

  onUnmounted(clearTimer)

  return { error, success, showError, showSuccess, clear }
}
