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
import { i18n } from '@/i18n'
import { formatFileSize } from '@/utils/docPdf'

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

/**
 * The same refusal, already phrased — '' when there is nothing to refuse.
 *
 * The reason code is the rule; this is the sentence, and it lives here so the
 * three ways an image reaches a page (picker, drop, paste) and the two editors
 * that offer them cannot drift apart in wording or in the units they quote.
 * Both servers answer with a fixed Russian line and no number at all, which is
 * the thing being replaced: `handlers/document_assets.go` says "файл слишком
 * большой" whatever language the reader chose.
 *
 * The lookup is per call, not at import time — same reason as errors.js (#2799):
 * a refusal shown after the user switched language must be in the new one.
 *
 * @param {File|null|undefined} file
 * @returns {string} the refusal to show, or '' if the file is fine
 */
export function mediaPickMessage(file) {
  const reason = mediaPickError(file)
  if (!reason) return ''
  if (reason === 'tooLarge') {
    return i18n.global.t('documents.editor.error.imageTooLarge', {
      limit: formatFileSize(MEDIA_MAX_BYTES),
    })
  }
  return i18n.global.t('documents.editor.error.imageType')
}

/**
 * How big an uploaded attachment is, for the row that lists it (#2932).
 *
 * The files tab used to write this by hand — `(bytes / 1024).toFixed(0)` and
 * `.toFixed(1)` — which is exactly half of the rule. It knew the unit is text
 * and took it from the catalogue, then glued the number on with a decimal
 * *point*, so the same file read "1.4 МБ" here and "1,4 МБ" in a document or a
 * call's chat, inside one Russian interface. The separator is locale business
 * for the same reason the unit is: `formatFileSize` (imported two lines up for
 * the refusal above) hands the number to `Intl.NumberFormat`, and so do the
 * phone's `docFileSize` / `confFileSize`.
 *
 * The scaled units keep one fraction digit, again per `formatFileSize`: the old
 * `toFixed(0)` on kilobytes rounded 14.6 KB to a flat "15 КБ" that disagreed
 * with every other place the same file is weighed. Whole bytes never want a
 * fraction.
 *
 * The unit stays a catalogue key rather than `formatFileSize`'s own, because
 * these rows have had their own (`task.files.size.*`) since before documents
 * existed and the two sets are worded independently.
 *
 * @param {number} bytes size as the server reported it
 * @returns {string} e.g. "14,6 КБ" · "1,4 МБ" · "512 Б"
 */
export function attachmentSizeText(bytes) {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n < 0) return ''
  const unit = n < 1024 ? 'b' : n < 1024 * 1024 ? 'kb' : 'mb'
  const value = unit === 'b' ? n : unit === 'kb' ? n / 1024 : n / 1024 / 1024
  const amount = new Intl.NumberFormat(i18n.global.locale.value, {
    maximumFractionDigits: unit === 'b' ? 0 : 1,
  }).format(value)
  return i18n.global.t(`task.files.size.${unit}`, { n: amount })
}
