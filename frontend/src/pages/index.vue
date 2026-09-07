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
        type="info"
        density="compact"
        class="mb-4"
      >
        Display engine is not running. Preview shows a placeholder until the engine is started.
      </v-alert>

      <div class="preview-stage mb-4">
        <div class="preview-frame">
          <div class="preview-surface d-flex align-center justify-center">
            <div class="text-center">
              <v-icon size="48" class="mb-2">mdi-monitor-off</v-icon>
              <div class="text-body-2">Engine not running</div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="auth.isAdmin" class="d-flex align-center flex-wrap mb-4">
        <v-btn variant="outlined" size="small" class="mr-2 mb-2" disabled>Previous</v-btn>
        <v-btn variant="outlined" size="small" class="mr-2 mb-2" disabled>Next</v-btn>
        <v-btn color="primary" variant="elevated" size="small" class="mb-2" disabled>Pause</v-btn>
      </div>

      <div class="text-caption text-medium-emphasis">
        Active screen: none · Tour paused · {{ events.connected ? 'Event stream connected' : 'Event stream idle' }}
      </div>
    </StandardCard>
  </v-container>
</template>

<script setup>
import StandardCard from '@/components/common/StandardCard.vue'
import { useDisplayState } from '@/composables/useDisplayState'
import { useEventStream } from '@/composables/useEventStream'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { display } = useDisplayState()
const events = useEventStream()
</script>

<style scoped>
.preview-stage {
  background: #000;
  width: 100%;
  max-width: 960px;
}

.preview-frame {
  width: 100%;
  aspect-ratio: 16 / 9;
}

.preview-surface {
  width: 100%;
  height: 100%;
  color: rgba(255, 255, 255, 0.7);
}
</style>
