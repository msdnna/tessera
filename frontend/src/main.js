import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import { initSentry } from './utils/sentry'
import { installStaleChunkRecovery } from './utils/staleChunk'
import { i18n } from './i18n'
import { ensureRegistry, isTauri } from './utils/servers'
import './styles/main.css'

// On desktop, evict any service worker the webview may still be running. A build
// shipped before the SW was gated to web-only (vite.config.js) could register one
// in WebView2's persistent profile, which then keeps serving its precached, stale
// app shell across app updates — the on-disk bundle is ignored and the app is
// frozen on an old version (no sidebar, wrong version tag). Unregister it and drop
// its caches, then reload once onto the fresh bundle. No-op on web (the SW is
// wanted there) and on later runs once nothing is registered. Returns true when a
// worker was found (i.e. this load was SW-controlled and needs a clean reload).
async function purgeDesktopServiceWorker() {
  if (!isTauri() || !('serviceWorker' in navigator)) return false
  const regs = await navigator.serviceWorker.getRegistrations().catch(() => [])
  if (!regs.length) return false
  await Promise.all(regs.map((r) => r.unregister().catch(() => {})))
  if (window.caches) {
    const keys = await caches.keys().catch(() => [])
    await Promise.all(keys.map((k) => caches.delete(k).catch(() => {})))
  }
  return true
}

async function boot() {
  // A sessionStorage guard keeps the post-purge reload from looping if the reload
  // somehow comes back still controlled.
  if ((await purgeDesktopServiceWorker()) && !sessionStorage.getItem('tessera_sw_purged')) {
    sessionStorage.setItem('tessera_sw_purged', '1')
    location.reload()
    return
  }

  // #2930: on desktop, fold a pre-multi-server install into the server registry
  // before anything reads a session key — the migration renames those keys into
  // the first profile's namespace, and a read that beat it would look in the
  // wrong place and start the app signed out. No-op on web and on later runs.
  ensureRegistry()

  const app = createApp(App)
  app.use(createPinia())
  // Starts on `ru`; App.vue watches the persisted language preference and
  // switches (loading the locale chunk) from there — see #2797.
  app.use(i18n)

  // #2787: browser telemetry. Started here, but deliberately NOT awaited before
  // mount — it fetches its config from the backend, and gating the first paint on
  // that would add a round trip for anonymous visitors, whose bootstrap() below
  // returns immediately. Sentry therefore misses errors thrown in the first few
  // hundred ms of the initial render; everything after is captured. initSentry()
  // never rejects (a disabled or unreachable backend is a normal outcome).
  initSentry(app, router)

  // #2684: the access token lives in memory, so every page load starts signed out
  // and the session is restored from the httpOnly refresh cookie. This has to
  // finish BEFORE the router is installed — the first navigation guard would
  // otherwise run against an empty store and bounce a signed-in user to /login on
  // every reload. bootstrap() never rejects and its request is time-bounded, so a
  // dead backend delays the first paint rather than blocking it forever.
  await useAuthStore().bootstrap()
  // A tab left open across a deploy reloads instead of stalling on a vanished chunk.
  installStaleChunkRecovery(router)
  app.use(router)
  app.mount('#app')
}

boot()
