import { createRouter, createWebHistory } from 'vue-router'
import { routes } from 'vue-router/auto-routes'
import { getIngressBase } from '@/utils/ingress'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(getIngressBase()),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.path !== '/login' && !auth.isAuthenticated) {
    return { path: '/login' }
  }
  if (to.path === '/login' && auth.isAuthenticated) {
    return { path: auth.mustChangePassword ? '/change-password' : '/' }
  }
  if (auth.isAuthenticated && auth.mustChangePassword && to.path !== '/change-password') {
    return { path: '/change-password' }
  }
  if (to.path === '/change-password' && auth.isAuthenticated && !auth.mustChangePassword) {
    return { path: '/' }
  }
  if (to.path.startsWith('/admin') && !auth.isAdmin) {
    return { path: '/' }
  }
  return true
})

export default router
