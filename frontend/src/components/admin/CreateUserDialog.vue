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
    />
    <PasswordField
      v-model="form.password"
      label="Temporary password"
      autocomplete="new-password"
      :hint="`${PASSWORD_HINT}. They will be asked to change it on login.`"
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
import PasswordField from '@/components/common/PasswordField.vue'
import { PASSWORD_HINT, PASSWORD_TOO_SHORT, passwordTooShort } from '@/utils/passwords'
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
  if (passwordTooShort(form.password)) {
    error.value = PASSWORD_TOO_SHORT
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
