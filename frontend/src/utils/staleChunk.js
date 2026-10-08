// Recovery from a stale build (Sentry TESSERA-FRONTEND «Failed to fetch
// dynamically imported module»). A tab opened before a deploy still holds the
// old index, whose lazy chunks (`AppLayout-<hash>.js`) are gone from the server:
// the next route change fails to import and the screen stays empty. A full load
// of the target URL fetches the new index and its chunks.
//
// One reload per target within RELOAD_WINDOW_MS: if the fresh page fails the same
// way, the problem is not staleness, and looping would hide it from Sentry.

const KEY = 'tessera_stale_chunk_reload'
export const RELOAD_WINDOW_MS = 30_000

const STALE_PATTERNS = [
  'Failed to fetch dynamically imported module', // Chromium
  'error loading dynamically imported module', // Firefox
  'Importing a module script failed', // Safari
  'Unable to preload CSS', // Vite's CSS preload helper
]

export function isStaleChunkError(err) {
  const text = String(err?.message || err || '')
  return STALE_PATTERNS.some((p) => text.includes(p))
}

// reloadOnce navigates to `url` with a full page load unless the same URL was
// already reloaded for this reason within the window. Returns whether it did.
export function reloadOnce(url, { storage = sessionStorage, now = Date.now(), go } = {}) {
  let last
  try {
    last = JSON.parse(storage.getItem(KEY) || 'null')
  } catch {
    last = null
  }
  if (last && last.url === url && now - last.at < RELOAD_WINDOW_MS) return false
  try {
    storage.setItem(KEY, JSON.stringify({ url, at: now }))
  } catch {
    /* private mode: reload anyway, the window guard is best-effort */
  }
  ;(go || ((u) => window.location.assign(u)))(url)
  return true
}

// installStaleChunkRecovery wires both entry points: the router (a lazy route
// component failed to import) and Vite's `vite:preloadError` (a preload of a
// dependency chunk failed before the import itself).
export function installStaleChunkRecovery(router) {
  router.onError((err, to) => {
    if (!isStaleChunkError(err)) return
    reloadOnce(to?.fullPath ? router.resolve(to.fullPath).href : window.location.href)
  })
  window.addEventListener('vite:preloadError', (event) => {
    if (reloadOnce(window.location.href)) event.preventDefault()
  })
}
