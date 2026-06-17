import { watch, onBeforeUnmount } from 'vue'

// Make an overlay (modal / drawer / note editor) close on the browser's Back
// button instead of letting Back navigate away from the page.
//
// While `isOpen` is true we push a throwaway history entry; a `popstate` (Back)
// pops it and we call `close()` rather than navigating. Closing via the UI
// (the X / Esc / Save path flips `isOpen` to false) unwinds the pushed entry so
// the history stays balanced and Back keeps working on the page underneath.
//
//   useOverlayBack(showRef, () => (showRef.value = false))
//
// `isOpen` must be a ref<boolean>; `close` flips it to false (and may do more).
export function useOverlayBack(isOpen, close) {
  let pushed = false

  function detach() {
    window.removeEventListener('popstate', onPop)
  }

  function onPop() {
    // Back was pressed while the overlay is open: our dummy entry is already
    // gone, so just tear down and close the overlay (no further nav).
    if (!isOpen.value) return
    pushed = false
    detach()
    close()
  }

  watch(isOpen, (open) => {
    if (open && !pushed) {
      pushed = true
      window.history.pushState({ ...window.history.state, tOverlay: true }, '')
      window.addEventListener('popstate', onPop)
    } else if (!open && pushed) {
      // Closed from the UI — remove our entry so we don't strand it in history.
      pushed = false
      detach()
      if (window.history.state && window.history.state.tOverlay) window.history.back()
    }
  })

  onBeforeUnmount(() => {
    if (pushed) detach()
  })
}
