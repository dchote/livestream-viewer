<template>
  <v-app-bar app flat class="content-header">
    <v-app-bar-title class="font-weight-bold">
      <router-link to="/" class="app-header-link text-decoration-none">
        <span class="brand-text">livestream-viewer</span>
      </router-link>
    </v-app-bar-title>
    <v-spacer />
    <v-btn
      icon
      variant="text"
      size="small"
      class="px-2"
      :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
      @click="toggleTheme"
    >
      <v-icon>{{ isDark ? 'mdi-weather-sunny' : 'mdi-weather-night' }}</v-icon>
    </v-btn>
    <v-btn
      v-if="route.path !== '/login' && route.path !== '/change-password'"
      size="small"
      variant="elevated"
      color="primary"
      class="ml-2"
      to="/login"
    >
      Login
    </v-btn>
  </v-app-bar>

  <v-main>
    <slot />
  </v-main>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useTheme } from 'vuetify'
import { THEME_KEY } from '@/plugins/vuetify'

const route = useRoute()
const theme = useTheme()
const isDark = computed(() => theme.global.current.value.dark)

function toggleTheme() {
  const next = theme.global.current.value.dark ? 'light' : 'dark'
  theme.change(next)
  localStorage.setItem(THEME_KEY, next)
}
</script>

<style scoped>
.content-header {
  background: rgb(var(--v-theme-surface)) !important;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.v-theme--dark .content-header {
  border-bottom-color: rgba(255, 255, 255, 0.12);
}

.app-header-link {
  display: flex;
  align-items: center;
  color: rgb(var(--v-theme-on-surface));
}

.brand-text {
  font-weight: 600;
}
</style>
