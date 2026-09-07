<template>
  <div>
    <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
    <div class="field-row mb-4">
      <v-text-field
        v-model="form.name"
        label="Name"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
      />
      <v-select
        v-model="form.kind"
        :items="kindItems"
        label="Kind"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
      />
    </div>
    <v-alert
      v-if="form.kind === 'youtube' && systemInfo && systemInfo.yt_dlp === false"
      type="warning"
      density="compact"
      class="mb-4"
    >
      yt-dlp is not installed on this host. YouTube sources can be saved, but probing and playback need yt-dlp on PATH.
    </v-alert>
    <div v-if="form.kind === 'file'" class="mb-4">
      <UploadDropzone @uploaded="onUploaded" />
      <div v-if="form.options.upload_id" class="text-caption">Using upload #{{ form.options.upload_id }}</div>
    </div>
    <v-text-field
      v-else
      v-model="form.url"
      :label="urlLabel"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      class="mb-4"
    />
    <div v-if="form.kind === 'rtsp'" class="field-row mb-4">
      <v-select
        v-model="form.options.transport"
        :items="['tcp', 'udp']"
        label="Transport"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
        style="max-width: 140px;"
      />
      <v-text-field
        v-model="form.username"
        label="Username"
        variant="outlined"
        density="compact"
        hide-details="auto"
        autocomplete="off"
      />
      <PasswordField
        v-model="form.password"
        :label="passwordLabel"
        autocomplete="new-password"
        field-class=""
      />
    </div>
    <v-switch
      v-model="form.enabled"
      label="Enabled"
      color="primary"
      inset
      density="compact"
      hide-details="auto"
    />
  </div>
</template>

<script setup>
import { computed, reactive, watch } from 'vue'
import PasswordField from '@/components/common/PasswordField.vue'
import UploadDropzone from '@/components/sources/UploadDropzone.vue'
import { kindLabel } from '@/utils/formatters'

const props = defineProps({
  source: {
    type: Object,
    default: null,
  },
  systemInfo: {
    type: Object,
    default: null,
  },
  error: {
    type: String,
    default: '',
  },
})

const kindItems = ['youtube', 'rtsp', 'hls', 'dash', 'http', 'srt', 'rtmp', 'file'].map((k) => ({
  title: kindLabel(k),
  value: k,
}))

const form = reactive(emptyForm())

watch(
  () => props.source,
  (src) => {
    Object.assign(form, emptyForm())
    if (!src) return
    form.name = src.name || ''
    form.kind = src.kind || 'rtsp'
    form.url = src.url || ''
    form.username = src.username || ''
    form.password = ''
    form.enabled = src.enabled !== false
    form.options = {
      transport: src.options?.transport || 'tcp',
      upload_id: src.options?.upload_id || null,
    }
  },
  { immediate: true },
)

// Credentials only apply to RTSP. Clear them when the kind changes so a
// half-edited form cannot submit a username for a scheme that never uses one.
watch(
  () => form.kind,
  (kind) => {
    if (kind !== 'rtsp') {
      form.username = ''
      form.password = ''
    }
  },
)

const urlLabel = computed(() => (form.kind === 'youtube' ? 'YouTube URL' : 'URL'))
const passwordLabel = computed(() => (props.source?.has_password ? 'Password (leave blank to keep)' : 'Password'))

function emptyForm() {
  return {
    name: '',
    kind: 'rtsp',
    url: '',
    username: '',
    password: '',
    enabled: true,
    options: { transport: 'tcp', upload_id: null },
  }
}

function onUploaded(up) {
  form.options.upload_id = up.id
  form.url = up.path || ''
}

function payload() {
  const body = {
    name: form.name.trim(),
    kind: form.kind,
    url: form.url.trim(),
    username: form.username,
    enabled: form.enabled,
    options: {},
  }
  if (form.kind === 'rtsp') {
    body.options.transport = form.options.transport || 'tcp'
    if (form.password) body.password = form.password
  }
  if (form.kind === 'file') {
    if (form.options.upload_id) body.options.upload_id = form.options.upload_id
  }
  return body
}

defineExpose({ payload })
</script>
