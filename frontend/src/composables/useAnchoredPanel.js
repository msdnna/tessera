import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { pushBlocker } from '@/shortcuts/dispatcher'

// useAnchoredPanel places a floating panel next to an anchor and runs the
// chores every v2 popover needs (#2969): fixed position clamped to the viewport,
// re-placed on resize and scroll; a click outside closes it; global shortcuts
// are blocked while it is open; focus goes back where it came from on close.
//
//   show       ref<boolean>
//   anchor     () => Element | { x, y } | null — read on every placement
//   panel      ref<Element> — the panel itself (for measuring and click-outside)
//   onOutside  called on a pointerdown outside panel and anchor
//   placement  'right-start' (to the right, tops aligned) | 'bottom-start' |
//              'bottom-end' | 'point' (at the {x, y} itself)
//   offset     { x, y } nudges from the anchor edge (mockup: 4px right, −6px up)
export function useAnchoredPanel({
  show,
  anchor,
  panel,
  onOutside,
  placement = 'right-start',
  offset = { x: 4, y: -6 },
}) {
  const style = ref({ left: '0px', top: '0px' })
  let release = null
  let returnFocus = null

  function anchorRect() {
    const a = anchor()
    if (!a) return null
    if (typeof a.getBoundingClientRect === 'function') return a.getBoundingClientRect()
    if (typeof a.x === 'number')
      return { left: a.x, right: a.x, top: a.y, bottom: a.y, width: 0, height: 0 }
    return null
  }

  function place() {
    const r = anchorRect()
    const el = panel.value
    if (!r || !el) return
    const w = el.offsetWidth
    const h = el.offsetHeight
    let left
    let top
    switch (placement) {
      case 'bottom-start':
        left = r.left + offset.x
        top = r.bottom + offset.y
        break
      case 'bottom-end':
        left = r.right - w + offset.x
        top = r.bottom + offset.y
        break
      case 'point':
        left = r.left + offset.x
        top = r.top + offset.y
        break
      default:
        left = r.right + offset.x
        top = r.top + offset.y
    }
    const margin = 8
    const vw = window.innerWidth
    const vh = window.innerHeight
    // Not enough room on the right: open to the left of the anchor instead.
    if (placement === 'right-start' && left + w > vw - margin) left = r.left - w - offset.x
    left = Math.min(Math.max(margin, left), Math.max(margin, vw - w - margin))
    top = Math.min(Math.max(margin, top), Math.max(margin, vh - h - margin))
    style.value = { left: `${Math.round(left)}px`, top: `${Math.round(top)}px` }
  }

  function inside(target) {
    if (!(target instanceof Node)) return false
    if (panel.value?.contains(target)) return true
    const a = anchor()
    if (a && typeof a.contains === 'function' && a.contains(target)) return true
    // A popover's own pickers (Naive selects, menus, a nested icon search) live
    // in teleported layers: clicking them is not "outside".
    const el = target instanceof Element ? target : target.parentElement
    return !!el?.closest(
      '.n-popover, .n-dropdown-menu, .n-modal-container, .n-base-select-menu, .v-binder-follower-container',
    )
  }

  function onPointerDown(e) {
    if (!inside(e.target)) onOutside?.(e)
  }

  function open() {
    returnFocus = document.activeElement
    release = pushBlocker()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    document.addEventListener('pointerdown', onPointerDown, true)
    nextTick(place)
  }

  function close() {
    release?.()
    release = null
    window.removeEventListener('resize', place)
    window.removeEventListener('scroll', place, true)
    document.removeEventListener('pointerdown', onPointerDown, true)
    const back = returnFocus
    returnFocus = null
    if (back?.isConnected) back.focus?.({ preventScroll: true })
  }

  watch(
    show,
    (on) => {
      if (on) open()
      else close()
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    if (release) close()
  })

  return { style, place }
}
