<template>
  <v-container class="page-content">
    <StandardCard card-class="mb-4">
      <template #header>
        <span class="text-h5 header-title">Stream Sources</span>
        <v-spacer />
        <v-btn
          v-if="auth.isAdmin"
          color="primary"
          variant="elevated"
          size="small"
          @click="openAdd"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add Source
        </v-btn>
      </template>

      <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
      <v-alert v-if="success" type="success" density="compact" class="mb-4">{{ success }}</v-alert>
      <v-alert v-if="capsHint" type="info" density="compact" class="mb-4">{{ capsHint }}</v-alert>

      <SourceList
        v-if="sources.length > 0"
        :sources="sources"
        :is-admin="auth.isAdmin"
        :probing-id="probingId"
        @probe="probeSource"
        @edit="openEdit"
        @remove="askDelete"
      />
      <EmptyState
        v-else
        icon="mdi-video-input-antenna"
        title="No sources yet"
        copy="Add a stream source to get started."
      />
    </StandardCard>

    <YouTubeSessionCard
      v-if="hasYouTubeSource"
      :is-admin="auth.isAdmin"
      :yt-dlp-version="systemInfo?.yt_dlp_version || ''"
      :last-error-code="youtubeIssue.code"
      :last-error="youtubeIssue.message"
    />

    <StandardDialog
      v-model="showForm"
      :title="editing ? 'Edit Source' : 'Add Source'"
      max-width="560"
      :fullscreen="mobile"
      @close="closeForm"
    >
      <SourceForm ref="formRef" :source="editing" :system-info="systemInfo" :error="formError" />
      <template #actions>
        <v-spacer />
        <v-btn variant="text" class="mr-2" @click="showForm = false">Cancel</v-btn>
        <v-btn color="primary" variant="elevated" :loading="saving" @click="saveSource">
          {{ editing ? 'Save' : 'Create' }}
        </v-btn>
      </template>
    </StandardDialog>

    <ConfirmDeleteDialog
      v-model="showDelete"
      title="Delete source?"
      :name="toDelete?.name"
      :error="deleteError"
      :loading="deleting"
      @close="toDelete = null"
      @confirm="confirmDelete"
    >
      <p v-if="deleteRefs.length" class="text-body-2 mb-0">
        In use by: {{ deleteRefs.map((s) => s.name).join(', ') }}
      </p>
    </ConfirmDeleteDialog>
  </v-container>
</template>

<script setup>
import { onMounted, ref, computed, watch } from 'vue'
import { useDisplay } from 'vuetify'
import ConfirmDeleteDialog from '@/components/common/ConfirmDeleteDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import SourceForm from '@/components/sources/SourceForm.vue'
import SourceList from '@/components/sources/SourceList.vue'
import YouTubeSessionCard from '@/components/sources/YouTubeSessionCard.vue'
import { api } from '@/utils/api'
import { useFeedback } from '@/composables/useFeedback'
import { useDisplayState } from '@/composables/useDisplayState'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { mobile } = useDisplay()
const { error, success, showError, showSuccess, clear: clearFeedback } = useFeedback()
const { display } = useDisplayState()

const sources = ref([])
const showForm = ref(false)
const formRef = ref(null)
const formError = ref('')
const saving = ref(false)
const editing = ref(null)
const systemInfo = ref(null)
const probingId = ref(null)
const showDelete = ref(false)
const toDelete = ref(null)
const deleteRefs = ref([])
const deleteError = ref('')
const deleting = ref(false)

const capsHint = computed(() => {
  const c = systemInfo.value?.capabilities
  if (!c) return ''
  const parts = []
  if (c.videotoolbox) parts.push('VideoToolbox')
  if (c.vaapi) parts.push('VA-API')
  if (c.h264_hw) parts.push('H.264 hardware')
  else parts.push('H.264 software')
  if (c.hevc_hw) parts.push('HEVC hardware')
  else parts.push('HEVC software')
  return `This host: ${parts.join(', ')}. Status chips follow live decode health when the engine is running.`
})

const hasYouTubeSource = computed(() => sources.value.some((s) => s.kind === 'youtube'))

const youtubeIssue = computed(() => {
  const list = display.decoders || []
  const hit = list.find((d) => d.error_code === 'youtube_bot_check')
    || list.find((d) => d.error_code === 'youtube_auth')
  if (!hit) return { code: '', message: '' }
  return { code: hit.error_code, message: hit.error || '' }
})

function mergeDecoders(list) {
  const byID = {}
  ;(display.decoders || []).forEach((d) => {
    byID[d.source_id] = d
  })
  return (list || []).map((s) => {
    const d = byID[s.id]
    if (!d) return s
    return {
      ...s,
      decoder: d.decoder || s.decoder,
      error_code: d.error_code || s.error_code,
      error: d.error || s.error,
    }
  })
}

async function load() {
  try {
    const data = await api.get('api/v1/sources')
    sources.value = mergeDecoders(data.sources || [])
  } catch (err) {
    console.log('[Sources] API error:', err)
    showError(err.message || 'Failed to load sources')
  }
}

watch(
  () => display.decoders,
  () => {
    sources.value = mergeDecoders(sources.value)
  },
  { deep: true },
)

function openAdd() {
  editing.value = null
  formError.value = ''
  showForm.value = true
}

function openEdit(item) {
  editing.value = item
  formError.value = ''
  showForm.value = true
}

function closeForm() {
  editing.value = null
  formError.value = ''
}

async function saveSource() {
  formError.value = ''
  const body = formRef.value?.payload()
  if (!body?.name) {
    formError.value = 'Name is required'
    return
  }
  saving.value = true
  const isEdit = !!editing.value
  try {
    if (isEdit) {
      await api.patch(`api/v1/sources/${editing.value.id}`, body)
    } else {
      await api.post('api/v1/sources', body)
    }
    showForm.value = false
    await load()
    showSuccess(isEdit ? 'Source saved' : 'Source created')
  } catch (err) {
    console.log('[Sources] API error:', err)
    formError.value = err.message || 'Failed to save source'
  } finally {
    saving.value = false
  }
}

async function probeSource(item) {
  probingId.value = item.id
  clearFeedback()
  try {
    await api.post(`api/v1/sources/${item.id}/probe`)
    await load()
  } catch (err) {
    console.log('[Sources] API error:', err)
    showError(err.message || 'Probe failed')
  } finally {
    probingId.value = null
  }
}

function askDelete(item) {
  toDelete.value = item
  deleteRefs.value = []
  deleteError.value = ''
  showDelete.value = true
}

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await api.delete(`api/v1/sources/${toDelete.value.id}`)
    showDelete.value = false
    toDelete.value = null
    await load()
    showSuccess('Source deleted')
  } catch (err) {
    console.log('[Sources] API error:', err)
    if (err.status === 409) {
      deleteRefs.value = err.body?.details?.screens || []
      deleteError.value = err.message || 'Source is in use'
    } else {
      deleteError.value = err.message || 'Failed to delete'
    }
  } finally {
    deleting.value = false
  }
}

onMounted(async () => {
  await load()
  try {
    systemInfo.value = await api.get('api/v1/system/info')
  } catch (err) {
    console.log('[Sources] system info:', err)
  }
})
</script>
