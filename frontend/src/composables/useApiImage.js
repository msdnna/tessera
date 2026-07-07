import { computed, toValue } from 'vue'
import { absolutizeApiUrl } from '@/utils/serverBase'

// Resolve an <img> src for a backend-provided media URL. On web it's unchanged
// (same-origin '/api/…'); on desktop the relative '/api/…' URL is absolutized to
// the configured server. The Tauri webview loads remote cross-origin <img> fine,
// so a plain <img src> works — no blob fetch needed. `src` may be a ref, a getter,
// or a plain string (toValue handles all three); returns a computed string.
export function useApiImage(src) {
  return computed(() => absolutizeApiUrl(toValue(src)) || '')
}
