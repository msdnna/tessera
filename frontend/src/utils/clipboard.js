// copyText — write a string to the clipboard, resolving to whether it worked.
// Prefers the async Clipboard API (secure contexts), falling back to a hidden
// textarea + execCommand('copy') for insecure origins / older webviews (Tauri
// WebKitGTK, http:// on LAN). Returns false only when both paths fail, so callers
// can decide how to surface success/failure (toast, icon swap, …).
export async function copyText(text) {
  const value = String(text ?? '')
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
    // Keep it out of view and out of the layout / scroll.
    el.style.position = 'fixed'
    el.style.top = '-9999px'
    el.setAttribute('readonly', '')
    document.body.appendChild(el)
    el.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(el)
    return ok
  } catch {
    return false
  }
}
