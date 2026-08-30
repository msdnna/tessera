import { describe, it, expect } from 'vitest'
import { Editor, getSchema } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { withImportedPage } from '@/utils/docOffice'
import {
  DEFAULT_PAGE,
  GUTTER_LANE_PX,
  MARGIN_PRESETS,
  PAGE_SIZES,
  contentWidthMM,
  isLandscape,
  isPageSetup,
  marginKey,
  mmToPx,
  normalizePage,
  pageOf,
  pageStyleVars,
  samePage,
  sizeKey,
  withMargins,
  withOrientation,
  withSize,
} from '@/utils/docPage'

const A4 = { w: 210, h: 297, ml: 20, mr: 20, mt: 20, mb: 20 }
const A4_LANDSCAPE = { w: 297, h: 210, ml: 20, mr: 20, mt: 20, mb: 20 }

describe('page geometry', () => {
  // isPageSetup mirrors checkDocPage in document_schema.go. Drift between the
  // two is not a cosmetic problem: the editor would offer to save a geometry
  // the server refuses, and the user would meet it as a 400 on an ordinary
  // save, with nothing on screen pointing at the page dialog.
  it('accepts a geometry the server would accept', () => {
    expect(isPageSetup(A4)).toBe(true)
    expect(isPageSetup(A4_LANDSCAPE)).toBe(true)
    // Imported pages are not round: twips do not divide evenly into millimetres.
    expect(isPageSetup({ w: 209.9, h: 296.9, ml: 15, mr: 15, mt: 19.8, mb: 19.8 })).toBe(true)
  })

  it('refuses everything that is not one', () => {
    const bad = {
      null: null,
      undefined: undefined,
      string: 'A4',
      array: [210, 297],
      'missing a key': { w: 210, h: 297, ml: 20, mr: 20, mt: 20 },
      'extra key': { ...A4, orient: 'landscape' },
      'string side': { ...A4, w: '210mm' },
      NaN: { ...A4, w: NaN },
      Infinity: { ...A4, h: Infinity },
      'too small': { ...A4, w: 10 },
      'too large': { ...A4, h: 99999 },
      'negative margin': { ...A4, ml: -20 },
      'margins meet sideways': { ...A4, ml: 110, mr: 110 },
      'margins meet vertically': { ...A4, mt: 150, mb: 150 },
    }
    for (const [name, value] of Object.entries(bad)) {
      expect(isPageSetup(value), name).toBe(false)
    }
  })

  // ProseMirror serialises an unset attribute rather than omitting it, so
  // `page: null` is what every document that has never been through the dialog
  // carries — the common case, not an edge one.
  it('falls back to A4 for a document without a geometry', () => {
    expect(normalizePage(null)).toEqual(DEFAULT_PAGE)
    expect(normalizePage(undefined)).toEqual(DEFAULT_PAGE)
    expect(normalizePage({ w: 1, h: 1, ml: 0, mr: 0, mt: 0, mb: 0 })).toEqual(DEFAULT_PAGE)
    expect(pageOf({ type: 'doc', content: [] })).toEqual(DEFAULT_PAGE)
    expect(pageOf({ type: 'doc', attrs: { page: A4_LANDSCAPE } })).toEqual(A4_LANDSCAPE)
  })

  it('returns a copy, so a caller cannot edit the shared default', () => {
    const first = normalizePage(null)
    first.w = 999
    expect(normalizePage(null).w).toBe(DEFAULT_PAGE.w)
  })

  it('turns the sheet over without moving the margins off their edges', () => {
    const turned = withOrientation(A4, true)
    expect(turned).toEqual(A4_LANDSCAPE)
    expect(isLandscape(turned)).toBe(true)
    // Turning it the way it already faces is a no-op, not a second flip.
    expect(withOrientation(turned, true)).toEqual(A4_LANDSCAPE)
    expect(withOrientation(turned, false)).toEqual(A4)
  })

  // A size is a sheet, not a sheet-in-an-orientation: picking "A5" on a
  // landscape document must give a landscape A5, or the size buttons would
  // silently undo the orientation button.
  it('keeps the orientation when the size changes', () => {
    const a5 = PAGE_SIZES.find((s) => s.key === 'a5')
    const next = withSize(A4_LANDSCAPE, a5)
    expect(isLandscape(next)).toBe(true)
    expect(next).toEqual({ ...A4_LANDSCAPE, w: 210, h: 148 })
    expect(sizeKey(next)).toBe('a5')
    expect(sizeKey(withSize(A4, a5))).toBe('a5')
  })

  it('names the size and the margin preset in either orientation', () => {
    expect(sizeKey(A4)).toBe('a4')
    expect(sizeKey(A4_LANDSCAPE)).toBe('a4')
    // Rounding slack, because an imported A4 arrives as 209.9 × 296.9.
    expect(sizeKey({ ...A4, w: 209.9, h: 296.9 })).toBe('a4')
    expect(sizeKey({ ...A4, w: 200, h: 300 })).toBe('')
    const normal = MARGIN_PRESETS.find((m) => m.key === 'normal')
    expect(marginKey(withMargins(A4, normal))).toBe('normal')
    expect(marginKey({ ...A4, ml: 7 })).toBe('')
  })

  it('measures the printable column', () => {
    expect(contentWidthMM(A4)).toBe(170)
    expect(contentWidthMM(A4_LANDSCAPE)).toBe(257)
  })

  it('compares two geometries by value', () => {
    expect(samePage(A4, { ...A4 })).toBe(true)
    expect(samePage(A4, A4_LANDSCAPE)).toBe(false)
    expect(samePage(A4, null)).toBe(false)
    expect(samePage(null, null)).toBe(true)
  })
})

describe('sheet sizing', () => {
  it('converts millimetres at 96 dpi', () => {
    expect(mmToPx(25.4)).toBeCloseTo(96, 9)
    expect(Math.round(mmToPx(210))).toBe(794)
  })

  // The bug in one assertion: a table imported at its Word width (933px, laid
  // out for a landscape page) has to fit the column the sheet gives it. On the
  // portrait sheet it does not, which is what the screenshot in the task shows.
  it('gives a landscape sheet the column its content was laid out for', () => {
    const portrait = pageStyleVars(A4)
    const landscape = pageStyleVars(A4_LANDSCAPE)
    const column = (vars) =>
      parseFloat(vars['--doc-sheet-w']) -
      parseFloat(vars['--doc-sheet-pl']) -
      parseFloat(vars['--doc-sheet-pr'])

    expect(column(portrait)).toBeLessThan(933)
    expect(column(landscape)).toBeGreaterThan(933)
  })

  // The drag handle's lane is UI and not a document margin, so it may widen the
  // padding — but it must not eat printable width, or a narrow-margin document
  // would quietly lose a centimetre of column to a toolbar affordance.
  it('adds the drag-handle lane to the sheet instead of taking it from the column', () => {
    const narrow = { ...A4, ml: 13 }
    const vars = pageStyleVars(narrow)
    expect(parseFloat(vars['--doc-sheet-pl'])).toBe(GUTTER_LANE_PX)

    const column =
      parseFloat(vars['--doc-sheet-w']) -
      parseFloat(vars['--doc-sheet-pl']) -
      parseFloat(vars['--doc-sheet-pr'])
    expect(column).toBeCloseTo(mmToPx(narrow.w - narrow.ml - narrow.mr), 1)
  })

  it('uses the document margin when it is wider than the lane', () => {
    const wide = { ...A4, ml: 50 }
    const vars = pageStyleVars(wide)
    expect(parseFloat(vars['--doc-sheet-pl'])).toBeCloseTo(mmToPx(50), 1)
    expect(parseFloat(vars['--doc-sheet-w'])).toBeCloseTo(mmToPx(210), 1)
  })

  it('emits every property the sheet rule reads', () => {
    expect(Object.keys(pageStyleVars(A4)).sort()).toEqual([
      '--doc-sheet-pb',
      '--doc-sheet-pl',
      '--doc-sheet-pr',
      '--doc-sheet-pt',
      '--doc-sheet-w',
    ])
  })
})

describe('the page attribute in the editor', () => {
  it('is declared on the doc node and nowhere else', () => {
    const schema = getSchema(docExtensions())
    expect(schema.nodes.doc.spec.attrs).toHaveProperty('page')
    expect(schema.nodes.paragraph.spec.attrs).not.toHaveProperty('page')
  })

  it('round-trips through setDocPage and getJSON', () => {
    const editor = new Editor({
      extensions: docExtensions(),
      content: { type: 'doc', content: [{ type: 'paragraph' }] },
    })
    try {
      expect(editor.getJSON().attrs.page).toBe(null)

      editor.commands.setDocPage(A4_LANDSCAPE)
      expect(editor.getJSON().attrs.page).toEqual(A4_LANDSCAPE)
      expect(pageOf(editor.getJSON())).toEqual(A4_LANDSCAPE)

      // Rubbish clears the attribute rather than being stored: the server would
      // refuse it on save, and a document that cannot be saved is a worse
      // outcome than one that fell back to A4.
      editor.commands.setDocPage({ w: 'wide' })
      expect(editor.getJSON().attrs.page).toBe(null)
    } finally {
      editor.destroy()
    }
  })
})

describe('imported page geometry', () => {
  // The geometry arrives beside the HTML rather than inside it — LibreOffice
  // keeps only the first section's @page, which is exactly why the server reads
  // it from the source bytes instead.
  it('lands on the doc node', () => {
    const doc = withImportedPage({ type: 'doc', content: [] }, A4_LANDSCAPE)
    expect(doc.attrs.page).toEqual(A4_LANDSCAPE)
    expect(pageOf(doc)).toEqual(A4_LANDSCAPE)
  })

  it('leaves the document alone when the server sent nothing usable', () => {
    const body = { type: 'doc', content: [{ type: 'paragraph' }] }
    // .doc, .rtf and .txt carry no geometry, and an older backend sends no field
    // at all — all three arrive here as a falsy value.
    for (const page of [null, undefined, {}, 'A4', { w: 210 }]) {
      expect(withImportedPage(body, page)).toBe(body)
    }
  })

  it('does not mutate the parsed document or the server payload', () => {
    const body = { type: 'doc', content: [] }
    const page = { ...A4_LANDSCAPE }
    const out = withImportedPage(body, page)
    out.attrs.page.w = 999
    expect(page.w).toBe(297)
    expect(body.attrs).toBeUndefined()
  })
})
