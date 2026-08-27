import { ref } from 'vue'
import { serverBase } from '@/utils/serverBase'

// Runtime-resolved app-download links for the web login screen.
//
// Correctness of the links (and their versions) is the whole point: instead of
// baking a version into the frontend build, we fetch the SAME manifests the
// self-update mechanisms already publish and derive the URLs from them. Bumping
// Android/Desktop + republishing its manifest makes the buttons point at the new
// file with no frontend rebuild.
//
//   Android : <base>/apks/latest.json     → { apk, version }
//   Desktop : <base>/desktop/latest.json   → { version, downloads: { linux[], windows[] } }
//             (falls back to the updater `platforms` block for older manifests)
//
// On web serverBase() is '' so these are same-origin ('/apks/…', '/desktop/…'),
// served by the site's nginx (deploy/docker-compose.yml bind-mounts). On dev
// (Vite) the manifests 404 → the button simply stays hidden.

// A variant carries its wire-format id only; the human label is resolved where
// it is drawn (DownloadMenu). Naming it here would freeze the label at fetch
// time — the manifests are read once at login and the menu outlives a language
// switch (#2799).

async function fetchJson(url) {
  try {
    const r = await fetch(url, { cache: 'no-store' })
    if (!r.ok) return null
    return await r.json()
  } catch {
    return null
  }
}

function normalizeVariants(list, base) {
  if (!Array.isArray(list)) return null
  const out = list
    .map((v) => {
      if (!v) return null
      // `file` = bare filename → resolve relative to the serving origin (same as
      // the Android APK); `url` = legacy absolute URL (older manifests).
      const url = v.file ? `${base}/desktop/${v.file}` : v.url
      if (!url) return null
      return { format: v.format || 'file', url }
    })
    .filter(Boolean)
  return out.length ? out : null
}

function fallbackVariant(url, format) {
  return url ? [{ format, url }] : null
}

// detectOS reports the visitor's platform. Android's UA also contains "Linux",
// so it must be checked first.
function detectOS() {
  if (typeof navigator === 'undefined') return 'other'
  const ua = navigator.userAgent || ''
  if (/android/i.test(ua)) return 'android'
  const plat = (navigator.userAgentData?.platform || navigator.platform || '').toLowerCase()
  if (plat.includes('win') || /windows/i.test(ua)) return 'windows'
  if (plat.includes('linux') || /linux/i.test(ua)) return 'linux'
  return 'other' // mac / iOS / unknown — no build offered yet
}

export function useDownloads() {
  const loading = ref(true)
  const detected = ref(detectOS())
  // Each: { version, variants: [{ format, url }] } | null
  const android = ref(null)
  const windows = ref(null)
  const linux = ref(null)

  async function load() {
    const base = serverBase()
    const [apk, desk] = await Promise.all([
      fetchJson(`${base}/apks/latest.json`),
      fetchJson(`${base}/desktop/latest.json`),
    ])

    if (apk?.apk) {
      android.value = {
        version: apk.version || '',
        variants: [{ format: 'apk', url: `${base}/apks/${apk.apk}` }],
      }
    }

    if (desk) {
      const dl = desk.downloads || {}
      const lin =
        normalizeVariants(dl.linux, base) ||
        fallbackVariant(desk.platforms?.['linux-x86_64']?.url, 'appimage')
      if (lin) linux.value = { version: desk.version || '', variants: lin }

      const win =
        normalizeVariants(dl.windows, base) ||
        fallbackVariant(desk.platforms?.['windows-x86_64']?.url, 'exe')
      if (win) windows.value = { version: desk.version || '', variants: win }
    }

    loading.value = false
  }
  load()

  return { loading, detected, android, windows, linux }
}
