import { getCurrentInstance } from 'vue'
import { useMessage } from '@/composables/useMessage'

/**
 * The message API (composables/useMessage — the bottom toast lane, #3117) for code that may run without a component around it.
 *
 * Composables that write to the server (`useTaskMenu`, `useChartBars`) are called
 * from a component's `setup` in the app — but also directly out of specs, where
 * there is no instance to inject from and naive's own hook throws. A missing
 * message bus must not cost the caller its action, so this answers `null` and the
 * caller reports through `toast?.error(...)`.
 */
export function useToast() {
  if (!getCurrentInstance()) return null
  try {
    return useMessage()
  } catch {
    // No `<n-message-provider>` above this component — same deal as above.
    return null
  }
}
