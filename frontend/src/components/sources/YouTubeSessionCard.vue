<template>
  <StandardCard title="YouTube" title-class="text-h5" card-class="mb-4">
    <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>
    <v-alert v-if="success" type="success" density="compact" class="mb-4">{{ success }}</v-alert>
    <v-alert
      v-if="liveErrorCode === 'youtube_bot_check' || liveErrorCode === 'youtube_auth'"
      type="error"
      density="compact"
      class="mb-4"
    >
      {{ liveError || 'YouTube playback failed.' }}
    </v-alert>

    <div class="field-row" :class="potError ? 'mb-2' : 'mb-4'">
      <div>
        <p class="text-body-2 mb-2">Token provider</p>
        <div class="d-flex align-center flex-wrap" style="gap: 8px">
          <v-chip size="small" :color="potColor" variant="flat">{{ potLabel }}</v-chip>
          <span v-if="ytDlpVersion" class="text-caption text-medium-emphasis">yt-dlp {{ ytDlpVersion }}</span>
        </div>
      </div>
      <div>
        <p class="text-body-2 mb-2">Cookies</p>
        <div class="d-flex align-center flex-wrap" style="gap: 8px">
          <v-chip size="small" :color="status.configured ? 'success' : 'grey'" variant="flat">
            {{ status.configured ? 'Uploaded' : 'None' }}
          </v-chip>
          <span v-if="status.configured && status.updated_at" class="text-caption text-medium-emphasis">
            Updated {{ formatWhen(status.updated_at) }}
          </span>
        </div>
      </div>
    </div>
    <p v-if="potError" class="text-caption text-medium-emphasis mb-4">{{ potError }}</p>

    <p v-if="!configOpen.includes('config')" class="text-body-2 text-medium-emphasis mb-4">
      Most streams play with no extra setup. If YouTube asks you to sign in or treats this
      computer as a bot, open Configuration and upload cookies from a browser that can watch the stream.
    </p>

    <v-expansion-panels v-model="configOpen" multiple flat variant="accordion">
      <v-expansion-panel value="config">
        <v-expansion-panel-title>
          <div class="d-flex align-center flex-grow-1 flex-wrap pe-2" style="gap: 8px; min-width: 0;">
            <span class="text-subtitle-1">Configuration</span>
            <span class="text-body-2 text-medium-emphasis">Cookies and optional token</span>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <p class="text-body-2 mb-4">
            Export a Netscape <code>cookies.txt</code> from a browser that can watch the stream on this machine.
            Private, members-only, or age-restricted videos also need cookies.
            On a Mac you can run <code>./scripts/export-youtube-cookies.sh</code> from Terminal (Chrome may prompt for the keychain).
            Automated playback can get the account restricted.
            <a
              href="https://github.com/yt-dlp/yt-dlp/wiki/Extractors#exporting-youtube-cookies"
              target="_blank"
              rel="noopener noreferrer"
            >How to export cookies</a>
          </p>
          <p v-if="cookieDetail" class="text-caption text-medium-emphasis mb-4">{{ cookieDetail }}</p>
          <p v-if="potSetup" class="text-caption text-medium-emphasis mb-4">Provider setup: {{ potSetup }}</p>

          <v-sheet
            class="upload-dropzone pa-4 mb-4"
            :class="{ 'upload-dropzone--active': dragover }"
            @dragover.prevent="onDrag(true)"
            @dragleave.prevent="onDrag(false)"
            @drop.prevent="onDrop"
          >
            <div class="text-body-2 mb-2">Drop cookies.txt here or choose a file.</div>
            <v-progress-linear
              v-if="progress != null"
              class="mb-4"
              :model-value="progress * 100"
              color="primary"
            />
            <div class="d-flex align-center flex-wrap" style="gap: 8px">
              <v-btn
                variant="outlined"
                size="small"
                :disabled="!isAdmin"
                :loading="uploading"
                @click="input?.click()"
              >
                Upload cookies
              </v-btn>
              <v-btn
                v-if="status.configured && isAdmin"
                variant="text"
                size="small"
                color="error"
                :loading="removing"
                @click="removeCookies"
              >
                Remove cookies
              </v-btn>
              <span v-if="fileName" class="text-caption">{{ fileName }}</span>
            </div>
            <input
              ref="input"
              type="file"
              accept=".txt,text/plain"
              class="d-none"
              @change="onPick"
            >
          </v-sheet>

          <PasswordField
            :model-value="poToken"
            label="PO token override (optional)"
            autocomplete="new-password"
            hint="Leave blank unless you were given a token to paste."
            persistent-hint
            :disabled="!isAdmin"
            @update:model-value="onPOToken"
          />
          <div v-if="isAdmin" class="d-flex justify-end" style="gap: 8px">
            <v-btn
              v-if="status.po_token"
              variant="text"
              size="small"
              color="error"
              :loading="savingToken"
              @click="removePOToken"
            >
              Remove token
            </v-btn>
            <v-btn
              color="primary"
              variant="elevated"
              size="small"
              :loading="savingToken"
              :disabled="!isAdmin || !poDirty"
              @click="savePOToken"
            >
              Save
            </v-btn>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </StandardCard>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import PasswordField from '@/components/common/PasswordField.vue'
import StandardCard from '@/components/common/StandardCard.vue'
import { api } from '@/utils/api'
import { useFeedback } from '@/composables/useFeedback'

const props = defineProps({
  isAdmin: {
    type: Boolean,
    default: false,
  },
  ytDlpVersion: {
    type: String,
    default: '',
  },
  lastErrorCode: {
    type: String,
    default: '',
  },
  lastError: {
    type: String,
    default: '',
  },
})

const { error, success, showError, showSuccess } = useFeedback()
const liveErrorCode = computed(() => props.lastErrorCode || status.value.last_error_code)
const liveError = computed(() => props.lastError || status.value.last_error)

const status = ref({
  configured: false,
  youtube_hosts: 0,
  po_token: false,
  last_error_code: '',
  last_error: '',
  pot: { mode: 'off', running: false, base_url: '', version: '', last_error: '' },
})
const dragover = ref(false)
const progress = ref(null)
const uploading = ref(false)
const removing = ref(false)
const savingToken = ref(false)
const fileName = ref('')
const input = ref(null)
const poToken = ref('')
const poDirty = ref(false)
const configOpen = ref([])
const pot = computed(() => status.value.pot || {})
const potLabel = computed(() => {
  const p = pot.value
  if (p.running) return p.version ? `Running · ${p.version}` : 'Running'
  if (p.mode && p.mode !== 'off') return 'Not responding'
  return 'Off'
})
const potColor = computed(() => {
  const p = pot.value
  if (p.running) return 'success'
  if (p.mode && p.mode !== 'off') return 'warning'
  return 'grey'
})
const potError = computed(() => pot.value.last_error || '')
const potSetup = computed(() => {
  const p = pot.value
  const bits = []
  if (p.mode) bits.push(p.mode)
  if (p.base_url) bits.push(p.base_url)
  return bits.join(' · ')
})
const cookieDetail = computed(() => {
  const bits = []
  if (status.value.youtube_hosts) bits.push(`${status.value.youtube_hosts} YouTube cookies in the file`)
  if (status.value.po_token) bits.push('A PO token override is saved')
  return bits.join(' · ')
})

watch(
  liveErrorCode,
  (code) => {
    if (code === 'youtube_bot_check' || code === 'youtube_auth') {
      configOpen.value = ['config']
    }
  },
  { immediate: true },
)

async function load() {
  try {
    status.value = await api.get('api/v1/system/youtube')
  } catch (err) {
    console.log('[YouTubeSession] API error:', err)
    showError(err.message || 'Failed to load YouTube session')
  }
}

async function send(file) {
  if (!props.isAdmin) return
  fileName.value = file.name
  progress.value = 0
  uploading.value = true
  try {
    status.value = await api.upload('api/v1/system/youtube/cookies', file, (p) => {
      progress.value = p
    }, 'PUT')
    showSuccess('Cookies uploaded')
  } catch (err) {
    console.log('[YouTubeSession] API error:', err)
    showError(err.message || 'Failed to upload cookies')
  } finally {
    progress.value = null
    uploading.value = false
  }
}

function onPick(ev) {
  const file = ev.target.files?.[0]
  if (file) send(file)
  ev.target.value = ''
}

function onDrag(active) {
  if (!props.isAdmin) return
  dragover.value = active
}

function onDrop(ev) {
  dragover.value = false
  if (!props.isAdmin) return
  const file = ev.dataTransfer.files?.[0]
  if (file) send(file)
}

async function removeCookies() {
  removing.value = true
  try {
    status.value = await api.delete('api/v1/system/youtube/cookies')
    fileName.value = ''
    showSuccess('Cookies removed')
  } catch (err) {
    console.log('[YouTubeSession] API error:', err)
    showError(err.message || 'Failed to remove cookies')
  } finally {
    removing.value = false
  }
}

function onPOToken(v) {
  poToken.value = v
  poDirty.value = true
}

async function savePOToken() {
  savingToken.value = true
  try {
    status.value = await api.put('api/v1/system/youtube/po-token', { po_token: poToken.value })
    poToken.value = ''
    poDirty.value = false
    showSuccess(status.value.po_token ? 'PO token saved' : 'PO token removed')
  } catch (err) {
    console.log('[YouTubeSession] API error:', err)
    showError(err.message || 'Failed to save PO token')
  } finally {
    savingToken.value = false
  }
}

async function removePOToken() {
  poToken.value = ''
  poDirty.value = false
  savingToken.value = true
  try {
    status.value = await api.put('api/v1/system/youtube/po-token', { po_token: '' })
    showSuccess('PO token removed')
  } catch (err) {
    console.log('[YouTubeSession] API error:', err)
    showError(err.message || 'Failed to remove PO token')
  } finally {
    savingToken.value = false
  }
}

function formatWhen(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString()
}

onMounted(async () => {
  await load()
})
</script>

<style scoped>
.upload-dropzone {
  border: 1px dashed rgba(var(--v-theme-on-surface), 0.24);
}
.upload-dropzone--active {
  border-color: rgb(var(--v-theme-primary));
}
</style>
