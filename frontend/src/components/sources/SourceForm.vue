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
    <v-alert
      v-else-if="form.kind === 'youtube'"
      type="info"
      density="compact"
      class="mb-4"
    >
      YouTube's terms of service apply. Playback uses yt-dlp to resolve a stream URL on this host.
      Automated access can get an account restricted. Cookies stay on this machine and are only
      needed when YouTube still treats it as a bot, or for private, members-only, or age-restricted videos.
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
    <v-text-field
      v-model.number="form.options.buffer_seconds"
      label="Buffer (seconds)"
      type="number"
      min="0"
      max="30"
      step="1"
      hint="Seconds behind live. Higher is smoother. YouTube, HLS, and DASH default to 4."
      persistent-hint
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      class="mb-4"
    />
    <div class="d-flex align-center flex-wrap switch-cluster">
      <v-switch
        v-model="form.enabled"
        label="Enabled"
        color="primary"
        inset
        density="compact"
        hide-details="auto"
      />
      <v-switch
        v-model="form.options.force_software"
        label="Force software decode"
        color="primary"
        inset
        density="compact"
        hide-details="auto"
      />
      <v-switch
        v-if="form.kind === 'rtsp'"
        v-model="form.options.tls_verify"
        label="Verify TLS certificate (RTSPS)"
        color="primary"
        inset
        density="compact"
        hide-details="auto"
      />
    </div>
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
      tls_verify: src.options?.tls_verify === true,
      buffer_seconds: bufferSecondsFromSource(src),
      force_software: src.options?.force_software === true,
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
    if (!props.source) {
      form.options.buffer_seconds = defaultBufferSeconds(kind)
    }
  },
)

const urlLabel = computed(() => (form.kind === 'youtube' ? 'YouTube URL' : 'URL'))
const passwordLabel = computed(() => (props.source?.has_password ? 'Password (leave blank to keep)' : 'Password'))

function defaultBufferSeconds(kind) {
  return kind === 'youtube' || kind === 'hls' || kind === 'dash' ? 4 : 0
}

function bufferSecondsFromSource(src) {
  if (src.options?.buffer_ms != null && src.options.buffer_ms !== '') {
    return Math.round(Number(src.options.buffer_ms) / 1000)
  }
  return defaultBufferSeconds(src.kind)
}

function emptyForm() {
  return {
    name: '',
    kind: 'rtsp',
    url: '',
    username: '',
    password: '',
    enabled: true,
    options: {
      transport: 'tcp',
      upload_id: null,
      tls_verify: false,
      buffer_seconds: 0,
      force_software: false,
    },
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
    options: {
      force_software: !!form.options.force_software,
      buffer_ms: Math.max(0, Math.min(30000, Math.round(Number(form.options.buffer_seconds) * 1000) || 0)),
    },
  }
  if (form.kind === 'rtsp') {
    body.options.transport = form.options.transport || 'tcp'
    body.options.tls_verify = !!form.options.tls_verify
    if (form.password) body.password = form.password
  }
  if (form.kind === 'file') {
    if (form.options.upload_id) body.options.upload_id = form.options.upload_id
  }
  return body
}

defineExpose({ payload })
</script>
