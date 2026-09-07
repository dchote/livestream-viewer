<template>
  <v-container class="page-content">
    <v-row justify="center">
      <v-col cols="12" sm="8" md="6">
        <StandardCard title="Change password" title-class="text-h5">
          <p class="text-body-2 text-medium-emphasis mb-4">
            You must set a new password before continuing.
          </p>
          <v-form @submit.prevent="submit">
            <v-alert v-if="error" type="error" density="compact" class="mb-4">
              {{ error }}
            </v-alert>
            <v-text-field
              v-model="currentPassword"
              label="Current password"
              type="password"
              variant="outlined"
              density="comfortable"
              hide-details="auto"
              class="mb-4"
              autocomplete="current-password"
            />
            <v-text-field
              v-model="newPassword"
              label="New password"
              type="password"
              variant="outlined"
              density="comfortable"
              hide-details="auto"
              class="mb-4"
              autocomplete="new-password"
              hint="At least 8 characters"
              persistent-hint
            />
            <v-text-field
              v-model="confirmPassword"
              label="Confirm new password"
              type="password"
              variant="outlined"
              density="comfortable"
              hide-details="auto"
              class="mb-4"
              autocomplete="new-password"
            />
            <div class="d-flex justify-end mt-4">
              <v-btn variant="text" class="mr-2" @click="logout">Logout</v-btn>
              <v-btn type="submit" color="primary" variant="elevated" :loading="loading">
                Change password
              </v-btn>
            </div>
          </v-form>
        </StandardCard>
      </v-col>
    </v-row>
  </v-container>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import StandardCard from '@/components/common/StandardCard.vue'
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
  if (newPassword.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
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
