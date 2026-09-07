<template>
  <div>
    <div class="field-row mb-4">
      <v-select
        :model-value="modelValue.type"
        :items="typeItems"
        label="Type"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        @update:model-value="setType"
      />
      <v-select
        v-if="subtypes.length"
        :model-value="modelValue.subtype"
        :items="subtypes"
        label="Subtype"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        @update:model-value="patch({ subtype: $event })"
      />
    </div>
    <div v-if="modelValue.type !== 'cut'" class="field-row mb-4">
      <v-text-field
        :model-value="Math.round((modelValue.duration_ms || 0) / 100) / 10"
        label="Duration (seconds)"
        type="number"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        style="max-width: 200px;"
        @update:model-value="patch({ duration_ms: Math.max(1, Math.round(Number($event) * 1000)) })"
      />
      <v-select
        :model-value="easingChoice"
        :items="easingItems"
        label="Easing"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        @update:model-value="setEasing"
      />
    </div>
    <v-text-field
      v-if="easingChoice === 'custom'"
      :model-value="modelValue.easing"
      label="cubic-bezier(x1, y1, x2, y2)"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      :disabled="!editable"
      class="mb-4"
      style="max-width: 320px;"
      @update:model-value="patch({ easing: $event })"
    />
    <v-text-field
      v-if="needsColor"
      :model-value="modelValue.color || '#000000'"
      label="Color"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      :disabled="!editable"
      class="mb-4"
      style="max-width: 200px;"
      @update:model-value="patch({ color: $event })"
    />
    <div v-if="curve && modelValue.type !== 'cut'" class="easing-preview" :style="{ '--ease': curve }" />
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  modelValue: {
    type: Object,
    default: () => ({ type: 'cut', duration_ms: 0 }),
  },
  catalogue: {
    type: Array,
    default: () => [],
  },
  presets: {
    type: Array,
    default: () => [],
  },
  editable: {
    type: Boolean,
    default: true,
  },
})

const emit = defineEmits(['update:modelValue'])

const typeItems = computed(() => props.catalogue.filter((t) => t.available !== false).map((t) => ({
  title: t.type,
  value: t.type,
})))

const selectedType = computed(() => props.catalogue.find((t) => t.type === props.modelValue.type))
const subtypes = computed(() => selectedType.value?.subtypes || [])
const needsColor = computed(() => props.modelValue.subtype === 'fadeToColor' || props.modelValue.subtype === 'fadeFromColor')

const easingItems = computed(() => [
  ...props.presets.map((p) => ({ title: p.name, value: p.name })),
  { title: 'Custom cubic-bezier', value: 'custom' },
])

const easingChoice = computed(() => {
  const e = props.modelValue.easing || 'ease-in-out'
  if (props.presets.some((p) => p.name === e)) return e
  if (e.startsWith('cubic-bezier')) return 'custom'
  return e
})

const curve = computed(() => {
  const e = props.modelValue.easing || 'ease-in-out'
  const preset = props.presets.find((p) => p.name === e)
  if (preset) return `cubic-bezier(${preset.points.join(',')})`
  if (e.startsWith('cubic-bezier')) return e
  return 'linear'
})

function patch(fields) {
  emit('update:modelValue', { ...props.modelValue, ...fields })
}

function setType(type) {
  const info = props.catalogue.find((t) => t.type === type)
  const next = {
    ...props.modelValue,
    type,
    subtype: info?.default_subtype || '',
    duration_ms: type === 'cut' ? 0 : (props.modelValue.duration_ms || 600),
  }
  if (type === 'cut') next.easing = ''
  emit('update:modelValue', next)
}

function setEasing(v) {
  if (v === 'custom') {
    patch({ easing: 'cubic-bezier(0.42, 0, 0.58, 1)' })
    return
  }
  patch({ easing: v })
}
</script>

<style scoped>
.easing-preview {
  width: 160px;
  height: 48px;
  background: linear-gradient(90deg, rgb(var(--v-theme-primary)), transparent);
  animation: slide 1.2s var(--ease) infinite alternate;
}
@keyframes slide {
  from { transform: translateX(0); }
  to { transform: translateX(80px); }
}
</style>
