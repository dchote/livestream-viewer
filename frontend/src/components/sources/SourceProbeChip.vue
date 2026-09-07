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

const color = computed(() => {
  if (!props.source.enabled) return 'grey'
  const status = props.source.probe?.status
  if (status === 'ok') {
    if (props.source.probe?.hw_decode === true) return 'success'
    if (props.source.probe?.hw_decode === false) return 'warning'
    return 'info'
  }
  if (status === 'error') return 'error'
  return 'grey'
})

const label = computed(() => {
  if (!props.source.enabled) return 'Disabled'
  const status = props.source.probe?.status
  if (status === 'ok') {
    if (props.source.probe?.hw_decode === true) return 'Hardware'
    if (props.source.probe?.hw_decode === false) return 'Software'
    return props.source.probe?.codec || 'Probed'
  }
  if (status === 'error') return 'Offline'
  if (status === 'unavailable') return 'Unavailable'
  return 'Idle'
})
</script>
