import { isTauri } from './serverBase'

// copyText — write a string to the clipboard, resolving to whether it worked.
// Order: the Tauri clipboard plugin on desktop (WebKitGTK's navigator.clipboard /
// execCommand are unreliable there), then the async Clipboard API (secure web
// contexts), then a hidden textarea + execCommand('copy') for insecure origins /
// older webviews. Returns false only when every path fails, so callers can decide
// how to surface success/failure (toast, icon swap, …).
export async function copyText(text) {
  const value = String(text ?? '')
  if (isTauri()) {
    try {
      const { writeText } = await import('@tauri-apps/plugin-clipboard-manager')
      await writeText(value)
      return true
    } catch {
      /* fall through to the browser paths */
    }
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
      return true
    }
  } catch {
    /* fall through to the execCommand path */
  }
  try {
    const el = document.createElement('textarea')
    el.value = value
    // Off-screen but selectable. Must be focused before the copy or execCommand
    // copies nothing (e.g. when a modal traps focus). Preserve any existing page
    // selection and restore it afterwards.
    el.style.position = 'fixed'
    el.style.left = '-9999px'
    el.style.top = '0'
    el.setAttribute('readonly', '')
    const prevActive = document.activeElement
    const selection = document.getSelection()
    const savedRange = selection && selection.rangeCount ? selection.getRangeAt(0) : null
    document.body.appendChild(el)
    el.focus({ preventScroll: true })
    el.setSelectionRange(0, value.length)
    let ok = false
    try {
      ok = document.execCommand('copy')
    } finally {
      document.body.removeChild(el)
      if (savedRange && selection) {
        selection.removeAllRanges()
        selection.addRange(savedRange)
      }
      if (prevActive && prevActive.focus) prevActive.focus({ preventScroll: true })
    }
    return ok
  } catch {
    return false
  }
}
