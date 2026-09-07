<template>
  <v-container class="page-content">
    <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
    <v-alert v-if="success" type="success" density="compact" class="mb-4">{{ success }}</v-alert>

    <StandardCard class="mb-4">
      <template #header>
        <span class="text-h5 header-title">Screens</span>
        <v-spacer />
        <v-btn
          v-if="auth.isAdmin"
          color="primary"
          variant="elevated"
          size="small"
          @click="openAddScreen"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add Screen
        </v-btn>
      </template>
      <ScreensEditor
        ref="screensEditor"
        :screens="screens"
        :sources="sources"
        :layouts="layouts.state.layouts"
        :aspect="aspect"
        :catalogue="transitions"
        :presets="easings"
        :editable="auth.isAdmin"
        :saving-id="savingScreenId"
        @save="saveScreen"
        @delete="askDeleteScreen"
      />
    </StandardCard>

    <StandardCard>
      <template #header>
        <span class="text-h5 header-title">Tour</span>
        <v-spacer />
        <v-btn
          v-if="auth.isAdmin && screens.length > 0"
          color="primary"
          variant="elevated"
          size="small"
          @click="tourEditor?.add()"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add Entry
        </v-btn>
      </template>
      <TourEditor
        ref="tourEditor"
        :tour="tour"
        :screens="screens"
        :catalogue="transitions"
        :presets="easings"
        :editable="auth.isAdmin"
        @update:tour="tour = $event"
      />
      <template v-if="auth.isAdmin && screens.length > 0" #actions>
        <v-btn
          color="primary"
          variant="elevated"
          :loading="savingTour"
          :disabled="!tourDirty"
          @click="saveTour"
        >
          Save
        </v-btn>
      </template>
    </StandardCard>

    <StandardDialog
      v-model="showAddScreen"
      title="Add Screen"
      max-width="480"
      :fullscreen="mobile"
      @close="showAddScreen = false"
    >
      <v-alert v-if="addError" type="error" density="compact" class="mb-4">{{ addError }}</v-alert>
      <v-text-field
        v-model="addForm.name"
        label="Name"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
      />
      <v-select
        v-model="addForm.kind"
        :items="kindItems"
        label="Kind"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
      />
      <template #actions>
        <v-spacer />
        <v-btn variant="text" class="mr-2" @click="showAddScreen = false">Cancel</v-btn>
        <v-btn color="primary" variant="elevated" :loading="creating" @click="createScreen">
          Create
        </v-btn>
      </template>
    </StandardDialog>

    <ConfirmDeleteDialog
      v-model="showDelete"
      title="Delete screen?"
      :name="screenToDelete?.name"
      :error="deleteError"
      :loading="deleting"
      @close="screenToDelete = null"
      @confirm="confirmDeleteScreen"
    />
  </v-container>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useDisplay } from 'vuetify'
import ConfirmDeleteDialog from '@/components/common/ConfirmDeleteDialog.vue'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import ScreensEditor from '@/components/display/ScreensEditor.vue'
import TourEditor from '@/components/display/TourEditor.vue'
import { api } from '@/utils/api'
import { FULL_BLEED_ID } from '@/utils/layouts'
import { screenPayload, tourPayload } from '@/utils/payloads'
import { useFeedback } from '@/composables/useFeedback'
import { useLayouts } from '@/composables/useLayouts'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { mobile } = useDisplay()
const layouts = useLayouts()
const { error, success, showError, showSuccess, clear: clearFeedback } = useFeedback()

const screens = ref([])
const sources = ref([])
const tour = ref({ enabled: false, loop: false, entries: [] })
const tourSnapshot = ref('')
const config = ref({ output_width: 1920, output_height: 1080 })
const transitions = ref([])
const easings = ref([])
const savingScreenId = ref(null)
const savingTour = ref(false)
const showAddScreen = ref(false)
const addError = ref('')
const creating = ref(false)
const addForm = reactive({ name: '', kind: 'grid' })
const showDelete = ref(false)
const screenToDelete = ref(null)
const deleteError = ref('')
const deleting = ref(false)
const tourEditor = ref(null)
const screensEditor = ref(null)

const kindItems = [
  { title: 'Grid', value: 'grid' },
  { title: 'Transition', value: 'transition' },
]

const aspect = computed(() => {
  const w = config.value.output_width || 1920
  const h = config.value.output_height || 1080
  return w / h
})

const tourDirty = computed(
  () => JSON.stringify(tourPayload(tour.value)) !== tourSnapshot.value,
)

async function load() {
  try {
    const [s, t, src, cfg, cat] = await Promise.all([
      api.get('api/v1/screens'),
      api.get('api/v1/tour'),
      api.get('api/v1/sources'),
      api.get('api/v1/config'),
      api.get('api/v1/transitions'),
    ])
    screens.value = s.screens || []
    tour.value = t
    if (!tour.value.entries) tour.value.entries = []
    tourSnapshot.value = JSON.stringify(tourPayload(tour.value))
    sources.value = src.sources || []
    config.value = cfg
    transitions.value = cat.transitions || []
    easings.value = cat.easing_presets || []
  } catch (err) {
    console.log('[Display] API error:', err)
    showError(err.message || 'Failed to load display strategy')
  }
}

function openAddScreen() {
  addForm.name = ''
  addForm.kind = 'grid'
  addError.value = ''
  showAddScreen.value = true
}

async function createScreen() {
  addError.value = ''
  if (!addForm.name.trim()) {
    addError.value = 'Name is required'
    return
  }
  creating.value = true
  try {
    const body = { name: addForm.name.trim(), kind: addForm.kind }
    if (addForm.kind === 'grid') body.layout = FULL_BLEED_ID
    else {
      body.items = sources.value[0] ? [{ source_id: sources.value[0].id, dwell_ms: 15000, fit: 'contain' }] : []
      body.transition = { type: 'cut', duration_ms: 0 }
      body.loop = true
    }
    const created = await api.post('api/v1/screens', body)
    showAddScreen.value = false
    await load()
    screensEditor.value?.focusCreated(created?.id)
    showSuccess('Screen created')
  } catch (err) {
    console.log('[Display] API error:', err)
    addError.value = err.message || 'Failed to create screen'
  } finally {
    creating.value = false
  }
}

async function saveScreen(draft) {
  if (!draft?.id) return
  savingScreenId.value = draft.id
  clearFeedback()
  try {
    await api.patch(`api/v1/screens/${draft.id}`, screenPayload(draft))
    await load()
    // The panel is still open, so the list watcher deliberately skips it.
    screensEditor.value?.syncDraft(draft.id)
    showSuccess('Screen saved')
  } catch (err) {
    console.log('[Display] API error:', err)
    showError(err.message || 'Failed to save screen')
  } finally {
    savingScreenId.value = null
  }
}

function askDeleteScreen(item) {
  screenToDelete.value = item
  deleteError.value = ''
  showDelete.value = true
}

async function confirmDeleteScreen() {
  if (!screenToDelete.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.delete(`api/v1/screens/${screenToDelete.value.id}`)
    showDelete.value = false
    await load()
    showSuccess('Screen deleted')
  } catch (err) {
    console.log('[Display] API error:', err)
    if (err.status === 409) {
      const entries = err.body?.details?.tour_entries || []
      deleteError.value = entries.length
        ? `This screen is in the tour (${entries.length} ${entries.length === 1 ? 'entry' : 'entries'}). Remove it from the tour first.`
        : (err.message || 'Screen is in use')
    } else {
      deleteError.value = err.message || 'Failed to delete screen'
    }
  } finally {
    deleting.value = false
  }
}

async function saveTour() {
  if (!tourDirty.value) return
  savingTour.value = true
  clearFeedback()
  try {
    await api.put('api/v1/tour', tourPayload(tour.value))
    await load()
    showSuccess('Tour saved')
  } catch (err) {
    console.log('[Display] API error:', err)
    showError(err.message || 'Failed to save tour')
  } finally {
    savingTour.value = false
  }
}

onMounted(load)
</script>
