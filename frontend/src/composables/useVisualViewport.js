import { ref, onMounted, onBeforeUnmount } from 'vue'

// The on-screen keyboard on a phone (v2, #2976 — package G «Клавиатура»).
//
// A mobile browser shrinks the *visual* viewport when the keyboard comes up and
// leaves the layout one alone, so a bottom sheet pinned to `bottom: 0` ends up
// under the keys. This publishes the keyboard's height as `--kb-h` on the root
// (sheets with a field add it to their bottom offset) and a flag the shell uses
// to put the bottom navigation away while typing. In Tauri the WebView reports
// the IME inset the same way.
//
// 120px is the floor for «that's a keyboard»: the URL bar collapsing on scroll
// changes the visual viewport by ~56px and must not count.
const KEYBOARD_MIN = 120

export function useVisualViewport() {
  const keyboardOpen = ref(false)
  const keyboardHeight = ref(0)
  let vv = null

  function update() {
    if (!vv) return
    const h = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop))
    const open = h > KEYBOARD_MIN
    keyboardHeight.value = open ? h : 0
    keyboardOpen.value = open
    document.documentElement.style.setProperty('--kb-h', `${keyboardHeight.value}px`)
  }

  onMounted(() => {
    vv = window.visualViewport || null
    if (!vv) return
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    update()
  })
  onBeforeUnmount(() => {
    vv?.removeEventListener('resize', update)
    vv?.removeEventListener('scroll', update)
    document.documentElement.style.removeProperty('--kb-h')
  })

  return { keyboardOpen, keyboardHeight }
}
