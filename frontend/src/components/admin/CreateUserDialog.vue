<template>
  <StandardDialog
    :model-value="modelValue"
    title="Create user"
    max-width="400"
    :fullscreen="mobile"
    @update:model-value="$emit('update:modelValue', $event)"
    @close="onClose"
  >
    <v-alert v-if="error" type="error" density="compact" class="mb-4">
      {{ error }}
    </v-alert>
    <v-text-field
      v-model="form.username"
      label="Username"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      class="mb-4"
      style="max-width: 320px;"
    />
    <v-text-field
      v-model="form.password"
      label="Temporary password"
      type="password"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="new-password"
      class="mb-4"
      style="max-width: 320px;"
      hint="At least 8 characters. They will be asked to change it on login."
      persistent-hint
    />
    <v-select
      v-model="form.role"
      :items="ROLE_OPTIONS"
      label="Role"
      variant="outlined"
      density="compact"
      hide-details="auto"
      autocomplete="off"
      class="mb-4"
      style="max-width: 320px;"
    />
    <template #actions>
      <v-spacer />
      <v-btn variant="text" class="mr-2" @click="$emit('update:modelValue', false)">Cancel</v-btn>
      <v-btn color="primary" variant="elevated" :loading="loading" @click="submit">
        Create
      </v-btn>
    </template>
  </StandardDialog>
</template>

<script setup>
import { reactive, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import StandardDialog from '@/components/common/StandardDialog.vue'
import { ROLE_OPTIONS, ROLE_USER } from '@/utils/roles'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  loading: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'close', 'save'])

const { mobile } = useDisplay()
const error = ref('')
const form = reactive({
  username: '',
  password: '',
  role: ROLE_USER,
})

function resetForm() {
  form.username = ''
  form.password = ''
  form.role = ROLE_USER
  error.value = ''
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) resetForm()
  },
)

function onClose() {
  resetForm()
  emit('close')
}

function submit() {
  error.value = ''
  const username = form.username.trim()
  if (!username) {
    error.value = 'Username is required'
    return
  }
  if (form.password.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  emit('save', {
    username,
    password: form.password,
    role: form.role,
  })
}

defineExpose({
  setError(message) {
    error.value = message || ''
  },
})
</script>
