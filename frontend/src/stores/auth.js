import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { auth } from '@/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('tessera_token') || '')
  const refreshToken = ref(localStorage.getItem('tessera_refresh_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('tessera_user') || 'null'))

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => !!user.value?.is_admin)

  // setAuth persists the {access_token, refresh_token, user} response shape.
  function setAuth(res) {
    token.value = res.access_token
    refreshToken.value = res.refresh_token || ''
    user.value = res.user || null
    localStorage.setItem('tessera_token', token.value)
    if (refreshToken.value) {
      localStorage.setItem('tessera_refresh_token', refreshToken.value)
    } else {
      localStorage.removeItem('tessera_refresh_token')
    }
    localStorage.setItem('tessera_user', JSON.stringify(user.value))
  }

  function logout() {
    token.value = ''
    refreshToken.value = ''
    user.value = null
    localStorage.removeItem('tessera_token')
    localStorage.removeItem('tessera_refresh_token')
    localStorage.removeItem('tessera_user')
  }

  async function login(email, password) {
    const res = await auth.login({ email, password })
    setAuth(res.data)
  }

  async function register(email, name, password) {
    const res = await auth.register({ email, name, password })
    setAuth(res.data)
  }

  // verify confirms a stored token is still valid on app start.
  async function verify() {
    if (!token.value) return
    try {
      const res = await auth.me()
      user.value = res.data
      localStorage.setItem('tessera_user', JSON.stringify(user.value))
    } catch {
      logout()
    }
  }

  return { token, user, isAuthenticated, isAdmin, login, register, logout, verify, setAuth }
})
