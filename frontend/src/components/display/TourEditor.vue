<template>
  <div>
    <!-- Prerequisite: screens must exist before a tour can be built (VMS layout-tour pattern). -->
    <EmptyState
      v-if="!screens.length"
      icon="mdi-view-grid-outline"
      title="Create a screen first"
      copy="A tour sequences screens on the wall. Define at least one screen above, then add it here."
    />

    <template v-else>
      <div class="d-flex align-center flex-wrap mb-4 switch-cluster">
        <v-switch
          :model-value="tour.enabled"
          label="Enabled"
          color="primary"
          inset
          density="compact"
          hide-details="auto"
          :disabled="!editable || !entries.length"
          @update:model-value="$emit('update:tour', { ...tour, enabled: $event })"
        />
        <v-switch
          :model-value="tour.loop"
          label="Loop"
          color="primary"
          inset
          density="compact"
          hide-details="auto"
          :disabled="!editable || !entries.length"
          @update:model-value="$emit('update:tour', { ...tour, loop: $event })"
        />
      </div>
      <p v-if="entries.length === 1" class="text-body-2 text-medium-emphasis mb-4">
        One entry is a static wall. Add more entries to step between screens.
      </p>

      <EmptyState
        v-if="!entries.length"
        icon="mdi-movie-open-play"
        title="No tour entries yet"
        copy="Add screens in order. Each entry’s dwell and incoming transition control how long it stays and how it arrives."
      />

      <v-expansion-panels
        v-else
        v-model="opened"
        multiple
        flat
        variant="accordion"
      >
        <v-expansion-panel
          v-for="(entry, i) in entries"
          :key="i"
          :value="i"
          :data-panel-key="`tour-${i}`"
          :class="{ 'panel-flash': highlighted === i }"
        >
          <v-expansion-panel-title>
            <div class="d-flex align-center flex-grow-1 flex-wrap pe-2" style="gap: 8px; min-width: 0;">
              <span class="text-subtitle-1 text-truncate">
                {{ i + 1 }}. {{ screenName(entry.screen_id) }}
              </span>
              <span class="text-body-2 text-medium-emphasis text-truncate">
                {{ formatDwellSeconds(entry.dwell_ms) }}s · {{ formatTransitionSummary(entry.transition) }}
              </span>
              <v-spacer />
              <div
                v-if="editable"
                class="d-flex align-center"
                style="gap: 4px"
                @click.stop
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
                  :disabled="i === entries.length - 1"
                  aria-label="Move down"
                  @click="move(i, 1)"
                >
                  <v-icon size="small">mdi-arrow-down</v-icon>
                </v-btn>
                <v-btn
                  icon
                  variant="text"
                  size="small"
                  aria-label="Remove entry"
                  @click="remove(i)"
                >
                  <v-icon size="small">mdi-close</v-icon>
                </v-btn>
              </div>
            </div>
          </v-expansion-panel-title>
          <v-expansion-panel-text>
            <div class="field-row mb-4">
              <v-select
                :model-value="entry.screen_id"
                :items="screenItems"
                label="Screen"
                variant="outlined"
                density="compact"
                hide-details="auto"
                autocomplete="off"
                :disabled="!editable"
                @update:model-value="patch(i, { screen_id: $event })"
              />
              <v-text-field
                :model-value="formatDwellSeconds(entry.dwell_ms)"
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
            </div>
            <div class="text-subtitle-2 mb-4">Transition when entering this entry</div>
            <TransitionEditor
              :model-value="entry.transition || DEFAULT_CUT"
              :catalogue="catalogue"
              :presets="presets"
              :editable="editable"
              @update:model-value="patch(i, { transition: $event })"
            />
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import EmptyState from '@/components/common/EmptyState.vue'
import TransitionEditor from '@/components/display/TransitionEditor.vue'
import {
  dwellMsFromSeconds,
  formatDwellSeconds,
  formatTransitionSummary,
  nameByID,
  selectItems,
} from '@/utils/formatters'
import { DEFAULT_CUT } from '@/utils/payloads'

const HIGHLIGHT_MS = 2000

const props = defineProps({
  tour: {
    type: Object,
    required: true,
  },
  screens: {
    type: Array,
    default: () => [],
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

const emit = defineEmits(['update:tour'])
/** Open panel indices; empty = all collapsed. Multiple may be open. */
const opened = ref([])
const highlighted = ref(null)
let highlightTimer = null

const entries = computed(() => props.tour.entries || [])
const screenItems = computed(() => selectItems(props.screens))
const screenNames = computed(() => nameByID(props.screens))

watch(entries, (list) => {
  opened.value = opened.value.filter((i) => i >= 0 && i < list.length)
})

onUnmounted(() => {
  if (highlightTimer) clearTimeout(highlightTimer)
})

function screenName(id) {
  return screenNames.value[id] || `Screen #${id}`
}

function withEntries(next) {
  emit('update:tour', { ...props.tour, entries: next })
}

function patch(i, fields) {
  const next = entries.value.map((e, idx) => (idx === i ? { ...e, ...fields } : e))
  withEntries(next)
}

function remapOpened(mapIndex) {
  opened.value = [...new Set(opened.value.map(mapIndex).filter((i) => i != null && i >= 0))]
}

function move(i, delta) {
  const next = [...entries.value]
  const j = i + delta
  const tmp = next[i]
  next[i] = next[j]
  next[j] = tmp
  withEntries(next.map((e, idx) => ({ ...e, position: idx })))
  remapOpened((idx) => {
    if (idx === i) return j
    if (idx === j) return i
    return idx
  })
}

function remove(i) {
  const next = entries.value.filter((_, idx) => idx !== i)
  const updated = next.map((e, idx) => ({ ...e, position: idx }))
  if (!updated.length && props.tour.enabled) {
    emit('update:tour', { ...props.tour, enabled: false, entries: updated })
  } else {
    withEntries(updated)
  }
  remapOpened((idx) => {
    if (idx === i) return null
    if (idx > i) return idx - 1
    return idx
  })
}

async function flash(index) {
  highlighted.value = index
  if (highlightTimer) clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    if (highlighted.value === index) highlighted.value = null
    highlightTimer = null
  }, HIGHLIGHT_MS)
  // Double nextTick: first for the list update, second for the expanded panel DOM.
  await nextTick()
  await nextTick()
  document
    .querySelector(`[data-panel-key="tour-${index}"]`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
}

async function add() {
  if (!props.screens.length) return
  const next = [...entries.value]
  next.push({
    position: next.length,
    screen_id: props.screens[0]?.id,
    dwell_ms: 30000,
    transition: { ...DEFAULT_CUT },
  })
  const index = next.length - 1
  withEntries(next)
  opened.value = [...new Set([...opened.value, index])]
  await flash(index)
}

defineExpose({ add })
</script>
