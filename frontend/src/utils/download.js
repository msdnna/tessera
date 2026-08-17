import { tasks as tasksApi } from '@/api'
import { isTauri } from '@/utils/serverBase'

// saveAttachment pulls a task attachment down and hands it to the user.
//
// It can't be a plain <a href> anywhere: GET /attachments/:id/download sits
// behind the protected route group and the access token lives in memory only, so
// the request has to go through the api client. That's also why attachment links
// inside descriptions and comments are intercepted rather than followed.
//
// Returns 'saved' (desktop, written to a path the user picked), 'downloaded'
// (web, handed to the browser — which reports it itself, so callers stay quiet)
// or 'cancelled'. Throws on a failed request — callers surface the message.
export async function saveAttachment(id, filename) {
  const res = await tasksApi.downloadAttachment(id)
  // Desktop: a real "Save as…" dialog + write to disk (the webview can't drive
  // an <a download> file save). Web keeps the anchor-download path.
  if (isTauri()) {
    const { save } = await import('@tauri-apps/plugin-dialog')
    const { writeFile } = await import('@tauri-apps/plugin-fs')
    const path = await save({ defaultPath: filename })
    if (!path) return 'cancelled'
    await writeFile(path, new Uint8Array(await res.data.arrayBuffer()))
    return 'saved'
  }
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
  return 'downloaded'
}

// ATTACHMENT_HREF_RE matches the links the editor inserts for task files. The
// origin prefix is optional: under Tauri the sanitiser rewrites root-relative
// '/api/…' URLs to the configured server origin before they reach the DOM.
export const ATTACHMENT_HREF_RE = /\/api\/attachments\/([0-9a-fA-F-]{36})\/download/

// attachmentIdFromHref returns the attachment id an inserted link points at, or
// null when the href is an ordinary link.
export function attachmentIdFromHref(href) {
  const m = ATTACHMENT_HREF_RE.exec(String(href || ''))
  return m ? m[1] : null
}
