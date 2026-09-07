<template>
  <div>
    <div v-if="!mobile" class="mb-4">
      <LayoutDiagram
        :rects="rects"
        :aspect="aspect"
        :clickable="editable"
        :selected-index="selected"
        :empty="emptyMap"
        @select="onSelectCell"
      >
        <template #cell="{ index }">
          <div class="text-center text-caption">
            <div v-if="tileLabel(index)">{{ tileLabel(index) }}</div>
            <v-icon v-else size="small">mdi-plus</v-icon>
          </div>
        </template>
      </LayoutDiagram>
    </div>
    <v-list v-else density="comfortable" class="mb-4" bg-color="transparent">
      <v-list-item
        v-for="(_rect, i) in rects"
        :key="i"
        :active="selected === i"
        color="primary"
        @click="onSelectCell(i)"
      >
        <v-list-item-title>Tile {{ i }}</v-list-item-title>
        <v-list-item-subtitle>{{ tileLabel(i) || 'Empty' }}</v-list-item-subtitle>
      </v-list-item>
    </v-list>

    <div v-if="selected >= 0 && current">
      <div class="text-subtitle-2 mb-4">Tile {{ selected }}</div>
      <div class="field-row mb-4">
        <v-select
          :model-value="assignment"
          :items="modes"
          label="Assignment"
          variant="outlined"
          density="compact"
          hide-details="auto"
          autocomplete="off"
          :disabled="!editable"
          @update:model-value="setAssignment"
        />
        <v-select
          v-if="assignment === 'source'"
          :model-value="current.source_id"
          :items="sourceItems"
          label="Source"
          variant="outlined"
          density="compact"
          hide-details="auto"
          autocomplete="off"
          clearable
          :disabled="!editable"
          @update:model-value="setSource"
        />
        <v-select
          :model-value="current.fit || 'contain'"
          :items="FIT_ITEMS"
          label="Fit"
          variant="outlined"
          density="compact"
          hide-details="auto"
          autocomplete="off"
          :disabled="!editable"
          style="max-width: 160px;"
          @update:model-value="setFit"
        />
      </div>
      <template v-if="assignment === 'sequence'">
        <div
          v-for="(seq, si) in current.sequence || []"
          :key="si"
          class="field-row mb-4"
        >
          <v-select
            :model-value="seq.source_id"
            :items="sourceItems"
            label="Source"
            variant="outlined"
            density="compact"
            hide-details="auto"
            autocomplete="off"
            :disabled="!editable"
            @update:model-value="setSeqSource(si, $event)"
          />
          <v-text-field
            :model-value="formatDwellSeconds(seq.dwell_ms)"
            label="Dwell time (seconds)"
            type="number"
            variant="outlined"
            density="compact"
            hide-details="auto"
            autocomplete="off"
            :disabled="!editable"
            style="max-width: 200px;"
            @update:model-value="setSeqDwell(si, $event)"
          />
          <div v-if="editable" class="field-row__actions d-flex align-center">
            <v-btn
              icon
              variant="text"
              size="small"
              color="error"
              aria-label="Remove sequence item"
              @click="removeSeq(si)"
            >
              <v-icon size="small">mdi-close</v-icon>
            </v-btn>
          </div>
        </div>
        <v-btn v-if="editable" variant="outlined" size="small" class="mb-4" @click="addSeq">
          Add sequence item
        </v-btn>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import LayoutDiagram from '@/components/display/LayoutDiagram.vue'
import {
  FIT_ITEMS,
  dwellMsFromSeconds,
  formatDwellSeconds,
  nameByID,
  selectItems,
} from '@/utils/formatters'

const props = defineProps({
  tiles: {
    type: Array,
    default: () => [],
  },
  rects: {
    type: Array,
    default: () => [],
  },
  sources: {
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

const emit = defineEmits(['update:tiles'])
const { mobile } = useDisplay()
const selected = ref(0)
/** Local assignment UI state so "Source" stays selected before a source_id is chosen. */
const assignment = ref('empty')
const modes = [
  { title: 'Empty', value: 'empty' },
  { title: 'Source', value: 'source' },
  { title: 'Sequence', value: 'sequence' },
]

const local = computed({
  get: () => props.tiles,
  set: (v) => emit('update:tiles', v),
})

const sourceItems = computed(() => selectItems(props.sources))
const sourceName = computed(() => nameByID(props.sources))

const current = computed(() => {
  const found = local.value.find((t) => t.index === selected.value)
  if (found) return found
  if (selected.value >= 0 && selected.value < props.rects.length) {
    return { index: selected.value, source_id: null, fit: 'contain', sequence: [] }
  }
  return null
})

function assignmentFromTile(t) {
  if (!t) return 'empty'
  if ((t.sequence || []).length) return 'sequence'
  if (t.source_id) return 'source'
  return 'empty'
}

function syncAssignmentFromTile() {
  assignment.value = assignmentFromTile(current.value)
}

function onSelectCell(i) {
  selected.value = i
  syncAssignmentFromTile()
}

watch(
  () => [selected.value, props.tiles],
  () => {
    if (assignment.value === 'source' || assignment.value === 'sequence') {
      const inferred = assignmentFromTile(current.value)
      if (inferred !== 'empty') assignment.value = inferred
      return
    }
    syncAssignmentFromTile()
  },
  { deep: true },
)

function setAssignment(v) {
  if (!props.editable || v === assignment.value) return
  assignment.value = v
  updateTile((t) => {
    if (v === 'empty') {
      t.source_id = null
      t.sequence = []
    } else if (v === 'source') {
      t.sequence = []
    } else if (v === 'sequence') {
      t.source_id = null
      if (!t.sequence?.length) {
        t.sequence = [{ position: 0, source_id: props.sources[0]?.id, dwell_ms: 8000 }]
      }
    }
  })
}

const emptyMap = computed(() => {
  const m = {}
  props.rects.forEach((_, i) => {
    const t = local.value.find((x) => x.index === i)
    m[i] = !t || (!t.source_id && !(t.sequence || []).length)
  })
  return m
})

watch(() => props.rects.length, () => {
  if (selected.value >= props.rects.length) selected.value = 0
  syncAssignmentFromTile()
})

function tileLabel(i) {
  const t = local.value.find((x) => x.index === i)
  if (!t) return ''
  if ((t.sequence || []).length) return `Sequence (${t.sequence.length})`
  if (t.source_id) return sourceName.value[t.source_id] || `#${t.source_id}`
  return ''
}

function copyTiles() {
  return local.value.map((t) => ({
    ...t,
    sequence: (t.sequence || []).map((s) => ({ ...s })),
  }))
}

function updateTile(fn) {
  const next = copyTiles()
  let t = next.find((x) => x.index === selected.value)
  if (!t) {
    t = { index: selected.value, source_id: null, fit: 'contain', sequence: [] }
    next.push(t)
  }
  fn(t)
  emit('update:tiles', next)
}

function setSource(id) {
  assignment.value = id ? 'source' : 'empty'
  updateTile((t) => {
    t.source_id = id || null
    t.sequence = []
  })
}

function setFit(fit) {
  updateTile((t) => { t.fit = fit })
}

function setSeqSource(i, id) {
  updateTile((t) => {
    t.sequence[i].source_id = id
  })
}

function setSeqDwell(i, seconds) {
  updateTile((t) => {
    t.sequence[i].dwell_ms = dwellMsFromSeconds(seconds)
  })
}

function addSeq() {
  assignment.value = 'sequence'
  updateTile((t) => {
    t.sequence = t.sequence || []
    t.sequence.push({ position: t.sequence.length, source_id: props.sources[0]?.id, dwell_ms: 8000 })
  })
}

function removeSeq(i) {
  updateTile((t) => {
    t.sequence.splice(i, 1)
    t.sequence.forEach((s, idx) => { s.position = idx })
    if (!t.sequence.length) assignment.value = 'empty'
  })
}

syncAssignmentFromTile()
</script>
