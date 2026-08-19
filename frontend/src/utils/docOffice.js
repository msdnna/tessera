import { htmlToDoc } from './docImport'
import { normalizeOfficeHtml } from './docOfficeHtml'
import { PDF_EXTENSION, isPdfFile, pdfDocument } from './docPdf'

// Office import/export (#2733). The conversion itself happens in a LibreOffice
// sidecar the backend talks to; this module owns the two halves that belong in
// the browser.
//
// Import deliberately finishes here rather than on the server. The endpoint
// returns HTML, and it is parsed with the editor's own TipTap schema
// (htmlToDoc) — the same path a pasted document and an uploaded .md template
// already take. Parsing server-side would mean a second HTML→blocks walk in Go
// that drifts from this one, and the schema *is* the allow-list, so anything it
// cannot represent is dropped at parse time instead of coming back as a 400 the
// user cannot act on.

// Extensions the import route accepts. Kept in step with docImportExts in
// backend/handlers/document_convert.go — the picker offering a format the
// server refuses is a worse failure than not offering it, because the user only
// finds out after choosing a file.
export const OFFICE_EXTENSIONS = ['.doc', '.docx', '.odt', '.rtf', '.fodt', '.html', '.htm', '.txt']

// Matches maxDocImportBytes server-side. Checked here too so an oversized file
// is refused before it is uploaded rather than after.
export const MAX_OFFICE_BYTES = 20 * 1024 * 1024

export const EXPORT_LABELS = {
  pdf: 'PDF',
  docx: 'Word (.docx)',
  odt: 'OpenDocument (.odt)',
  html: 'HTML',
}

/**
 * Whether a picked file goes to the server's import route.
 *
 * PDF is included even though it never reaches the converter: it travels the
 * same endpoint and comes back as a stored file rather than as HTML (see
 * docPdf.js). Keeping it in this predicate is what makes one picker and one
 * upload path cover every server-side import.
 *
 * @param {string} name file name
 * @returns {boolean}
 */
export function isOfficeFile(name) {
  const n = String(name || '').toLowerCase()
  return isPdfFile(n) || OFFICE_EXTENSIONS.some((ext) => n.endsWith(ext))
}

/**
 * True when the file needs the sidecar. PDF and the browser-side formats do
 * not, so the import button stays usable on an install without one.
 * @param {string} name file name
 */
export function needsConverter(name) {
  return isOfficeFile(name) && !isPdfFile(name)
}

/**
 * The `accept` attribute for the import file input: office formats, PDF, and
 * the two the browser handles on its own (D9), so one picker covers every
 * import.
 * @param {string[]} [extra]
 * @returns {string}
 */
export function importAccept(extra = ['.md', '.markdown', '.json']) {
  return [...OFFICE_EXTENSIONS, PDF_EXTENSION, ...extra].join(',')
}

/**
 * Uploads an office file and saves the converted body onto the document the
 * server created for it.
 *
 * The two steps are not merged: the server creates the document (it owns slugs,
 * positions and the workspace check) and hands back HTML, and the body is then
 * saved through the ordinary content endpoint — so an import is validated by
 * exactly the same code as typing, rather than by a second path that has to be
 * kept as strict.
 *
 * If the save fails the document survives as an empty one; that is deliberate,
 * because the alternative is deleting something the user can see was created
 * and losing the only trace of the attempt.
 *
 * @param {object} api the documents API slice (importFile, updateContent)
 * @param {string} wsId workspace id
 * @param {File} file picked file
 * @param {{parentId?: string}} [opts]
 * @returns {Promise<{document: object, imagesDropped: number, imagesDroppedReason: string}>}
 */
export async function importOfficeFile(api, wsId, file, opts = {}) {
  if (!file) throw new Error('Файл не выбран')
  if (!isOfficeFile(file.name)) {
    throw new Error(`Поддерживаются ${OFFICE_EXTENSIONS.join(', ')}`)
  }
  if (file.size > MAX_OFFICE_BYTES) {
    throw new Error('Файл больше 20 МБ')
  }
  const form = new FormData()
  form.append('file', file)
  if (opts.parentId) form.append('parent_id', opts.parentId)

  const res = await api.importFile(wsId, form)
  const doc = res.data?.document
  if (!doc?.id) throw new Error('Сервер не вернул документ')

  // A PDF comes back as a stored file rather than as HTML — there is nothing to
  // parse, and the body is the single block that points at it.
  //
  // Everything else goes through normalizeOfficeHtml first: the sidecar speaks
  // legacy HTML (<font>, <center>, class rules in a <style> block) that the
  // schema cannot see, so without that step the colours, sizes, rules and code
  // blocks of the source document are dropped at parse time (#2755).
  const content = res.data?.pdf
    ? pdfDocument(res.data.pdf)
    : htmlToDoc(normalizeOfficeHtml(res.data.html || ''))
  const saved = await api.updateContent(doc.id, content, doc.updated_at)
  return {
    document: saved?.data?.updated_at ? { ...doc, updated_at: saved.data.updated_at } : doc,
    imagesDropped: Number(res.data.images_dropped) || 0,
    // Why they were dropped, phrased by the server (it is the side that knows
    // whether the bytes were an unsupported format or a ceiling was hit). An
    // older backend does not send it, so the caller must treat it as optional.
    imagesDroppedReason: String(res.data.images_dropped_reason || ''),
  }
}

/**
 * File name for a downloaded export. Falsy or whitespace-only titles fall back
 * to a fixed name: a file called ".pdf" is invisible in a downloads folder.
 * @param {string} title
 * @param {string} format
 * @returns {string}
 */
export function exportFileName(title, format) {
  const base = String(title || '').trim() || 'Документ'
  // Only the characters that break a file name on some platform are replaced —
  // Cyrillic and spaces are fine and stripping them would mangle every title.
  return base.replace(/[\\/:*?"<>|]/g, '_') + '.' + format
}

/**
 * Hands a downloaded blob to the browser.
 *
 * The object URL is revoked afterwards; without that every export leaks the
 * whole file for the lifetime of the tab, which for a session of PDF exports is
 * exactly the kind of leak nobody attributes to the export button.
 *
 * @param {Blob} blob
 * @param {string} filename
 */
export function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
