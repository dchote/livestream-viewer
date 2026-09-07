<template>
  <v-container class="page-content page-narrow">
    <StandardCard title="Change password" title-class="text-h5">
      <p class="text-body-2 text-medium-emphasis mb-4">
        You must set a new password before continuing.
      </p>
      <v-form @submit.prevent="submit">
        <v-alert v-if="error" type="error" density="compact" class="mb-4">
          {{ error }}
        </v-alert>
        <PasswordField
          v-model="currentPassword"
          label="Current password"
          density="comfortable"
          autocomplete="current-password"
        />
        <PasswordField
          v-model="newPassword"
          label="New password"
          density="comfortable"
          autocomplete="new-password"
          :hint="PASSWORD_HINT"
          persistent-hint
        />
        <PasswordField
          v-model="confirmPassword"
          label="Confirm new password"
          density="comfortable"
          autocomplete="new-password"
        />
        <div class="d-flex justify-end">
          <v-btn variant="text" class="mr-2" @click="logout">Logout</v-btn>
          <v-btn type="submit" color="primary" variant="elevated" :loading="loading">
            Change password
          </v-btn>
        </div>
      </v-form>
    </StandardCard>
  </v-container>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import StandardCard from '@/components/common/StandardCard.vue'
import PasswordField from '@/components/common/PasswordField.vue'
import { PASSWORD_HINT, PASSWORD_TOO_SHORT, passwordTooShort } from '@/utils/passwords'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)

function logout() {
  auth.logout()
  router.push('/login')
}

async function submit() {
  error.value = ''
  if (passwordTooShort(newPassword.value)) {
    error.value = PASSWORD_TOO_SHORT
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'New passwords do not match'
    return
  }
  loading.value = true
  try {
    await auth.changePassword(currentPassword.value, newPassword.value)
    router.push('/')
  } catch (err) {
    console.log('[ChangePassword] API error:', err)
    error.value = err.message || 'Failed to change password'
  } finally {
    loading.value = false
  }
}
</script>
