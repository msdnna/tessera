import { ref, watch } from 'vue'
import api from '@/api'
import { isTauri, absolutizeApiUrl } from '@/utils/serverBase'

// On the desktop app the webview won't load cross-origin <img> from the remote
// server (the page origin is the tauri:// app protocol), but axios/fetch works —
// so pull the image bytes over axios and hand back a same-origin blob: URL.
//
// Blobs are cached by absolute URL (avatars repeat across many cards) and kept
// for the session — small, and avoids re-fetching. Web is unaffected: images are
// same-origin there, so callers use the URL directly.
const cache = new Map() // absolute url -> objectURL (string)

export async function fetchImageObjectURL(url) {
  const abs = absolutizeApiUrl(url)
  const hit = cache.get(abs)
  if (hit) return hit
  const res = await api.get(abs, { responseType: 'blob', skipLoader: true })
  const obj = URL.createObjectURL(res.data)
  cache.set(abs, obj)
  return obj
}

// useApiImage(getUrl) → a ref holding the <img> src to bind. On web it's the URL
// unchanged; on desktop it resolves to a cached blob: URL fetched via axios.
export function useApiImage(getUrl) {
  const src = ref('')
  async function load(u) {
    if (!u) {
      src.value = ''
      return
    }
    if (!isTauri()) {
      src.value = u
      return
    }
    const want = u
    try {
      const obj = await fetchImageObjectURL(u)
      // Ignore a resolved fetch for a URL we've since moved off of.
      if (getUrl() === want) src.value = obj
    } catch {
      // Blob fetch failed (e.g. the proxy 302-redirected cross-origin and the XHR
      // hit CORS). Fall back to the direct URL so a plain <img> can still follow
      // the redirect / load it (no-cors); onerror then shows initials.
      if (getUrl() === want) src.value = absolutizeApiUrl(want)
    }
  }
  watch(getUrl, load, { immediate: true })
  return src
}
