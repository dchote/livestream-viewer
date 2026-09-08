<template>
  <!-- No margin: spacing belongs to the container, not to a chip that sits in a table cell. -->
  <v-chip
    size="small"
    :color="color"
    variant="flat"
  >
    {{ label }}
  </v-chip>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  source: {
    type: Object,
    required: true,
  },
})

const live = computed(() => decoderChip(props.source))

const color = computed(() => live.value.color)
const label = computed(() => live.value.label)

function decoderChip(src) {
  if (!src.enabled) return { color: 'grey', label: 'Disabled' }
  if (isYouTubeBotCheck(src)) return { color: 'error', label: 'Bot check' }
  if (isYouTubeAuth(src)) return { color: 'error', label: 'Sign in required' }
  switch (src.decoder) {
    case 'connecting':
    case 'reconnecting':
      return { color: 'info', label: src.decoder === 'connecting' ? 'Connecting' : 'Reconnecting' }
    case 'hardware':
      return { color: 'success', label: 'Hardware' }
    case 'software':
      return { color: 'warning', label: 'Software' }
    case 'failed':
      return { color: 'error', label: 'Failed' }
    case 'offline':
      return { color: 'error', label: 'Offline' }
    case 'disabled':
      return { color: 'grey', label: 'Disabled' }
  }
  const status = src.probe?.status
  if (status === 'ok') {
    if (src.probe?.hw_decode === true) return { color: 'success', label: 'Hardware' }
    if (src.probe?.hw_decode === false) return { color: 'warning', label: 'Software' }
    return { color: 'info', label: src.probe?.codec || 'Probed' }
  }
  if (status === 'error' || src.decoder === 'offline') return { color: 'error', label: 'Offline' }
  if (status === 'unavailable') return { color: 'grey', label: 'Unavailable' }
  return { color: 'grey', label: 'Idle' }
}

function isYouTubeBotCheck(src) {
  return src.error_code === 'youtube_bot_check' || src.probe?.code === 'youtube_bot_check'
}

function isYouTubeAuth(src) {
  return src.error_code === 'youtube_auth' || src.probe?.code === 'youtube_auth'
}
</script>
