import { onMounted, onBeforeUnmount } from 'vue'
import { attach } from '@/shortcuts/dispatcher'

// Attaches handlers to catalog shortcuts for the lifetime of the calling
// component (v2, #2956): `useShortcuts({ 'card.open': open }, { when })`.
// `when` gates the whole set — e.g. «a card is selected» — and is read at the
// moment of the keystroke, so it can be any reactive-free getter.
export function useShortcuts(map, { when } = {}) {
  let detach = []
  onMounted(() => {
    detach = Object.entries(map).map(([id, run]) => attach(id, run, when))
  })
  onBeforeUnmount(() => {
    detach.forEach((d) => d())
    detach = []
  })
}
