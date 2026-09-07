<template>
  <div>
    <EmptyState
      v-if="!screens.length"
      icon="mdi-view-grid-plus"
      title="No screens defined yet"
      copy="Create a grid or transition screen to compose the wall."
    />

    <v-expansion-panels
      v-else
      v-model="opened"
      multiple
      flat
      variant="accordion"
    >
      <v-expansion-panel
        v-for="screen in screens"
        :key="screen.id"
        :value="screen.id"
        :data-panel-key="screen.id"
        :class="{ 'panel-flash': highlighted === screen.id }"
      >
        <v-expansion-panel-title>
          <div class="d-flex align-center flex-grow-1 flex-wrap pe-2" style="gap: 8px; min-width: 0;">
            <span class="text-subtitle-1 text-truncate">{{ draftName(screen.id) }}</span>
            <span class="text-body-2 text-medium-emphasis text-truncate">
              {{ summary(screen) }}
            </span>
            <v-spacer />
            <v-btn
              v-if="editable"
              icon
              variant="text"
              size="small"
              color="error"
              aria-label="Delete screen"
              @click.stop="$emit('delete', screen)"
            >
              <v-icon size="small">mdi-delete</v-icon>
            </v-btn>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <template v-if="drafts[screen.id]">
            <div class="mb-4">
              <v-text-field
                v-model="drafts[screen.id].name"
                label="Name"
                variant="outlined"
                density="compact"
                hide-details="auto"
                autocomplete="off"
                :disabled="!editable"
                style="max-width: 320px;"
              />
            </div>
            <template v-if="drafts[screen.id].kind === 'grid'">
              <LayoutPicker
                v-model="drafts[screen.id].layout"
                :layouts="layouts"
                :aspect="aspect"
                :editable="editable"
                @update:model-value="(id) => onLayoutChange(screen.id, id)"
              />
              <TileEditor
                class="mb-4"
                :tiles="drafts[screen.id].tiles || []"
                :rects="rectsFor(drafts[screen.id].layout)"
                :sources="sources"
                :aspect="aspect"
                :editable="editable"
                @update:tiles="drafts[screen.id].tiles = $event"
              />
            </template>
            <template v-else>
              <div class="mb-4">
                <v-switch
                  v-model="drafts[screen.id].loop"
                  label="Loop playlist"
                  color="primary"
                  inset
                  density="compact"
                  hide-details="auto"
                  :disabled="!editable"
                />
              </div>
              <PlaylistEditor
                :items="drafts[screen.id].items || []"
                :sources="sources"
                :editable="editable"
                @update:items="drafts[screen.id].items = $event"
              />
              <div class="text-subtitle-2 mb-4">Transition between items</div>
              <TransitionEditor
                v-model="drafts[screen.id].transition"
                :catalogue="catalogue"
                :presets="presets"
                :editable="editable"
              />
            </template>
            <div
              v-if="editable"
              class="d-flex justify-end flex-wrap"
              style="gap: 8px"
            >
              <v-btn
                color="primary"
                variant="elevated"
                :loading="savingId === screen.id"
                :disabled="!isDirty(screen.id)"
                @click="$emit('save', drafts[screen.id])"
              >
                Save
              </v-btn>
            </div>
          </template>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </div>
</template>

<script setup>
import { nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LayoutPicker from '@/components/display/LayoutPicker.vue'
import TileEditor from '@/components/display/TileEditor.vue'
import PlaylistEditor from '@/components/display/PlaylistEditor.vue'
import TransitionEditor from '@/components/display/TransitionEditor.vue'
import { DEFAULT_CUT, isDirty as shapesDiffer, screenPayload } from '@/utils/payloads'

const HIGHLIGHT_MS = 2000

const props = defineProps({
  screens: {
    type: Array,
    default: () => [],
  },
  sources: {
    type: Array,
    default: () => [],
  },
  layouts: {
    type: Array,
    default: () => [],
  },
  aspect: {
    type: Number,
    default: 16 / 9,
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
  savingId: {
    type: [Number, String],
    default: null,
  },
})

defineEmits(['save', 'delete'])

/** Open panel ids; empty = all collapsed. Multiple may be open. */
const opened = ref([])
const drafts = reactive({})
const highlighted = ref(null)
let highlightTimer = null

watch(
  () => props.screens,
  (list) => {
    const ids = new Set(list.map((s) => s.id))
    for (const key of Object.keys(drafts)) {
      const id = Number(key)
      if (!ids.has(id)) delete drafts[id]
    }
    for (const screen of list) {
      // Refresh draft from server when not currently expanded (preserve in-progress edits).
      if (!opened.value.includes(screen.id) || !drafts[screen.id]) {
        drafts[screen.id] = cloneScreen(screen)
      }
    }
    opened.value = opened.value.filter((id) => ids.has(id))
  },
  { immediate: true, deep: true },
)

// Collapsing a panel discards unsaved edits (no Cancel button).
watch(opened, (next, prev) => {
  if (!prev?.length) return
  for (const id of prev) {
    if (!next.includes(id)) syncDraft(id)
  }
})

onUnmounted(() => {
  if (highlightTimer) clearTimeout(highlightTimer)
})

function cloneScreen(screen) {
  const d = JSON.parse(JSON.stringify(screen))
  if (!d.transition) d.transition = { ...DEFAULT_CUT }
  if (!d.tiles) d.tiles = []
  if (!d.items) d.items = []
  return d
}

function isDirty(id) {
  return shapesDiffer(drafts[id], props.screens.find((s) => s.id === id), screenPayload)
}

function draftName(id) {
  return drafts[id]?.name || props.screens.find((s) => s.id === id)?.name || `Screen #${id}`
}

function summary(screen) {
  const d = drafts[screen.id] || screen
  if (d.kind === 'grid') return `Grid · ${d.layout || '—'}`
  const n = (d.items || []).length
  return `Transition · ${n} ${n === 1 ? 'item' : 'items'}`
}

function rectsFor(layoutId) {
  const l = props.layouts.find((x) => x.id === layoutId)
  return l?.rects || []
}

function onLayoutChange(screenId, layoutId) {
  const d = drafts[screenId]
  if (!d || d.kind !== 'grid') return
  const l = props.layouts.find((x) => x.id === layoutId)
  const n = l?.rects?.length
  if (!n) return
  d.tiles = (d.tiles || []).filter((t) => t.index >= 0 && t.index < n)
}

/** Rebuilds a draft from the current server state, discarding local edits. */
function syncDraft(id) {
  const screen = props.screens.find((s) => s.id === id)
  if (screen) drafts[id] = cloneScreen(screen)
}

/**
 * Expand a newly created screen, scroll it into view, and flash a focus ring
 * so the user lands on the editor they just asked for.
 */
async function focusCreated(id) {
  if (id == null) return
  if (!opened.value.includes(id)) {
    opened.value = [...opened.value, id]
  }
  highlighted.value = id
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    if (highlighted.value === id) highlighted.value = null
    highlightTimer = null
  }, HIGHLIGHT_MS)
  // Double nextTick: first for the open state, second for the panel DOM after expand.
  await nextTick()
  await nextTick()
  document
    .querySelector(`[data-panel-key="${id}"]`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
}

defineExpose({ syncDraft, focusCreated })
</script>
