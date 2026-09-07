<template>
  <v-sheet
    class="upload-dropzone pa-4"
    :class="{ 'upload-dropzone--active': dragover }"
    @dragover.prevent="dragover = true"
    @dragleave.prevent="dragover = false"
    @drop.prevent="onDrop"
  >
    <div class="text-body-2 mb-2">Drop a video file here or choose one.</div>
    <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
    <v-progress-linear
      v-if="progress != null"
      class="mb-4"
      :model-value="progress * 100"
      color="primary"
    />
    <div class="d-flex align-center flex-wrap" style="gap: 8px">
      <v-btn variant="outlined" size="small" @click="input?.click()">
        Choose file
      </v-btn>
      <span v-if="fileName" class="text-caption">{{ fileName }}</span>
    </div>
    <input
      ref="input"
      type="file"
      accept="video/*"
      class="d-none"
      @change="onPick"
    >
  </v-sheet>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '@/utils/api'

const emit = defineEmits(['uploaded'])

const dragover = ref(false)
const fileName = ref('')
const progress = ref(null)
const error = ref('')
const input = ref(null)

async function send(file) {
  error.value = ''
  fileName.value = file.name
  progress.value = 0
  try {
    const data = await api.upload('api/v1/uploads', file, (p) => { progress.value = p })
    emit('uploaded', data)
  } catch (err) {
    console.log('[UploadDropzone] API error:', err)
    error.value = err.message || 'Upload failed'
  } finally {
    progress.value = null
  }
}

function onPick(ev) {
  const file = ev.target.files?.[0]
  if (file) send(file)
}

function onDrop(ev) {
  dragover.value = false
  const file = ev.dataTransfer.files?.[0]
  if (file) send(file)
}
</script>

<style scoped>
.upload-dropzone {
  border: 1px dashed rgba(var(--v-theme-on-surface), 0.24);
}
.upload-dropzone--active {
  border-color: rgb(var(--v-theme-primary));
}
</style>
