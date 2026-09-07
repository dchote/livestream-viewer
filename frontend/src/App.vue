<template>
  <v-app>
    <AuthenticatedLayout v-if="showAppShell" key="authenticated">
      <router-view />
    </AuthenticatedLayout>
    <DefaultLayout v-else key="default">
      <router-view />
    </DefaultLayout>
  </v-app>
</template>

<script setup>
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import AuthenticatedLayout from '@/layouts/AuthenticatedLayout.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'

const auth = useAuthStore()
const showAppShell = computed(() => auth.isAuthenticated && !auth.mustChangePassword)
</script>
