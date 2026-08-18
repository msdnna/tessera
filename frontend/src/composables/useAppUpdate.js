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
export function useAppUpdate() {
  const { needRefresh, updateServiceWorker } = useRegisterSW({
    immediate: true,
    onRegisteredSW(_swUrl, registration) {
      // The SW only checks for an update on navigation by default; a long-lived
      // tab (a board left open all day) would miss a deploy. Poll hourly so it
      // still surfaces the toast without a manual reload.
      if (registration) {
        setInterval(() => registration.update(), 60 * 60 * 1000)
      }
    },
  })
  return { needRefresh, updateServiceWorker }
}
