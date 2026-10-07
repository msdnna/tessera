import { ref, onMounted, onUnmounted } from 'vue'

// useResponsive exposes a reactive `isMobile` flag (viewport ≤ 768px) backed by
// matchMedia. Used to swap the desktop sider for a drawer.
export function useResponsive(breakpoint = 768) {
  const isMobile = ref(false)
  let mql

  function update(e) {
    isMobile.value = e.matches
  }

  onMounted(() => {
    // No matchMedia (jsdom, very old webviews): stay on the desktop layout.
    if (typeof window.matchMedia !== 'function') return
    mql = window.matchMedia(`(max-width: ${breakpoint}px)`)
    isMobile.value = mql.matches
    mql.addEventListener('change', update)
  })
  onUnmounted(() => mql?.removeEventListener('change', update))

  return { isMobile }
}
