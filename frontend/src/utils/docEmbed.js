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
import { blockIdsInOrder, blockNodeById, quoteFromBlock } from '@/utils/docComments'

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
 * The payload of a "discuss this block" tap (#2894 §5).
 *
 * The threads themselves are drawn natively — there is no room beside the text
 * on a phone for the panel the web has — so everything the host needs to show
 * them has to leave with the tap:
 *
 * - `quote`: the block's text *now*, because that is the only moment it is
 *   certainly the text the remark is about; the block goes on being edited and
 *   the annotation writes nothing into the document.
 * - `blocks`: every block id in reading order, so the host can tell a thread
 *   anchored to a live block from one whose block has been deleted, and lay the
 *   anchored ones out in document order. It travels with the tap rather than
 *   being read off the app's own copy of the body: that copy is the last *saved*
 *   version, so a paragraph typed a second ago would not be in it.
 *
 * @param {object} json ProseMirror document JSON
 * @param {string} blockId the block that was tapped
 * @returns {{block_id: string, quote: string, blocks: Array<string>}}
 */
export function annotatePayload(json, blockId) {
  const id = str(blockId)
  const node = id ? blockNodeById(json, id) : null
  return {
    block_id: id,
    // A tap that finds no node means the block is already gone — the thread is
    // detached, and an empty quote is the honest answer rather than a stale one.
    quote: node ? quoteFromBlock(node) : '',
    blocks: blockIdsInOrder(json),
  }
}

/**
 * Reads the payload of an office import the host is handing over (#2894 §8).
 *
 * The phone uploads the file itself — the endpoint creates the document, which
 * is the app's business — but the HTML that comes back is parsed *here*, by the
 * editor's own schema, and saved through its ordinary autosave. The alternative
 * was a second HTML→blocks walk in Kotlin, which is exactly the drift #2755
 * spent a task undoing on the one converter we already had.
 *
 * Everything is validated because it crosses as a string: a payload that is not
 * an object, or carries no HTML, must leave the open document alone rather than
 * blank it.
 *
 * A PDF comes back from the same endpoint as a *stored file* rather than as
 * HTML (it is never converted), so it is carried here too — one handoff for
 * every server-side import, as on the web.
 *
 * @param {string|object} raw the JSON the host passed
 * @returns {{html: string, page: object|null, pdf: object|null}|null} null when
 *   there is nothing to apply
 */
export function parseImportPayload(raw) {
  let parsed = raw
  if (typeof raw === 'string') {
    try {
      parsed = JSON.parse(raw)
    } catch {
      return null
    }
  }
  if (!parsed || typeof parsed !== 'object') return null
  const pdf =
    parsed.pdf && typeof parsed.pdf === 'object' && str(parsed.pdf.src) ? parsed.pdf : null
  const html = str(parsed.html)
  if (!html && !pdf) return null
  // The geometry is optional (a .doc or a .txt has none the server could read)
  // and is passed on untouched — withImportedPage is the side that knows what a
  // valid page setup looks like.
  const page = parsed.page && typeof parsed.page === 'object' ? parsed.page : null
  return { html, page, pdf }
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
