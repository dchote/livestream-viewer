<template>
  <div>
    <div class="d-flex align-center flex-wrap mb-4" style="gap: 8px">
      <v-btn
        v-for="f in families"
        :key="f.id"
        size="small"
        :variant="family === f.id ? 'elevated' : 'outlined'"
        color="primary"
        :disabled="!editable"
        @click="family = f.id"
      >
        {{ f.label }}
      </v-btn>
    </div>
    <div class="d-flex flex-wrap mb-4" style="gap: 8px">
      <v-btn
        v-for="layout in filtered"
        :key="layout.id"
        variant="text"
        class="layout-option pa-1"
        :class="{ 'layout-option--selected': modelValue === layout.id }"
        height="auto"
        :disabled="!editable"
        @click="editable && $emit('update:modelValue', layout.id)"
      >
        <span class="d-block text-left">
          <LayoutDiagram class="mb-1" :rects="layout.rects" :aspect="aspect" />
          <span class="text-caption d-block">{{ layout.name }}</span>
        </span>
      </v-btn>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import LayoutDiagram from '@/components/display/LayoutDiagram.vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: '',
  },
  layouts: {
    type: Array,
    default: () => [],
  },
  aspect: {
    type: Number,
    default: 16 / 9,
  },
  editable: {
    type: Boolean,
    default: true,
  },
})

defineEmits(['update:modelValue'])

/**
 * Display names for the families the API returns. The list of families itself
 * is derived from the catalogue, not hard-coded: a family added server-side
 * would otherwise silently drop its layouts from the picker.
 */
const FAMILY_LABELS = {
  full_bleed: 'Full bleed',
  equal: 'Equal',
  hotspot: 'Hotspot',
  vertical: 'Vertical',
  panoramic: 'Panoramic',
}

function familyLabel(id) {
  return FAMILY_LABELS[id] || id.replace(/_/g, ' ')
}

const families = computed(() => {
  const seen = []
  for (const l of props.layouts) {
    if (l.family && !seen.includes(l.family)) seen.push(l.family)
  }
  return seen.map((id) => ({ id, label: familyLabel(id) }))
})

const family = ref('')

watch(
  () => [props.modelValue, props.layouts],
  () => {
    const selected = props.layouts.find((l) => l.id === props.modelValue)
    if (selected?.family) {
      family.value = selected.family
    } else if (!families.value.some((f) => f.id === family.value)) {
      family.value = families.value[0]?.id || ''
    }
  },
  { immediate: true },
)

const filtered = computed(() => props.layouts.filter((l) => l.family === family.value))
</script>

<style scoped>
.layout-option {
  width: 140px;
  min-width: 140px;
  border: 2px solid transparent;
  text-transform: none;
  letter-spacing: normal;
}
.layout-option--selected {
  border-color: rgb(var(--v-theme-primary));
}
</style>
