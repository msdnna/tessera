import { ref } from 'vue'
import { meta } from '@/api'

// Running versions of both services, for the sidebar version badge (#2747).
//
// Web build info is compile-time: vite.config.js bakes __APP_VERSION__ /
// __APP_COMMIT__ / __BUILD_DATE__ into the bundle. The API's info comes from
// GET /api/version and is fetched once, then shared by every caller (the badge
// is mounted once, but keeping the cache module-level costs nothing and is
// robust to remounts).

const web = {
  version: typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '',
  commit: typeof __APP_COMMIT__ !== 'undefined' ? __APP_COMMIT__ : '',
  builtAt: typeof __BUILD_DATE__ !== 'undefined' ? __BUILD_DATE__ : '',
}

const api = ref(null)
let inflight = null

function loadApi() {
  if (api.value || inflight) return inflight
  inflight = meta
    .version()
    .then((r) => {
      api.value = {
        version: r.data?.api || '',
        commit: r.data?.commit || '',
        builtAt: r.data?.built_at || '',
      }
    })
    .catch(() => {
      /* offline / unauthorized — the badge falls back to web-only */
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

export function useVersionInfo() {
  loadApi()
  return { web, api }
}
