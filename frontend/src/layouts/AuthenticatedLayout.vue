<template>
  <v-navigation-drawer
    v-model="drawer"
    v-model:rail="rail"
    :permanent="!isMobile"
    :temporary="isMobile"
    :mobile-breakpoint="960"
    app
    class="sidebar-nav"
    width="260"
    rail-width="56"
  >
    <div class="sidebar-brand pa-4">
      <router-link to="/" class="d-flex align-center text-decoration-none">
        <span v-show="!rail" class="sidebar-brand-text">livestream-viewer</span>
      </router-link>
    </div>
    <v-divider class="sidebar-divider" />
    <v-list nav density="comfortable" class="sidebar-list">
      <v-list-item
        prepend-icon="mdi-monitor"
        title="Preview"
        to="/"
        :active="route.path === '/'"
        rounded="lg"
        class="sidebar-item"
      />
      <div v-show="!rail" class="sidebar-section-label px-4 py-2">SETTINGS</div>
      <v-list-item
        prepend-icon="mdi-video-input-antenna"
        title="Stream Sources"
        to="/settings/sources"
        :active="route.path.startsWith('/settings/sources')"
        rounded="lg"
        class="sidebar-item"
      />
      <v-list-item
        prepend-icon="mdi-view-grid-plus"
        title="Display Strategy"
        to="/settings/display"
        :active="route.path.startsWith('/settings/display')"
        rounded="lg"
        class="sidebar-item"
      />
      <v-list-item
        v-if="auth.isAdmin"
        prepend-icon="mdi-account-multiple"
        title="Users"
        to="/admin/users"
        :active="route.path.startsWith('/admin/users')"
        rounded="lg"
        class="sidebar-item"
      />
    </v-list>
  </v-navigation-drawer>

  <v-app-bar app flat class="content-header">
    <v-btn
      v-if="isMobile || rail"
      icon
      variant="text"
      class="app-bar-icon-btn px-2"
      :title="isMobile ? 'Menu' : (rail ? 'Expand' : 'Collapse')"
      @click="toggleDrawer"
    >
      <v-icon>mdi-menu</v-icon>
    </v-btn>
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
    <v-menu location="bottom end">
      <template #activator="{ props: menuProps }">
        <v-btn v-bind="menuProps" variant="text" class="user-menu-btn px-3">
          <v-avatar color="primary" size="36" class="mr-3">
            <span class="text-body-2 font-weight-medium">{{ userInitial }}</span>
          </v-avatar>
          <div class="text-left d-none d-sm-flex flex-column">
            <span class="text-body-2 font-weight-medium">{{ user?.username }}</span>
            <span class="text-caption text-medium-emphasis">{{ userRoleLabel }}</span>
          </div>
          <v-icon class="ml-2">mdi-chevron-down</v-icon>
        </v-btn>
      </template>
      <v-list density="comfortable">
        <v-list-item prepend-icon="mdi-logout" title="Logout" @click="logout" />
      </v-list>
    </v-menu>
  </v-app-bar>

  <v-main>
    <slot />
  </v-main>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useTheme, useDisplay } from 'vuetify'
import { THEME_KEY } from '@/plugins/vuetify'
import { useAuthStore } from '@/stores/auth'
import { roleLabel } from '@/utils/roles'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const theme = useTheme()
const { width } = useDisplay()
const drawer = ref(null)
const rail = ref(false)
const isMobile = computed(() => width.value < 960)
const user = computed(() => auth.user)
const isDark = computed(() => theme.global.current.value.dark)
const userRoleLabel = computed(() => roleLabel(user.value?.role))
const userInitial = computed(() => (user.value?.username?.[0] || '?').toUpperCase())

watch(width, (w) => {
  if (w < 960) {
    drawer.value = false
    rail.value = false
  } else if (w < 1280) {
    drawer.value = true
    rail.value = true
  } else {
    drawer.value = true
    rail.value = false
  }
}, { immediate: true })

function toggleDrawer() {
  if (isMobile.value) {
    drawer.value = !drawer.value
  } else {
    rail.value = !rail.value
  }
}

function toggleTheme() {
  const next = theme.global.current.value.dark ? 'light' : 'dark'
  theme.change(next)
  localStorage.setItem(THEME_KEY, next)
}

async function logout() {
  auth.logout()
  router.push('/login')
}

onMounted(() => {
  if (auth.token) {
    auth.fetchMe()
  }
})
</script>

<style scoped>
.sidebar-nav {
  background: #1f2937 !important;
  border-right: none !important;
}

.sidebar-nav :deep(.v-navigation-drawer__content) {
  background: #1f2937;
}

.sidebar-brand-text {
  color: white;
  font-weight: 600;
  font-size: 1.1rem;
}

.sidebar-section-label {
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.sidebar-item :deep(.v-list-item--active) {
  background: rgba(255, 255, 255, 0.1) !important;
  color: white !important;
}

.sidebar-nav :deep(.v-list-item) {
  color: rgba(255, 255, 255, 0.85);
}

.sidebar-nav :deep(.v-list-item:hover) {
  background: rgba(255, 255, 255, 0.05);
  color: white;
}

.sidebar-divider {
  border-color: rgba(255, 255, 255, 0.12) !important;
}

.content-header {
  background: #1f2937 !important;
  border-bottom: 1px solid rgba(255, 255, 255, 0.12);
}

.content-header :deep(*) {
  color: rgba(255, 255, 255, 0.9);
}

.content-header :deep(.text-medium-emphasis) {
  color: rgba(255, 255, 255, 0.7) !important;
}

.user-menu-btn {
  text-transform: none;
}

.app-bar-icon-btn {
  flex-shrink: 0;
}
</style>
