<template>
  <v-container class="page-content">
    <v-row justify="center">
      <v-col cols="12" sm="8" md="6">
        <StandardCard title="Login" title-class="text-h5">
          <v-form @submit.prevent="submit">
            <v-alert v-if="error" type="error" density="compact" class="mb-4">
              {{ error }}
            </v-alert>
            <v-text-field
              v-model="username"
              label="Username"
              variant="outlined"
              density="comfortable"
              hide-details="auto"
              class="mb-4"
              autocomplete="username"
            />
            <v-text-field
              v-model="password"
              label="Password"
              type="password"
              variant="outlined"
              density="comfortable"
              hide-details="auto"
              class="mb-4"
              autocomplete="current-password"
            />
            <div class="d-flex justify-end mt-4">
              <v-btn type="submit" color="primary" variant="elevated" :loading="loading">
                Login
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
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    if (auth.mustChangePassword) {
      router.push('/change-password')
    } else {
      router.push('/')
    }
  } catch (err) {
    console.log('[Login] API error:', err)
    error.value = err.message || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>
