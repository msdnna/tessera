// PDF inside a document (#2733, D8 of #2718 — item 1 of the parent asks for
// "PDF (чтение)").
//
// Unlike every other import format, a PDF is not converted into blocks: run one
// through LibreOffice and you get a page of absolutely positioned text frames,
// which has no paragraphs, no headings and no reading order. What survives is
// worse for annotations, version diffs and export than simply keeping the file.
// So the file is stored as a document asset and the body holds one `pdfEmbed`
// block pointing at it, rendered here by pdf.js.
//
// Everything in this module is deliberately free of pdf.js itself: the library
// is ~1 MB and only DocPdf.vue (a lazy node view) touches it. That keeps the
// arithmetic below unit-testable without a canvas, and keeps the documents route
// from paying for pdf.js on every open.

export const PDF_EXTENSION = '.pdf'
export const PDF_MIME = 'application/pdf'

// Matches maxDocImportBytes on the server. Stated here too so the picker can
// refuse a 40 MB scan before uploading it rather than after.
export const MAX_PDF_BYTES = 20 * 1024 * 1024

// Rendering every page of a long PDF at once locks the tab: each page is a
// canvas the size of the viewport, and a 300-page scan is several GB of bitmap.
// Pages are rendered in a window around the one being read.
export const PDF_WINDOW = 3

/**
 * True when a file name looks like a PDF.
 * @param {string} name
 */
export function isPdfFile(name) {
  return String(name || '')
    .toLowerCase()
    .endsWith(PDF_EXTENSION)
}

/**
 * The block a stored PDF becomes.
 *
 * `size` is kept alongside the URL so the viewer can say how big the file is
 * before it has fetched a byte of it — on a slow link that is the difference
 * between "broken" and "loading a 12 MB scan".
 *
 * @param {{src: string, name?: string, size?: number}} pdf descriptor from the API
 * @returns {object} ProseMirror node JSON
 */
export function pdfBlockNode(pdf) {
  const src = String(pdf?.src || '')
  if (!src) throw new Error('Сервер не вернул ссылку на файл')
  return {
    type: 'pdfEmbed',
    attrs: {
      src,
      name: String(pdf?.name || 'документ.pdf'),
      size: Number(pdf?.size) || 0,
    },
  }
}

/**
 * A whole document whose body is one PDF — what an imported PDF becomes.
 * @param {{src: string, name?: string, size?: number}} pdf
 */
export function pdfDocument(pdf) {
  return { type: 'doc', content: [pdfBlockNode(pdf)] }
}

/**
 * Human-readable byte count. Russian abbreviations, because the whole section
 * is in Russian and "12.4 MB" next to "12,4 МБ" elsewhere reads as a bug.
 * @param {number} bytes
 */
export function formatFileSize(bytes) {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n <= 0) return ''
  const units = ['Б', 'КБ', 'МБ', 'ГБ']
  let value = n
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  // Whole bytes never want a decimal point; anything scaled reads better with
  // one significant fraction digit.
  const text = unit === 0 ? String(Math.round(value)) : value.toFixed(1).replace(/\.0$/, '')
  return `${text.replace('.', ',')} ${units[unit]}`
}

/**
 * Keeps a page number inside the document.
 *
 * Returns 1 for an empty or unknown document rather than 0: the viewer shows
 * "стр. N из M", and page 0 of 0 is the kind of thing that gets shipped.
 *
 * @param {number} page requested page, 1-based
 * @param {number} total page count
 */
export function clampPage(page, total) {
  const count = Math.max(1, Math.floor(Number(total) || 1))
  const n = Math.floor(Number(page) || 1)
  if (n < 1) return 1
  if (n > count) return count
  return n
}

/**
 * The scale that fits a page to the available width.
 *
 * pdf.js reports a page's size at scale 1 in CSS pixels, so this is a plain
 * ratio — but it is clamped at both ends: an unbounded scale turns a business
 * card sized page into a blurry wall on a wide monitor, and a container that
 * has not been laid out yet reports width 0, which would render nothing at all
 * and look like a failed load.
 *
 * @param {number} pageWidth page width at scale 1
 * @param {number} containerWidth available width in CSS pixels
 * @param {{min?: number, max?: number}} [limits]
 */
export function fitScale(pageWidth, containerWidth, limits = {}) {
  const min = limits.min ?? 0.25
  const max = limits.max ?? 2
  const w = Number(pageWidth)
  const avail = Number(containerWidth)
  if (!Number.isFinite(w) || w <= 0 || !Number.isFinite(avail) || avail <= 0) return 1
  return Math.min(max, Math.max(min, avail / w))
}

/**
 * The pages worth rendering around the one being read.
 * @param {number} page current page, 1-based
 * @param {number} total page count
 * @param {number} [window] pages to keep on each side
 * @returns {number[]} ascending page numbers
 */
export function pagesAround(page, total, window = PDF_WINDOW) {
  const count = Math.max(0, Math.floor(Number(total) || 0))
  if (!count) return []
  const current = clampPage(page, count)
  const from = Math.max(1, current - window)
  const to = Math.min(count, current + window)
  const out = []
  for (let i = from; i <= to; i += 1) out.push(i)
  return out
}

/**
 * The PDF blocks in a document body, in reading order.
 * @param {object} json ProseMirror document JSON
 * @returns {Array<{src: string, name: string, size: number, id: string|null}>}
 */
export function pdfBlocksIn(json) {
  const out = []
  const walk = (node) => {
    if (!node || typeof node !== 'object') return
    if (node.type === 'pdfEmbed') {
      out.push({
        src: node.attrs?.src || '',
        name: node.attrs?.name || '',
        size: Number(node.attrs?.size) || 0,
        id: node.attrs?.id || null,
      })
    }
    if (Array.isArray(node.content)) node.content.forEach(walk)
  }
  walk(json)
  return out
}
