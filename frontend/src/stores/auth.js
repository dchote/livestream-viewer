import { defineStore } from 'pinia'
import { api, getToken, setToken, getStoredUser, setStoredUser } from '@/utils/api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: null,
    user: null,
  }),
  getters: {
    isAuthenticated: (s) => Boolean(s.token),
    isAdmin: (s) => s.user?.role === 'admin',
    mustChangePassword: (s) => Boolean(s.user?.must_change_password),
  },
  actions: {
    persist() {
      setToken(this.token)
      setStoredUser(this.user)
    },
    restore() {
      const token = getToken()
      const user = getStoredUser()
      if (token) {
        this.token = token
        this.user = user
      }
    },
    async login(username, password) {
      const data = await api.post('api/v1/auth/login', { username, password })
      this.token = data.token
      this.user = data.user
      this.persist()
      return data
    },
    async fetchMe() {
      if (!this.token) return
      try {
        this.user = await api.get('api/v1/auth/me')
        this.persist()
      } catch (err) {
        console.log('[auth] fetchMe failed:', err)
        if (err.status === 401) {
          this.logout()
        }
      }
    },
    async changePassword(currentPassword, newPassword) {
      const user = await api.post('api/v1/auth/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
      })
      this.user = user
      this.persist()
      return user
    },
    logout() {
      this.token = null
      this.user = null
      setToken(null)
      setStoredUser(null)
    },
  },
})
