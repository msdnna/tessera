import { onBeforeUnmount } from 'vue'
import { pressMoved } from '@/utils/dnd'

// Explicit touch long-press → opens a context menu. Needed where the native
// `contextmenu` event is unreliable on touch (e.g. draggable sidebar rows inside
// the mobile drawer). Bails out if the finger moved (that's a drag), reusing the
// global movement tracker. `open` receives a synthetic { clientX, clientY }.
export function useLongPress(open, delay = 450) {
  let timer = null
  let x = 0
  let y = 0
  function start(e) {
    const t = e.touches && e.touches[0]
    if (!t) return
    x = t.clientX
    y = t.clientY
    clearTimeout(timer)
    timer = setTimeout(() => {
      if (!pressMoved()) open({ clientX: x, clientY: y })
    }, delay)
  }
  function cancel() {
    clearTimeout(timer)
  }
  onBeforeUnmount(cancel)
  return { start, cancel }
}
