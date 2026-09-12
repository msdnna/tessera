<script setup>
import { computed, onMounted, onUnmounted, watch, watchEffect } from 'vue'
import { useRouter } from 'vue-router'
import { NConfigProvider, NGlobalStyle, NMessageProvider } from 'naive-ui'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { setI18nLocale } from '@/i18n'
import { naivePack } from '@/i18n/naive'
import { PRIORITY_COLORS } from '@/styles/tokens'
import AppConnectionOverlay from '@/components/AppConnectionOverlay.vue'
import UpdateToast from '@/components/UpdateToast.vue'
import ServerRail from '@/components/ServerRail.vue'
import ConferenceMiniWindow from '@/components/conference/ConferenceMiniWindow.vue'
import ConferenceAudioSink from '@/components/conference/ConferenceAudioSink.vue'
import {
  consumeOAuthState,
  listenDesktopOAuth,
  OAUTH_DONE_EVENT,
} from '@/composables/useDesktopOAuth'
import { isTauri } from '@/utils/serverBase'
import { useResponsive } from '@/composables/useResponsive'

const theme = useThemeStore()
const router = useRouter()

// Desktop server hotbar (#2930). It lives at the app root — NOT inside AppLayout —
// so it is reachable on the login and "no connection" screens too: those render
// outside AppLayout, and a stuck/expired server would otherwise trap the user
// with no way to switch away (the rework follow-up). It reserves a gutter via the
// global `--server-rail-w` custom property, which AppLayout and the connection
// overlay read to inset themselves past it.
const { isMobile } = useResponsive()
const showRail = computed(() => isTauri() && !isMobile.value)
watchEffect(() => {
  if (typeof document === 'undefined') return
  document.documentElement.style.setProperty('--server-rail-w', showRail.value ? '56px' : '0px')
})

// Language (#2797). The watch — rather than a call in the settings screen —
// is what makes every source of the preference land: the localStorage cache
// read at store init, the switch in SettingsView, and the server prefs that
// arrive after sign-in. Naive UI's own locale follows the same value.
const naiveLocale = computed(() => naivePack(theme.language).locale)
const naiveDateLocale = computed(() => naivePack(theme.language).dateLocale)
watch(() => theme.language, setI18nLocale, { immediate: true })

// Priority colours 1..4 (0 = "none", never gradient'd) → shared flag-icon gradients.
const PRIORITY_GRADS = PRIORITY_COLORS.slice(1)

// When a refresh ultimately fails, the api client clears auth and fires this.
// The store has to be cleared too, or the route guard still sees a signed-in
// session and bounces the redirect straight back home.
function onExpired() {
  useAuthStore().clearSession()
  router.push('/login')
}
onMounted(() => window.addEventListener('auth:expired', onExpired))
onUnmounted(() => window.removeEventListener('auth:expired', onExpired))

// Desktop GitLab login (#2696): the tessera:// handoff is caught here rather than in
// LoginView, because on Linux/Windows the OS delivers it through a *second* process
// launch — by the time it lands the visible screen can be anything, including the
// loader before the router settles. LoginView only needs to know the wait is over,
// which it learns from OAUTH_DONE_EVENT.
let unlistenOAuth = null
async function onDesktopOAuth({ access, refresh, state, error }) {
  // tessera:// is an OS-wide scheme, so this handler hears from any web page that cares
  // to open one. Only a handoff carrying the nonce of the login this app started is a
  // login; anything else is dropped without a trace — no routing, no OAUTH_DONE_EVENT,
  // so a forged link can neither sign the user in (#2932) nor cancel a real sign-in.
  if (!consumeOAuthState(state)) return
  let reason = error
  if (!reason) {
    try {
      await useAuthStore().loginWithTokens(access, refresh)
      window.dispatchEvent(new CustomEvent(OAUTH_DONE_EVENT, { detail: { error: null } }))
      router.replace('/')
      return
    } catch {
      reason = 'userinfo_failed'
    }
  }
  window.dispatchEvent(new CustomEvent(OAUTH_DONE_EVENT, { detail: { error: reason } }))
  if (!useAuthStore().isAuthenticated) {
    router.replace({ path: '/login', query: { oauth_error: reason } })
  }
}
onMounted(async () => {
  unlistenOAuth = await listenDesktopOAuth(onDesktopOAuth)
})
onUnmounted(() => unlistenOAuth?.())
</script>

<template>
  <!-- Global accent gradient for SVG icons (referenced via fill/stroke:
       url(#t-accent-grad-svg) by the .grad-icon helper). Stops follow the theme
       via --t-primary. -->
  <svg width="0" height="0" aria-hidden="true" style="position: absolute">
    <defs>
      <linearGradient id="t-accent-grad-svg" x1="0" y1="1" x2="1" y2="0">
        <stop offset="0" style="stop-color: color-mix(in srgb, var(--t-primary) 86%, #000)" />
        <stop offset="0.5" style="stop-color: var(--t-primary)" />
        <stop offset="1" style="stop-color: color-mix(in srgb, var(--t-primary) 86%, #fff)" />
      </linearGradient>
      <!-- Error gradient for destructive popconfirm icons. -->
      <linearGradient id="t-error-grad-svg" x1="0" y1="1" x2="1" y2="0">
        <stop offset="0" style="stop-color: #b33030" />
        <stop offset="0.5" style="stop-color: #e5484d" />
        <stop offset="1" style="stop-color: #f58181" />
      </linearGradient>
      <!-- Priority flag gradients (one per level, indices 1..4 of PRIORITY_COLORS).
           Shared so a board with 100s of cards holds 4 defs, not one SVG per card. -->
      <linearGradient
        v-for="(c, i) in PRIORITY_GRADS"
        :id="`t-prio-grad-${i + 1}`"
        :key="i"
        x1="0"
        y1="1"
        x2="1"
        y2="0"
      >
        <stop offset="0" :style="{ stopColor: `color-mix(in srgb, ${c} 86%, #000)` }" />
        <stop offset="0.5" :style="{ stopColor: c }" />
        <stop offset="1" :style="{ stopColor: `color-mix(in srgb, ${c} 86%, #fff)` }" />
      </linearGradient>
    </defs>
  </svg>
  <n-config-provider
    :theme="theme.naiveTheme"
    :theme-overrides="theme.themeOverrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
  >
    <n-global-style />
    <n-message-provider>
      <!-- Desktop server hotbar, docked at the app root so it survives on the
           login and offline screens (which live outside AppLayout). A fixed
           gutter on the left; AppLayout and the connection overlay inset
           themselves past it via --server-rail-w. -->
      <div v-if="showRail" class="app-rail-dock"><ServerRail /></div>
      <router-view v-slot="{ Component }">
        <transition name="fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
      <!-- Minimised conference (#2888): a floating window that keeps a call alive
           across route changes. Paints nothing unless a session is live and the
           open route is not that call's own room. -->
      <conference-mini-window />
      <!-- The call's audio, mounted for the life of the session so it plays on
           every route — the mini-window shows one tile, but the room must still
           be heard in full (#2888). -->
      <conference-audio-sink />
    </n-message-provider>
    <!-- Global "server slow / unreachable" overlay (branded loader + retry). -->
    <app-connection-overlay />
    <!-- Global "new version deployed" toast (PWA prompt-mode update, #2748).
         Web only: mounting it calls useRegisterSW(), which registers the service
         worker. On desktop the SW has no business running — it precaches the app
         shell and then serves a stale one across app updates (#2930 rework; see
         the desktop SW purge in main.js) — and desktop updates ship as new app
         builds, not PWA prompts. Gated so the worker is never even registered. -->
    <update-toast v-if="!isTauri()" />
  </n-config-provider>
</template>

<style scoped>
/* Server hotbar gutter, pinned to the viewport so it stays put across the login,
   offline and in-app screens alike (#2930). Width comes from the rail itself
   (56px). Kept below Naive's teleported modals/dropdowns (~2000+) so the rail's
   own edit modal opens over it, and the connection overlay insets past it rather
   than covering it — so a server can be switched even while the current one is
   unreachable. */
.app-rail-dock {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  z-index: 5;
}
</style>
