/**
 * The rules the two upload paths of a task run on (#2932).
 *
 * They are the server's, restated: `handlers/attachments.go` refuses an
 * attachment over 25 MiB (`maxAttachmentBytes`) and an inline image over 8 MiB
 * (`maxMediaBytes`) or outside the PNG/JPEG/GIF/WebP/BMP sniff allowlist
 * (`mediaExts`). Neither picker had a rule of its own, so a 40 MB file travelled
 * the wire in full before the refusal came back — and came back as the handler's
 * hardcoded *Russian* sentence ("файл больше 25 МБ"), which `utils/errors.js`
 * knows no sentinel for and passes through verbatim into an English UI. Saying it
 * here says it before the upload, in the reader's own language and units.
 *
 * Kept out of the components because a rule that is only ever true inside one is
 * a rule nobody can pin with a test — same reason as `utils/avatar.js`.
 */

/** The server's `maxAttachmentBytes`. */
export const ATTACHMENT_MAX_BYTES = 25 * 1024 * 1024

/** The server's `maxMediaBytes`, for images embedded in Markdown. */
export const MEDIA_MAX_BYTES = 8 * 1024 * 1024

/** The types the server's `mediaExts` sniff allowlist accepts, in its order. */
export const MEDIA_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/bmp']

/** Whether a file is over a cap the server compares the same bytes against. */
function overCap(file, cap) {
  return Number(file.size) > cap
}

/**
 * Why the server would refuse this as a task attachment, decided before it is
 * sent. Attachments are arbitrary files — the handler stores whatever arrives —
 * so size is the only rule there is.
 *
 * @param {File|null|undefined} file
 * @returns {''|'tooLarge'} a reason code, or '' if nothing is wrong
 */
export function attachmentPickError(file) {
  if (!file) return ''
  return overCap(file, ATTACHMENT_MAX_BYTES) ? 'tooLarge' : ''
}

/**
 * Why the server would refuse this as an inline image, decided before it is sent.
 *
 * Size is checked first because the handler does, so a 12 MB SVG reads as "too
 * large" on both sides rather than inviting a smaller SVG that would be refused
 * anyway. The type check is deliberately one-sided: it only fires on a type the
 * browser actually reported and the server certainly rejects — SVG above all,
 * which is absent from the allowlist on purpose (it is a script-bearing document
 * and /uploads is served on the app's own origin). A file the browser types as ''
 * is handed over untouched, because the server sniffs the *bytes*; refusing it
 * here would make the picker stricter than the server.
 *
 * @param {File|null|undefined} file
 * @returns {''|'tooLarge'|'badType'} a reason code, or '' if nothing is wrong
 */
export function mediaPickError(file) {
  if (!file) return ''
  if (overCap(file, MEDIA_MAX_BYTES)) return 'tooLarge'
  const type = String(file.type || '').toLowerCase()
  if (type && !MEDIA_TYPES.includes(type)) return 'badType'
  return ''
}
