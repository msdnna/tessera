// Handing a blob the app downloaded over to the browser as a file save.
//
// Every download in Tessera arrives as bytes rather than as a link: the routes
// sit behind the protected group and the access token lives in memory only, so
// an `<a href>` straight at the API would carry no Authorization header. That
// leaves the object-URL dance — create, click, revoke — and the revoke is where
// it goes wrong.
//
// `a.click()` only *starts* the download; the browser reads the blob out of the
// URL afterwards. Revoking on the next line therefore races the transfer and
// can cancel it outright, and the bigger the file the wider the gap: a task
// attachment runs to 25 MiB (backend maxAttachmentBytes) and a recording to
// far more. ConferenceRecordings found this first and deferred its revoke;
// this module is that fix as one function, so the next download path gets it
// for free instead of rediscovering the race.
//
// Deferred, not dropped: an object URL that is never revoked pins the whole
// file in memory for the lifetime of the tab.
//
// …and the whole dance is a *browser* mechanism. The desktop shell has no
// download handler at all — wry only registers one when the app supplies it
// (webkitgtk/mod.rs), and desktop/src-tauri never does — so under Tauri
// `a.click()` starts nothing and the file silently never appears. That is why
// saveAttachment grew a Save-as path of its own (#2717), and why
// desktop/src-tauri/capabilities/default.json grants fs:allow-write-file for
// exactly this. Making it a branch here rather than leaving it in one caller is
// the point of the module: the chat attachment, the recording and the document
// export all reached the desktop through the browser path and none of them
// worked there.

import { isTauri } from '@/utils/serverBase'

// BLOB_REVOKE_DELAY_MS is how long the browser gets to pick the bytes up. One
// minute is the conservative end of what the app already used (10s in the chat,
// 60s for recordings) — the cost of waiting is one blob held a little longer,
// the cost of being early is a download that silently never happens.
export const BLOB_REVOKE_DELAY_MS = 60_000

/**
 * Triggers a file save for `blob` under `filename`.
 *
 * Returns 'saved' (desktop, written to a path the user picked), 'downloaded'
 * (web, handed to the browser — which reports it itself, so callers stay quiet)
 * or 'cancelled'. Throws if the desktop write fails; the web path cannot.
 *
 * @param {Blob} blob
 * @param {string} filename
 * @returns {Promise<'saved'|'downloaded'|'cancelled'>}
 */
export async function handOffBlob(blob, filename) {
  if (isTauri()) {
    const { save } = await import('@tauri-apps/plugin-dialog')
    const { writeFile } = await import('@tauri-apps/plugin-fs')
    const path = await save({ defaultPath: filename })
    if (!path) return 'cancelled'
    await writeFile(path, new Uint8Array(await blob.arrayBuffer()))
    return 'saved'
  }
  // No await above this line on the web path: the click has to happen in the
  // same tick as the caller's gesture, or a popup blocker treats it as one the
  // user did not make.
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), BLOB_REVOKE_DELAY_MS)
  return 'downloaded'
}
