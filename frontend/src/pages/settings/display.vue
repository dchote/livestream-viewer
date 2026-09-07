<template>
  <v-container class="page-content">
    <v-alert
      v-if="error"
      type="error"
      density="compact"
      class="mb-4"
    >
      {{ error }}
    </v-alert>

    <StandardCard class="mb-4">
      <template #header>
        <span class="text-h5 header-title">Screens</span>
        <v-spacer />
        <v-btn
          v-if="auth.isAdmin"
          color="primary"
          variant="elevated"
          size="small"
          @click="showAddScreen = true"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add Screen
        </v-btn>
      </template>
      <v-data-table
        v-if="screens.length > 0"
        :headers="screenHeaders"
        :items="screens"
        density="comfortable"
        :items-per-page="50"
      >
        <template #bottom />
      </v-data-table>
      <div v-else class="empty-state">
        <v-icon class="empty-state__icon">mdi-view-grid-plus</v-icon>
        <div class="text-h6 empty-state__title">No screens defined yet</div>
        <p class="empty-state__copy">Create a grid or transition screen to compose the wall.</p>
      </div>
    </StandardCard>

    <StandardCard class="mb-4" title="Screen editor" title-class="text-h5">
      <p class="text-body-2 text-medium-emphasis mb-0">
        Select a screen to edit tiles (grid) or the playlist (transition). Layout geometry comes from
        <code>GET /api/v1/layouts</code>
        ({{ layouts.state.layouts.length }} layouts loaded).
      </p>
    </StandardCard>

    <StandardCard>
      <template #header>
        <span class="text-h5 header-title">Tour</span>
        <v-spacer />
        <v-btn v-if="auth.isAdmin" color="primary" variant="elevated" size="small" disabled>
          <v-icon start size="small">mdi-plus</v-icon>
          Add Entry
        </v-btn>
      </template>
      <v-data-table
        v-if="(tour.entries || []).length > 0"
        :headers="tourHeaders"
        :items="tour.entries || []"
        density="comfortable"
        :items-per-page="50"
      >
        <template #bottom />
      </v-data-table>
      <div v-else class="empty-state">
        <v-icon class="empty-state__icon">mdi-rotate-3d-variant</v-icon>
        <div class="text-h6 empty-state__title">No tour entries</div>
        <p class="empty-state__copy">
          Tour is {{ tour.enabled ? 'enabled' : 'disabled' }}. A single pinned screen is the expected starting configuration.
        </p>
      </div>
    </StandardCard>

    <StandardDialog
      v-model="showAddScreen"
      title="Add Screen"
      max-width="480"
      :fullscreen="mobile"
      @close="showAddScreen = false"
    >
      <v-alert type="info" density="compact" class="mb-4">
        Screen create is not implemented in this scaffold.
      </v-alert>
      <v-text-field
        label="Name"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
        style="max-width: 320px;"
      />
      <v-select
        :items="['grid', 'transition']"
        label="Kind"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        class="mb-4"
        style="max-width: 320px;"
      />
      <template #actions>
        <v-spacer />
        <v-btn variant="text" class="mr-2" @click="showAddScreen = false">Cancel</v-btn>
        <v-btn color="primary" variant="elevated" disabled>Save</v-btn>
      </template>
    </StandardDialog>
  </v-container>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useDisplay } from 'vuetify'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { api } from '@/utils/api'
import { useLayouts } from '@/composables/useLayouts'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { mobile } = useDisplay()

const screens = ref([])
const tour = ref({ enabled: false, loop: false, entries: [] })
const error = ref('')
const showAddScreen = ref(false)
const layouts = useLayouts()

const screenHeaders = [
  { title: 'Name', key: 'name' },
  { title: 'Kind', key: 'kind' },
  { title: 'Layout', key: 'layout' },
]
const tourHeaders = [
  { title: 'Position', key: 'position' },
  { title: 'Screen', key: 'screen_id' },
  { title: 'Dwell time (ms)', key: 'dwell_ms' },
]

onMounted(async () => {
  try {
    const [s, t] = await Promise.all([
      api.get('api/v1/screens'),
      api.get('api/v1/tour'),
    ])
    screens.value = s.screens || []
    tour.value = t
  } catch (err) {
    console.log('[Display] API error:', err)
    error.value = err.message || 'Failed to load display strategy'
  }
})
</script>
