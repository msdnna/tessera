// Navigating out of an overlay that owns a history entry (useOverlayBack).
//
// An open overlay — the «Входящие» sheet, the palette, a large window — pushes a
// throwaway history entry so the system Back closes it. Closing it from the UI
// unwinds that entry with `history.back()`, which lands asynchronously; a
// `router.push` made in the same tick is then undone by that very back(). So
// whoever closes an overlay in order to go somewhere waits for the unwind first
// (the pattern ProjectWindow.navigate introduced, #2971).
//
//   await unwindOverlay(() => inbox.close())
//   router.push('/more')
export async function unwindOverlay(close) {
  const ours = !!window.history.state?.tOverlay
  close?.()
  if (!ours) return
  await new Promise((resolve) => {
    const done = () => {
      window.removeEventListener('popstate', done)
      clearTimeout(timer)
      resolve()
    }
    const timer = setTimeout(done, 400)
    window.addEventListener('popstate', done)
  })
}
