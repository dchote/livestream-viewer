<template>
  <div>
    <div v-if="!items.length" class="text-body-2 text-medium-emphasis mb-4">
      No playlist items yet.
    </div>
    <div
      v-for="(item, i) in items"
      :key="i"
      class="field-row mb-4"
    >
      <v-select
        :model-value="item.source_id"
        :items="sourceItems"
        label="Source"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        @update:model-value="patch(i, { source_id: $event })"
      />
      <v-text-field
        :model-value="formatDwellSeconds(item.dwell_ms)"
        label="Dwell time (seconds)"
        type="number"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        style="max-width: 200px;"
        @update:model-value="patch(i, { dwell_ms: dwellMsFromSeconds($event) })"
      />
      <v-select
        :model-value="item.fit || 'contain'"
        :items="FIT_ITEMS"
        label="Fit"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        :disabled="!editable"
        style="max-width: 160px;"
        @update:model-value="patch(i, { fit: $event })"
      />
      <div
        v-if="editable"
        class="field-row__actions d-flex align-center flex-nowrap"
        style="gap: 4px"
      >
        <v-btn
          icon
          variant="text"
          size="small"
          :disabled="i === 0"
          aria-label="Move up"
          @click="move(i, -1)"
        >
          <v-icon size="small">mdi-arrow-up</v-icon>
        </v-btn>
        <v-btn
          icon
          variant="text"
          size="small"
          :disabled="i === items.length - 1"
          aria-label="Move down"
          @click="move(i, 1)"
        >
          <v-icon size="small">mdi-arrow-down</v-icon>
        </v-btn>
        <v-btn
          icon
          variant="text"
          size="small"
          color="error"
          aria-label="Remove item"
          @click="remove(i)"
        >
          <v-icon size="small">mdi-close</v-icon>
        </v-btn>
      </div>
    </div>
    <v-btn v-if="editable" variant="outlined" size="small" class="mb-4" @click="add">
      Add item
    </v-btn>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { FIT_ITEMS, dwellMsFromSeconds, formatDwellSeconds, selectItems } from '@/utils/formatters'

const props = defineProps({
  items: {
    type: Array,
    default: () => [],
  },
  sources: {
    type: Array,
    default: () => [],
  },
  editable: {
    type: Boolean,
    default: true,
  },
})

const emit = defineEmits(['update:items'])

const sourceItems = computed(() => selectItems(props.sources))

/** Clones the list and renumbers `position` to match array order. */
function renumbered(items) {
  return items.map((it, i) => ({ ...it, position: i }))
}

function emitNext(next) {
  emit('update:items', renumbered(next))
}

function patch(i, fields) {
  emitNext(props.items.map((it, idx) => (idx === i ? { ...it, ...fields } : it)))
}

function move(i, delta) {
  const next = [...props.items]
  const j = i + delta
  const tmp = next[i]
  next[i] = next[j]
  next[j] = tmp
  emitNext(next)
}

function remove(i) {
  emitNext(props.items.filter((_, idx) => idx !== i))
}

function add() {
  emitNext([
    ...props.items,
    { source_id: props.sources[0]?.id, dwell_ms: 15000, fit: 'contain' },
  ])
}
</script>
