<template>
  <div class="empty-state">
    <v-icon class="empty-state__icon">{{ icon }}</v-icon>
    <div class="text-h6 empty-state__title">{{ title }}</div>
    <p class="empty-state__copy">
      <slot>{{ copy }}</slot>
    </p>
  </div>
</template>

<script setup>
defineProps({
  icon: {
    type: String,
    required: true,
  },
  title: {
    type: String,
    required: true,
  },
  copy: {
    type: String,
    default: '',
  },
})
</script>
