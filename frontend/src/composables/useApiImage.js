import { computed } from 'vue'
import { absolutizeApiUrl } from '@/utils/serverBase'

// Resolve an <img> src for a backend-provided media URL. On web it's unchanged
// (same-origin '/api/…'); on desktop the relative '/api/…' URL is absolutized to
// the configured server. The Tauri webview loads remote cross-origin <img> fine,
// so a plain <img src> works — no blob fetch needed. `getUrl` is a getter; returns
// a computed string ('' when there's no URL).
export function useApiImage(getUrl) {
  return computed(() => absolutizeApiUrl(getUrl()) || '')
}
