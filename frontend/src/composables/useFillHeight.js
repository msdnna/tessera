import { onBeforeUnmount, onMounted, ref } from 'vue'

// The height that takes a board view from where it starts down to the bottom of
// the window, less `bottom` px (#2973): the calendar grid and the matrix fill the
// screen whatever sits above them — the sub-toolbar, the stage strip, a banner.
//
// Re-measured on a window resize and whenever the board around the view changes
// size (the stage strip opening above it). The second measure after our own
// height settles reads the same value, so the observer comes to rest.
export function useFillHeight(el, { bottom = 16, min = 360 } = {}) {
  const height = ref(null)
  let ro = null
  function measure() {
    const node = el.value
    if (!node || typeof window === 'undefined') return
    const top = node.getBoundingClientRect().top
    // A phone's bottom navigation (#2976) is not the view's to fill.
    const nav = document.querySelector('[data-testid="mobile-tabbar"]')?.offsetHeight || 0
    const h = Math.max(min, Math.round(window.innerHeight - top - bottom - nav))
    if (h !== height.value) height.value = h
  }
  onMounted(() => {
    measure()
    window.addEventListener('resize', measure)
    const host = el.value?.closest?.('.board-wrap') || el.value?.parentElement
    if (typeof ResizeObserver !== 'undefined' && host) {
      ro = new ResizeObserver(() => measure())
      ro.observe(host)
    }
  })
  onBeforeUnmount(() => {
    window.removeEventListener('resize', measure)
    ro?.disconnect()
  })
  return { height, measure }
}
