<template>
  <v-container class="page-content">
    <StandardCard title="Preview" title-class="text-h5">
      <v-alert
        v-if="display.error"
        type="error"
        density="compact"
        class="mb-4"
      >
        {{ display.error }}
      </v-alert>
      <v-alert
        v-if="!display.display_running"
        type="info"
        density="compact"
        class="mb-4"
      >
        Display engine is not running. Preview shows layout geometry from the scheduler until the engine is started.
      </v-alert>

      <div class="preview-stage mb-4">
        <div class="preview-frame" :style="{ aspectRatio: String(aspect) }">
          <LayoutDiagram
            v-if="previewRects.length"
            :rects="previewRects"
            :aspect="aspect"
            :empty="emptyMap"
          >
            <template #cell="{ index }">
              <div class="text-center text-caption">
                <img
                  v-if="tileThumb(index)"
                  :src="tileThumb(index)"
                  alt=""
                  class="preview-thumb mb-1"
                  @error="hideThumb(index)"
                >
                <div>{{ tileCaption(index) }}</div>
              </div>
            </template>
          </LayoutDiagram>
          <div v-else class="preview-surface d-flex align-center justify-center">
            <div class="text-center">
              <v-icon size="48" class="mb-2">mdi-monitor-off</v-icon>
              <div class="text-body-2">No active screen</div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="auth.isAdmin" class="d-flex align-center flex-wrap mb-4" style="gap: 8px">
        <v-btn variant="outlined" size="small" :loading="busy" @click="command('previous')">Previous</v-btn>
        <v-btn variant="outlined" size="small" :loading="busy" @click="command('next')">Next</v-btn>
        <v-btn
          color="primary"
          variant="elevated"
          size="small"
          :loading="busy"
          @click="command(display.paused ? 'resume' : 'pause')"
        >
          {{ display.paused ? 'Resume' : 'Pause' }}
        </v-btn>
        <v-select
          v-if="screens.length"
          :model-value="display.active_screen"
          :items="screenItems"
          label="Go to screen"
          variant="outlined"
          density="compact"
          hide-details="auto"
          autocomplete="off"
          style="max-width: 240px;"
          @update:model-value="gotoScreen"
        />
      </div>

      <div class="text-caption text-medium-emphasis mb-4">
        Active: {{ display.active_screen_name || display.active_screen || 'none' }}
        · Next: {{ nextScreenLabel }}
        · {{ display.paused ? 'Tour paused' : 'Tour running' }}
        · Dwell remaining: {{ Math.round((display.dwell_remaining_ms || 0) / 1000) }}s
        · {{ events.connected ? 'Event stream connected' : 'Event stream idle' }}
      </div>

      <v-data-table
        v-if="(display.tiles || []).length"
        :headers="tileHeaders"
        :items="display.tiles"
        density="compact"
        :items-per-page="50"
      >
        <template #bottom />
      </v-data-table>
    </StandardCard>
  </v-container>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import StandardCard from '@/components/common/StandardCard.vue'
import LayoutDiagram from '@/components/display/LayoutDiagram.vue'
import { useDisplayState } from '@/composables/useDisplayState'
import { useLayouts } from '@/composables/useLayouts'
import { useAuthStore } from '@/stores/auth'
import { api, getToken } from '@/utils/api'
import { nameByID, selectItems } from '@/utils/formatters'
import { getIngressBase } from '@/utils/ingress'
import { FULL_BLEED_ID } from '@/utils/layouts'

const auth = useAuthStore()
const { display, seed, events } = useDisplayState()
const layouts = useLayouts()
const screens = ref([])
const config = ref({ output_width: 1920, output_height: 1080 })
const busy = ref(false)
const hiddenThumbs = reactive({})

const aspect = computed(() => (config.value.output_width || 1920) / (config.value.output_height || 1080))
const screenItems = computed(() => selectItems(screens.value))
const screenName = computed(() => nameByID(screens.value))
const nextScreenLabel = computed(() => {
  const id = display.next_screen
  if (id == null) return 'none'
  return screenName.value[id] || String(id)
})

const activeScreen = computed(() => screens.value.find((s) => s.id === display.active_screen))
const previewRects = computed(() => {
  const sc = activeScreen.value
  if (!sc) return []
  if (sc.kind === 'transition') {
    const full = layouts.state.layouts.find((x) => x.id === FULL_BLEED_ID)
    return full?.rects || []
  }
  const l = layouts.state.layouts.find((x) => x.id === sc.layout)
  return l?.rects || []
})

const emptyMap = computed(() => {
  const m = {}
  ;(display.tiles || []).forEach((t) => {
    m[t.index] = !t.source_id
  })
  return m
})

const tileHeaders = [
  { title: 'Tile', key: 'index' },
  { title: 'Source', key: 'source_name' },
  { title: 'Fit', key: 'fit' },
  { title: 'Decoder', key: 'decoder' },
]

function tileCaption(index) {
  const t = (display.tiles || []).find((x) => x.index === index)
  if (!t) return ''
  return t.source_name || (t.source_id ? `#${t.source_id}` : 'Empty')
}

function tileThumb(index) {
  if (hiddenThumbs[index]) return ''
  const t = (display.tiles || []).find((x) => x.index === index)
  if (!t?.source_id) return ''
  const token = getToken()
  const q = token ? `?access_token=${encodeURIComponent(token)}` : ''
  return `${getIngressBase()}api/v1/sources/${t.source_id}/thumbnail${q}`
}

function hideThumb(index) {
  hiddenThumbs[index] = true
}

async function command(name) {
  busy.value = true
  display.error = null
  try {
    await api.post(`api/v1/display/${name}`)
    await seed()
  } catch (err) {
    console.log('[Preview] API error:', err)
    display.error = err.message
  } finally {
    busy.value = false
  }
}

async function gotoScreen(id) {
  if (!id) return
  busy.value = true
  display.error = null
  try {
    await api.post(`api/v1/display/goto/${id}`)
    await seed()
  } catch (err) {
    console.log('[Preview] API error:', err)
    display.error = err.message
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    const [s, cfg] = await Promise.all([
      api.get('api/v1/screens'),
      api.get('api/v1/config'),
    ])
    screens.value = s.screens || []
    config.value = cfg
  } catch (err) {
    console.log('[Preview] API error:', err)
  }
})
</script>

<style scoped>
.preview-stage {
  background: #000;
  width: 100%;
  max-width: 960px;
}

.preview-frame {
  width: 100%;
}

.preview-surface {
  width: 100%;
  height: 100%;
  min-height: 180px;
  color: rgba(255, 255, 255, 0.7);
}

.preview-thumb {
  max-width: 72px;
  max-height: 48px;
  object-fit: cover;
}
</style>
