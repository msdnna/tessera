<script setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AuthLayout from '@/components/AuthLayout.vue'
import LoaderOverlay from '@/components/LoaderOverlay.vue'

const router = useRouter()
const authStore = useAuthStore()

// The backend OAuth callback hands the access token back in the URL fragment
// (#access_token=..) — fragments aren't sent to the server or written to access
// logs. Since #2684 the refresh token is NOT in there: it arrives as an httpOnly
// cookie, because a fragment still lingers in browser history and in
// window.location, where injected script could read it. Parse it, establish the
// session, then clean the URL.
onMounted(async () => {
  const frag = new URLSearchParams((window.location.hash || '').replace(/^#/, ''))
  const access = frag.get('access_token')
  const refresh = frag.get('refresh_token') // web: absent; mobile hand-off only
  if (!access) {
    router.replace({ path: '/login', query: { oauth_error: 'no_token' } })
    return
  }
  try {
    await authStore.loginWithTokens(access, refresh)
    // Drop the fragment from history so tokens don't linger in the URL.
    window.history.replaceState(null, '', window.location.pathname)
    router.replace('/')
  } catch {
    router.replace({ path: '/login', query: { oauth_error: 'userinfo_failed' } })
  }
})
</script>

<template>
  <auth-layout title="Вход через GitLab">
    <div class="oauth-wait">
      <loader-overlay :show="true" contained :messages="['Завершаем вход…']" />
    </div>
  </auth-layout>
</template>

<style scoped>
.oauth-wait {
  position: relative;
  min-height: 120px;
}
</style>
