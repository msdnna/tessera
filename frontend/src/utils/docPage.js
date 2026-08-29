// Page geometry for a block document (#2821).
//
// The document is not paginated and will not be: laying text out into pages is
// a layout engine (block heights, breaks inside tables, running headers), not a
// feature. What a document *does* have is one page geometry — sheet size,
// orientation and margins — and that turned out to matter for more than looks.
//
// An imported .docx whose landscape section holds a wide table came in as
// portrait, because LibreOffice keeps only the first section's geometry, and
// the table's column widths came in as the absolute pixels Word laid them out
// in. A 933px table on a 692px sheet is the bug the task was filed for. Giving
// the document its real geometry — and the sheet its real width — is what fixes
// it: the table fits because the sheet became wide, not because the table was
// squeezed.
//
// Millimetres are the unit throughout: it is what both source formats speak
// (once twips and CSS lengths are converted), what the exported @page rule
// takes, and the only one in which "A4" is a round number. Pixels appear at the
// last moment, in the editor, and nowhere else.

const MM_PER_INCH = 25.4
const CSS_PPI = 96

/** The six numbers a geometry consists of. Order is the storage order. */
export const PAGE_KEYS = ['w', 'h', 'ml', 'mr', 'mt', 'mb']

// Bounds shared with the server (office.MinSide/MaxSide in Go). A value outside
// them is not a page, it is a misparsed unit — an inch count read as
// millimetres, say — and it must not reach the exported @page rule.
export const MIN_SIDE = 50
export const MAX_SIDE = 2000

/**
 * The geometry of a document nobody has opened the page dialog on. A4 with
 * 20 mm margins — deliberately identical to defaultDocPage on the server, and
 * to what LibreOffice assumed on its own before the export wrote an @page rule
 * at all, so that re-exporting an untouched document produces what it always
 * did.
 */
export const DEFAULT_PAGE = Object.freeze({ w: 210, h: 297, ml: 20, mr: 20, mt: 20, mb: 20 })

/**
 * Sheet sizes offered in the toolbar, portrait side up. Orientation is not part
 * of a size — it is the same sheet turned over — so a landscape A4 is this
 * entry with `w` and `h` swapped, and `sizeKey` finds it either way round.
 */
export const PAGE_SIZES = [
  { key: 'a4', w: 210, h: 297 },
  { key: 'a5', w: 148, h: 210 },
  { key: 'a3', w: 297, h: 420 },
  { key: 'letter', w: 215.9, h: 279.4 },
  { key: 'legal', w: 215.9, h: 355.6 },
]

/**
 * Margin presets. The values are Word's, in millimetres — "обычные" is Word's
 * 2.54 cm all round rounded to the millimetre the UI shows, and "узкие" is its
 * 1.27 cm — so that a document that came from Word and one set up here look the
 * same rather than nearly the same.
 */
export const MARGIN_PRESETS = [
  { key: 'normal', ml: 25, mr: 25, mt: 25, mb: 25 },
  { key: 'narrow', ml: 13, mr: 13, mt: 13, mb: 13 },
  { key: 'moderate', ml: 19, mr: 19, mt: 25, mb: 25 },
  { key: 'wide', ml: 50, mr: 50, mt: 25, mb: 25 },
]

/** Millimetres to CSS pixels at the 96 dpi the CSS unit system is defined in. */
export function mmToPx(mm) {
  return (Number(mm) * CSS_PPI) / MM_PER_INCH
}

/**
 * Whether a value is a usable page geometry.
 *
 * The same rules the Go validator enforces (checkDocPage), and for the same
 * reason: this object is exported into an @page rule, so "roughly a geometry"
 * is not good enough. Kept in one predicate rather than spread over the callers
 * so that the editor never offers to save something the server will refuse.
 *
 * @param {*} page candidate geometry
 * @returns {boolean}
 */
export function isPageSetup(page) {
  if (!page || typeof page !== 'object' || Array.isArray(page)) return false
  if (Object.keys(page).length !== PAGE_KEYS.length) return false
  for (const k of PAGE_KEYS) {
    const v = page[k]
    if (typeof v !== 'number' || !Number.isFinite(v)) return false
  }
  if (page.w < MIN_SIDE || page.w > MAX_SIDE || page.h < MIN_SIDE || page.h > MAX_SIDE) return false
  if (page.ml < 0 || page.mr < 0 || page.mt < 0 || page.mb < 0) return false
  // Margins that meet leave nothing to print in — a geometry the sheet cannot
  // draw and the export would turn into a blank page.
  return page.ml + page.mr < page.w && page.mt + page.mb < page.h
}

/**
 * The geometry to lay a document out with: its own if it has a usable one, the
 * default otherwise. Never returns null, so no caller has to decide what an
 * absent geometry means — and an absent one is the norm, since ProseMirror
 * serialises the unset attribute as `page: null` on every document that has not
 * been through the dialog.
 *
 * @param {*} page stored value of doc.attrs.page
 * @returns {{w:number,h:number,ml:number,mr:number,mt:number,mb:number}}
 */
export function normalizePage(page) {
  return isPageSetup(page) ? { ...page } : { ...DEFAULT_PAGE }
}

/** Reads the geometry off a document tree (ProseMirror JSON or a PM node). */
export function pageOf(doc) {
  return normalizePage(doc?.attrs?.page)
}

/** Two geometries are the same when all six numbers are. */
export function samePage(a, b) {
  if (a === b) return true
  if (!a || !b) return false
  return PAGE_KEYS.every((k) => a[k] === b[k])
}

/** Wider than tall. The one question the toolbar toggle and the import warning ask. */
export function isLandscape(page) {
  return page.w > page.h
}

/**
 * Turns the sheet over, leaving the margins on the edges they were on.
 *
 * Rotating the margins with the sheet is the other reasonable reading, and it
 * is the wrong one for this editor: the left margin is where the drag-handle
 * lane lives and where the reader's eye starts, and having it jump to the top
 * when a document is turned landscape reads as a bug rather than as a rotation.
 * Word does the same.
 */
export function withOrientation(page, landscape) {
  if (isLandscape(page) === landscape) return { ...page }
  return { ...page, w: page.h, h: page.w }
}

/** Applies a sheet size, keeping the current orientation and margins. */
export function withSize(page, size) {
  const next = { ...page, w: size.w, h: size.h }
  return isLandscape(page) ? withOrientation(next, true) : next
}

/** Applies a margin preset, keeping the sheet. */
export function withMargins(page, margins) {
  return { ...page, ml: margins.ml, mr: margins.mr, mt: margins.mt, mb: margins.mb }
}

/**
 * Which sheet size the geometry is, ignoring orientation; '' for a size that is
 * not one of the presets (an imported document is under no obligation to use
 * one). Compared with a tolerance because an imported A4 is 209.9 mm as often
 * as 210 — twips do not divide evenly into millimetres.
 */
export function sizeKey(page) {
  const [short, long] = page.w <= page.h ? [page.w, page.h] : [page.h, page.w]
  const near = (a, b) => Math.abs(a - b) < 0.5
  return PAGE_SIZES.find((s) => near(s.w, short) && near(s.h, long))?.key || ''
}

/** Which margin preset the geometry uses, '' when it uses none. */
export function marginKey(page) {
  const near = (a, b) => Math.abs(a - b) < 0.5
  return (
    MARGIN_PRESETS.find((m) => ['ml', 'mr', 'mt', 'mb'].every((k) => near(m[k], page[k])))?.key ||
    ''
  )
}

/** The printable column in millimetres — the sheet minus its side margins. */
export function contentWidthMM(page) {
  return page.w - page.ml - page.mr
}

// The drag handle rides in the sheet's left padding rather than floating in the
// work area beside it (DocEditor.vue, GUTTER_INSET). That lane is UI, not part
// of the document's margin, so a document with a narrow left margin must not
// lose it — and must not lose printable width to it either. Hence both halves
// of the rule below: the padding is the wider of the two, and the sheet grows by
// whatever the lane added, so that the content column stays exactly the
// document's own printable width. Without the second half a 13 mm margin would
// quietly shave 39px off the column, which for a table imported at its Word
// width is the difference between fitting and not.
export const GUTTER_LANE_PX = 88

const px = (v) => `${Math.round(v * 100) / 100}px`

/** The sheet's left padding: the document's margin, or the gutter lane if wider. */
function leftPadPx(page) {
  return Math.max(mmToPx(page.ml), GUTTER_LANE_PX)
}

/**
 * How wide the sheet is drawn — the paper plus whatever the gutter lane added to
 * its left padding, so that the content column keeps the document's own
 * printable width (see GUTTER_LANE_PX).
 */
export function sheetWidthPx(page) {
  return mmToPx(page.w) + (leftPadPx(page) - mmToPx(page.ml))
}

/**
 * CSS custom properties that size the sheet in the editor.
 *
 * Returned as a style object for the wrapper rather than written onto the
 * ProseMirror element: that element is created and owned by ProseMirror, and
 * anything set on it directly is one `setContent` away from being lost.
 *
 * @param {object} page geometry, already normalised
 * @returns {Record<string,string>} custom properties, px values
 */
export function pageStyleVars(page) {
  return {
    '--doc-sheet-w': px(sheetWidthPx(page)),
    '--doc-sheet-pl': px(leftPadPx(page)),
    '--doc-sheet-pr': px(mmToPx(page.mr)),
    '--doc-sheet-pt': px(mmToPx(page.mt)),
    '--doc-sheet-pb': px(mmToPx(page.mb)),
  }
}

// ---------------------------------------------------------------------------
// Sections (#2827)
//
// A document with a section break has more than one geometry: the doc node's
// own attribute covers everything up to the first break, and each break carries
// the geometry of what follows it. Section geometry lives here, next to the
// single-page one, because the two answer the same question — how wide is the
// column this block is laid out in — and the editor must not have two different
// ideas of that.

/** Node type of a section break. Named here so docPage has no import cycle. */
export const SECTION_BREAK = 'sectionBreak'

/**
 * The document's top-level blocks, as plain `{ type, attrs }` pairs.
 *
 * Takes either a ProseMirror node or the stored JSON, because both are handed
 * around here — the editor plugin has a node, the Vue component has the value it
 * was given as a prop, and both ask the same questions about sections. A node
 * carries a Fragment in `content`, so the array check tells the two apart.
 *
 * @param {*} doc document node or its JSON
 * @returns {Array<{type:string,attrs:object}>}
 */
export function topBlocks(doc) {
  if (!doc) return []
  if (Array.isArray(doc.content)) {
    return doc.content.map((n) => ({ type: n?.type, attrs: n?.attrs || {} }))
  }
  const out = []
  doc.forEach?.((node) => out.push({ type: node.type?.name, attrs: node.attrs || {} }))
  return out
}

/**
 * The geometry of every section, in document order.
 *
 * The first entry is always the doc node's own geometry (or the default), so a
 * document without a single break yields exactly one section and everything
 * below behaves as it did before #2827.
 *
 * @param {Array<{type?:string,attrs?:object}>} blocks top-level blocks
 * @param {*} docPage stored doc.attrs.page
 * @returns {Array<object>} normalised geometries, one per section
 */
export function sectionPages(blocks, docPage) {
  const pages = [normalizePage(docPage)]
  for (const b of blocks || []) {
    if (b?.type === SECTION_BREAK) pages.push(normalizePage(b.attrs?.page))
  }
  return pages
}

/**
 * Which section each top-level block belongs to.
 *
 * The break itself is counted as the first block of the section it opens — it
 * is where the new geometry starts, and drawing it at the old width would put a
 * "Альбомная, A4" caption on a portrait band.
 *
 * @param {Array<{type?:string}>} blocks top-level blocks
 * @returns {number[]} section index per block
 */
export function sectionIndexes(blocks) {
  let section = 0
  return (blocks || []).map((b) => {
    if (b?.type === SECTION_BREAK) section += 1
    return section
  })
}

/**
 * The widest of the given geometries — the one the sheet is sized to.
 *
 * The sheet is a single element (ProseMirror owns it), so its width has to
 * cover the widest band; narrower sections are drawn inside it, each with its
 * own paper edge.
 *
 * @param {Array<object>} pages normalised geometries
 * @returns {object} the widest one
 */
export function widestPage(pages) {
  let widest = pages?.[0] || { ...DEFAULT_PAGE }
  for (const p of pages || []) if (sheetWidthPx(p) > sheetWidthPx(widest)) widest = p
  return widest
}

/**
 * Inline style for one top-level block, laid out as a band of its section.
 *
 * The band is the block itself: ProseMirror has no decoration that wraps a
 * *range* of top-level nodes, so there is no element to be the section's sheet.
 * Each block therefore carries its own paper — its width is the section's sheet,
 * its horizontal padding the section's margins — and the bands stack into a
 * continuous page. That only works with the vertical spacing moved inside the
 * background (DocEditor.vue, `.sectioned`), because a margin between two blocks
 * would show the work area through the paper.
 *
 * @param {object} page the section's geometry, normalised
 * @param {boolean} [first] first block of its section — takes the top margin
 * @param {boolean} [last] last block of its section — takes the bottom margin
 * @returns {Record<string,string>} CSS declarations
 */
export function bandStyle(page, first = false, last = false) {
  const style = {
    width: px(sheetWidthPx(page)),
    'padding-left': px(leftPadPx(page)),
    'padding-right': px(mmToPx(page.mr)),
  }
  if (first) style['padding-top'] = px(mmToPx(page.mt))
  if (last) style['padding-bottom'] = px(mmToPx(page.mb))
  return style
}

/** A style object as a `style` attribute value — what a decoration takes. */
export function styleAttr(style) {
  return Object.entries(style)
    .map(([k, v]) => `${k}: ${v}`)
    .join('; ')
}
