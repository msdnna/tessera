import { useRegisterSW } from 'virtual:pwa-register/vue'

// One source of truth for the "a new build is deployed" state (#2748).
//
// vite-plugin-pwa runs in `prompt` mode: the new service worker installs in the
// background and waits, flipping `needRefresh` to true instead of reloading on
// its own. The update toast watches that flag; `updateServiceWorker(true)`
// activates the waiting worker and reloads the page onto the new build.
//
// In dev (no SW is built) the virtual module is a no-op and `needRefresh` stays
// false, so nothing renders.
// How often a focused, untouched tab re-checks for a fresh deploy. The SW only
// checks on navigation by default, so a board left open would otherwise miss a
// `make up` until the next reload (#2779 — the toast used to appear only after
// F5). sw.js is served `no-cache`, so each check is a cheap conditional fetch.
const POLL_MS = 60 * 1000

export function useAppUpdate() {
  const { needRefresh, updateServiceWorker } = useRegisterSW({
    immediate: true,
    onRegisteredSW(_swUrl, registration) {
      if (!registration) return
      const check = () => {
        // Skip work while the tab is hidden — visibilitychange re-checks the
        // moment it comes back, which is when a new build actually matters.
        if (document.visibilityState === 'visible') registration.update()
      }
      // Poll on a short interval so a deploy surfaces within ~a minute even on a
      // tab that's just sitting there, and re-check immediately whenever the user
      // returns to the tab (the common case: deploy happens while it's in the
      // background) — together this makes the toast appear promptly without a
      // manual reload.
      setInterval(check, POLL_MS)
      document.addEventListener('visibilitychange', check)
      window.addEventListener('focus', check)
    },
  })
  return { needRefresh, updateServiceWorker }
}
