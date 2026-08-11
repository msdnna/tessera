import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { auth, setAccessToken, restoreSession } from '@/api'
import { useThemeStore } from './theme'

export const useAuthStore = defineStore('auth', () => {
  // #2684: the access token is deliberately NOT read back from localStorage —
  // it lives in memory (see api/index.js) and a page load starts signed out
  // until bootstrap() trades the refresh cookie for a fresh one. `tessera_user`
  // is still cached: it holds no secret, only the profile the UI paints with.
  const token = ref('')
  const refreshToken = ref(localStorage.getItem('tessera_refresh_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('tessera_user') || 'null'))

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => !!user.value?.is_admin)

  // setToken keeps the store and the axios layer on the same access token.
  function setToken(value) {
    token.value = value || ''
    setAccessToken(token.value)
  }

  // setAuth persists the {access_token, refresh_token, user, preferences} shape
  // and hydrates the theme/preferences store from the server. `refresh_token` is
  // only present in body mode (desktop/mobile); on web it arrived as a cookie.
  function setAuth(res) {
    setToken(res.access_token)
    refreshToken.value = res.refresh_token || ''
    user.value = res.user || null
    if (refreshToken.value) {
      localStorage.setItem('tessera_refresh_token', refreshToken.value)
    } else {
      localStorage.removeItem('tessera_refresh_token')
    }
    localStorage.setItem('tessera_user', JSON.stringify(user.value))
    if (res.preferences) useThemeStore().hydrate(res.preferences)
  }

  // clearSession drops every local trace of the session. Kept separate from
  // logout() because expiry paths need it without a server round-trip.
  function clearSession() {
    setToken('')
    refreshToken.value = ''
    user.value = null
    localStorage.removeItem('tessera_refresh_token')
    localStorage.removeItem('tessera_user')
    useThemeStore().reset()
  }

  // logout revokes the session server-side — the refresh token outlives the tab
  // by 30 days, and in cookie mode the client cannot delete it at all.
  //
  // Order matters: the request is fired while the token is still available, then
  // local state is cleared SYNCHRONOUSLY, before the first await. Callers that
  // navigate without awaiting (the sidebar button does) would otherwise hit the
  // route guard while the store still says "signed in" and bounce back home.
  // The revoke is best-effort: offline must not leave anyone stuck signed in.
  function logout() {
    const revoked = auth.logout().catch(() => {})
    clearSession()
    return revoked
  }

  async function login(email, password) {
    const res = await auth.login({ email, password })
    setAuth(res.data)
  }

  async function register(email, name, password) {
    const res = await auth.register({ email, name, password })
    setAuth(res.data)
  }

  // setUser refreshes the cached user after a profile edit.
  function setUser(u) {
    user.value = u
    localStorage.setItem('tessera_user', JSON.stringify(u))
  }

  // loginWithTokens completes an OAuth redirect handoff. The callback delivers
  // only the access token in the URL fragment now — the refresh token came back
  // as a cookie, precisely so it can't be read out of window.location or the
  // browser history. `refresh` stays in the signature for the desktop/mobile
  // hand-off, which still carries one.
  async function loginWithTokens(access, refresh) {
    setToken(access)
    refreshToken.value = refresh || ''
    if (refreshToken.value) localStorage.setItem('tessera_refresh_token', refreshToken.value)
    else localStorage.removeItem('tessera_refresh_token')
    const res = await auth.me()
    user.value = res.data.user
    localStorage.setItem('tessera_user', JSON.stringify(user.value))
    if (res.data.preferences) useThemeStore().hydrate(res.data.preferences)
  }

  // bootstrap restores the session before the app renders: with the access token
  // held in memory only, every reload starts signed out and has to trade the
  // refresh cookie for a new one. Callers must await this BEFORE the router is
  // installed, or the first navigation guard bounces a signed-in user to /login.
  //
  // The cached user is the "was signed in here" hint — without it we'd fire a
  // pointless refresh (and eat a 401) on every anonymous visit to the login page.
  async function bootstrap() {
    if (!localStorage.getItem('tessera_user')) return
    const access = await restoreSession()
    if (!access) {
      clearSession()
      return
    }
    setToken(access)
    await verify()
  }

  // verify confirms the token is still valid and refreshes the cached profile.
  async function verify() {
    if (!token.value) return
    try {
      const res = await auth.me()
      user.value = res.data.user
      localStorage.setItem('tessera_user', JSON.stringify(user.value))
      if (res.data.preferences) useThemeStore().hydrate(res.data.preferences)
    } catch {
      clearSession()
    }
  }

  return {
    token,
    user,
    isAuthenticated,
    isAdmin,
    login,
    register,
    logout,
    clearSession,
    bootstrap,
    verify,
    setAuth,
    setUser,
    loginWithTokens,
  }
})
