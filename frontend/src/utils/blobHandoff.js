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

// BLOB_REVOKE_DELAY_MS is how long the browser gets to pick the bytes up. One
// minute is the conservative end of what the app already used (10s in the chat,
// 60s for recordings) — the cost of waiting is one blob held a little longer,
// the cost of being early is a download that silently never happens.
export const BLOB_REVOKE_DELAY_MS = 60_000

// handOffBlob triggers a file save for `blob` under `filename`.
export function handOffBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), BLOB_REVOKE_DELAY_MS)
}
