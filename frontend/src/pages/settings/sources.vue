<template>
  <v-container class="page-content">
    <StandardCard>
      <template #header>
        <span class="text-h5 header-title">Stream Sources</span>
        <v-spacer />
        <v-btn
          v-if="auth.isAdmin"
          color="primary"
          variant="elevated"
          size="small"
          @click="showAdd = true"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add Source
        </v-btn>
      </template>

      <v-alert
        v-if="error"
        type="error"
        density="compact"
        class="mb-4"
      >
        {{ error }}
      </v-alert>

      <v-data-table
        v-if="sources.length > 0"
        :headers="headers"
        :items="sources"
        density="comfortable"
        :items-per-page="50"
      >
        <template #bottom />
      </v-data-table>
      <div v-else class="empty-state">
        <v-icon class="empty-state__icon">mdi-video-input-antenna</v-icon>
        <div class="text-h6 empty-state__title">No sources yet</div>
        <p class="empty-state__copy">Add a stream source to get started.</p>
      </div>
    </StandardCard>

    <StandardDialog
      v-model="showAdd"
      title="Add Source"
      max-width="480"
      :fullscreen="mobile"
      @close="resetAdd"
    >
      <v-alert type="info" density="compact" class="mb-4">
        Source create is not implemented in this scaffold. The form shows the intended layout.
      </v-alert>
      <v-text-field
        v-model="form.name"
        label="Name"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
        style="max-width: 320px;"
      />
      <v-select
        v-model="form.kind"
        :items="kinds"
        label="Kind"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
        style="max-width: 320px;"
      />
      <v-text-field
        v-model="form.url"
        label="URL"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
      />
      <template #actions>
        <v-spacer />
        <v-btn variant="text" class="mr-2" @click="showAdd = false">Cancel</v-btn>
        <v-btn color="primary" variant="elevated" disabled>Save</v-btn>
      </template>
    </StandardDialog>
  </v-container>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useDisplay } from 'vuetify'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { api } from '@/utils/api'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { mobile } = useDisplay()

const sources = ref([])
const error = ref('')
const showAdd = ref(false)
const form = reactive({ name: '', kind: 'rtsp', url: '' })
const kinds = ['youtube', 'rtsp', 'hls', 'dash', 'http', 'srt', 'rtmp', 'file']
const headers = [
  { title: 'Name', key: 'name' },
  { title: 'Kind', key: 'kind' },
  { title: 'Enabled', key: 'enabled' },
]

function resetAdd() {
  form.name = ''
  form.kind = 'rtsp'
  form.url = ''
}

onMounted(async () => {
  try {
    const data = await api.get('api/v1/sources')
    sources.value = data.sources || []
  } catch (err) {
    console.log('[Sources] API error:', err)
    error.value = err.message || 'Failed to load sources'
  }
})
</script>
