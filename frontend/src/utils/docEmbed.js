// The contract between the document editor and the Android app that embeds it
// (#2894 §4). Kept out of the view on purpose: what the host may pass in, what
// it hears back and when the page is allowed to run at all is the interface —
// and an interface that only exists inside a mounted component is one nobody
// can test.
//
// Direction matters here. Page → host goes through a `@JavascriptInterface`
// object, so every argument crosses as a *string*; host → page goes through
// `window.tesseraEmbed`, installed by the view.
import { SUPPORTED_LOCALES } from '@/i18n'

/** The bridge object Android injects. Absent in a desktop browser — opening the
 *  embed URL by hand is how the page is developed, and it must still work. */
export const EMBED_BRIDGE = 'AndroidDoc'

function str(v) {
  return typeof v === 'string' ? v.trim() : ''
}

/**
 * Reads the launch parameters out of the embed URL.
 *
 * The access token travels in the URL rather than in a header because nothing
 * else can: the WebView's first request is a navigation. It is the short-lived
 * *access* token only — the refresh token stays in the app (see the token hook
 * in DocEmbedView), so the two sides never rotate each other's session out.
 *
 * `ready` is the gate: without a token and a workspace there is nothing to
 * load, and rendering an empty editor would invite typing into a void.
 */
export function parseEmbedParams(query = {}) {
  const token = str(query.token)
  const workspaceId = str(query.ws)
  const lang = str(query.lang)
  return {
    token,
    workspaceId,
    dark: str(query.theme) === 'dark',
    locale: SUPPORTED_LOCALES.includes(lang) ? lang : '',
    ready: Boolean(token && workspaceId),
  }
}

/**
 * The single word the host shows above the editor. One derivation, one place:
 * the status chip is native (Compose), so the page cannot draw it, and two
 * sides guessing separately is how "сохранено" ends up over unsaved text.
 *
 * Order is the priority: a conflict outranks an error (it stops saving
 * altogether), an error outranks work in progress, and "dirty" only means the
 * debounce has not fired yet.
 */
export function embedStatus({ saving = false, dirty = false, conflict = false, error = '' } = {}) {
  if (conflict) return 'conflict'
  if (error) return 'error'
  if (saving) return 'saving'
  if (dirty) return 'dirty'
  return 'saved'
}

/**
 * Calls a bridge method if the host provides it.
 *
 * Every miss is silent and returns false: the same page opens in a browser
 * during development, an older app build may not know a newer event, and
 * neither is a reason to throw inside a watcher and take the editor down.
 */
export function sendToHost(name, payload, host = globalThis[EMBED_BRIDGE]) {
  const fn = host && host[name]
  if (typeof fn !== 'function') return false
  const arg =
    payload === undefined
      ? undefined
      : typeof payload === 'string'
        ? payload
        : JSON.stringify(payload)
  try {
    if (arg === undefined) fn.call(host)
    else fn.call(host, arg)
    return true
  } catch {
    // A dead bridge (the WebView is being torn down) must not surface as an
    // editor error — there is no one left to show it to.
    return false
  }
}
