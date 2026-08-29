import { describe, it, expect } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import {
  DEFAULT_PAGE,
  SECTION_BREAK,
  bandStyle,
  sectionIndexes,
  sectionPages,
  sheetWidthPx,
  styleAttr,
  topBlocks,
  widestPage,
} from '@/utils/docPage'
import {
  bandDecorations,
  hasSections,
  sectionAt,
  sectionCaption,
} from '@/utils/docExtensions/sectionBreak'
import { slashItems } from '@/utils/docSlash'

// Sections (#2827) are the second geometry a document can have, and the whole
// feature rests on one rule: the doc node covers everything up to the first
// break, each break covers what follows it. Everything below checks that rule
// from a different side, because getting it off by one is the failure mode that
// would silently reflow half a document.

const LANDSCAPE = { w: 297, h: 210, ml: 15, mr: 15, mt: 20, mb: 20 }
const A5 = { w: 148, h: 210, ml: 10, mr: 10, mt: 10, mb: 10 }

const para = (text) => ({ type: 'paragraph', content: [{ type: 'text', text }] })
const brk = (page) => ({ type: SECTION_BREAK, attrs: { page } })

function doc(...blocks) {
  return { type: 'doc', content: blocks }
}

function makeEditor(content) {
  return new Editor({
    element: document.createElement('div'),
    extensions: docExtensions(),
    content,
  })
}

describe('section geometry', () => {
  it('gives a document without a break exactly one section', () => {
    const body = doc(para('один'), para('два'))
    expect(sectionPages(topBlocks(body), null)).toEqual([DEFAULT_PAGE])
    expect(sectionIndexes(topBlocks(body))).toEqual([0, 0])
    expect(hasSections(body)).toBe(false)
  })

  it('reads the doc attribute as the first section and each break as the next', () => {
    const body = doc(para('портрет'), brk(LANDSCAPE), para('альбом'), brk(A5), para('малая'))
    const pages = sectionPages(topBlocks(body), { ...DEFAULT_PAGE, ml: 30 })
    expect(pages).toHaveLength(3)
    expect(pages[0].ml).toBe(30)
    expect(pages[1]).toEqual(LANDSCAPE)
    expect(pages[2]).toEqual(A5)
  })

  // The break belongs to the section it opens, not to the one it ends: it is
  // where the new geometry starts, and drawing it at the old width would put a
  // landscape caption on a portrait band.
  it('counts the break itself as the first block of the new section', () => {
    const body = doc(para('портрет'), brk(LANDSCAPE), para('альбом'))
    expect(sectionIndexes(topBlocks(body))).toEqual([0, 1, 1])
  })

  it('falls back to the default for a break with no usable geometry', () => {
    const body = doc(para('a'), brk(null), brk({ w: 5, h: 5, ml: 0, mr: 0, mt: 0, mb: 0 }))
    const pages = sectionPages(topBlocks(body), null)
    expect(pages[1]).toEqual(DEFAULT_PAGE)
    expect(pages[2]).toEqual(DEFAULT_PAGE)
  })

  it('reads the top-level blocks off a ProseMirror node as well as off JSON', () => {
    const editor = makeEditor(doc(para('портрет'), brk(LANDSCAPE), para('альбом')))
    expect(topBlocks(editor.state.doc).map((b) => b.type)).toEqual([
      'paragraph',
      SECTION_BREAK,
      'paragraph',
    ])
    expect(hasSections(editor.state.doc)).toBe(true)
    editor.destroy()
  })

  it('sizes the sheet to the widest section', () => {
    const pages = [DEFAULT_PAGE, LANDSCAPE, A5]
    expect(widestPage(pages)).toEqual(LANDSCAPE)
    expect(sheetWidthPx(LANDSCAPE)).toBeGreaterThan(sheetWidthPx(DEFAULT_PAGE))
  })
})

describe('section bands', () => {
  it('pads a band with its own margins and takes the sheet margins only at the ends', () => {
    const middle = bandStyle(LANDSCAPE)
    expect(middle['padding-top']).toBeUndefined()
    expect(middle['padding-bottom']).toBeUndefined()
    const first = bandStyle(LANDSCAPE, true, false)
    expect(first['padding-top']).toBe('75.59px')
    expect(first['padding-bottom']).toBeUndefined()
    const last = bandStyle(LANDSCAPE, false, true)
    expect(last['padding-bottom']).toBe('75.59px')
  })

  // The gutter lane is UI, not a margin: a narrow left margin must not lose the
  // drag handle, and the printable column must not lose the width the lane took.
  it('keeps the printable column when the gutter lane is wider than the margin', () => {
    const style = bandStyle(A5)
    const width = parseFloat(style.width)
    const left = parseFloat(style['padding-left'])
    const right = parseFloat(style['padding-right'])
    expect(left).toBe(88)
    // The column is the document's own printable width — 148 − 10 − 10 mm at
    // 96 dpi — even though the lane took 88px where the margin asked for 37.8.
    expect(width - left - right).toBeCloseTo((128 * 96) / 25.4, 1)
  })

  it('decorates every top-level block of a sectioned document and nothing else', () => {
    const plain = makeEditor(doc(para('один'), para('два')))
    expect(bandDecorations(plain.state.doc).find()).toHaveLength(0)
    plain.destroy()

    const editor = makeEditor(doc(para('портрет'), brk(LANDSCAPE), para('альбом')))
    const decos = bandDecorations(editor.state.doc).find()
    expect(decos).toHaveLength(3)
    const widths = decos.map((d) => parseFloat(d.type.attrs.style.match(/width: ([\d.]+)px/)[1]))
    expect(widths[0]).toBeCloseTo(sheetWidthPx(DEFAULT_PAGE), 1)
    // The break opens the landscape section, so it is already drawn at its width.
    expect(widths[1]).toBeCloseTo(sheetWidthPx(LANDSCAPE), 1)
    expect(widths[2]).toBe(widths[1])
    expect(widths[1]).toBeGreaterThan(widths[0])
    editor.destroy()
  })

  it('writes a style object out as a style attribute', () => {
    expect(styleAttr({ width: '10px', 'padding-left': '2px' })).toBe(
      'width: 10px; padding-left: 2px',
    )
  })
})

describe('section commands', () => {
  it('inserts a break after the current block, copying the current geometry', () => {
    const editor = makeEditor(doc(para('портрет')))
    editor.commands.setSectionPage(LANDSCAPE)
    editor.commands.setTextSelection(2)
    expect(editor.commands.insertSectionBreak()).toBe(true)

    const body = editor.getJSON()
    const blocks = topBlocks(body)
    expect(blocks.map((b) => b.type)).toEqual(['paragraph', SECTION_BREAK, 'paragraph'])
    // Copied, not reset to A4: the break is reached to change one thing, and
    // resetting the margins would undo work done on the page dialog.
    expect(blocks[1].attrs.page).toEqual(LANDSCAPE)
    editor.destroy()
  })

  it('writes the geometry of the section the caret is in', () => {
    const editor = makeEditor(doc(para('портрет'), brk({ ...DEFAULT_PAGE }), para('альбом')))
    // Caret in the second section.
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    expect(sectionAt(editor.state).index).toBe(1)
    editor.commands.setSectionPage(LANDSCAPE)

    const blocks = topBlocks(editor.getJSON())
    expect(blocks[1].attrs.page).toEqual(LANDSCAPE)
    // The document's own geometry is untouched — that is the first section.
    expect(editor.getJSON().attrs?.page ?? null).toBe(null)

    // …and with the caret back in the first section the same command writes the
    // doc attribute instead.
    editor.commands.setTextSelection(2)
    expect(sectionAt(editor.state).index).toBe(0)
    editor.commands.setSectionPage(A5)
    expect(editor.getJSON().attrs.page).toEqual(A5)
    expect(topBlocks(editor.getJSON())[1].attrs.page).toEqual(LANDSCAPE)
    editor.destroy()
  })

  it('reports the geometry of the section the caret is in', () => {
    const editor = makeEditor(doc(para('портрет'), brk(LANDSCAPE), para('альбом')))
    editor.commands.setTextSelection(2)
    expect(sectionAt(editor.state).page).toEqual(DEFAULT_PAGE)
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    expect(sectionAt(editor.state).page).toEqual(LANDSCAPE)
    editor.destroy()
  })

  it('round-trips the geometry through HTML, so a copied break keeps its section', () => {
    const editor = makeEditor(doc(para('портрет'), brk(LANDSCAPE), para('альбом')))
    const html = editor.getHTML()
    expect(html).toContain('data-section-break')

    const back = makeEditor(html)
    expect(topBlocks(back.getJSON())[1].attrs.page).toEqual(LANDSCAPE)
    back.destroy()
    editor.destroy()
  })

  it('is reachable from the slash menu', () => {
    const item = slashItems().find((i) => i.key === SECTION_BREAK)
    expect(item).toBeTruthy()
    expect(item.keywords).toContain('альбомная')
  })

  it('captions a break with the orientation and size that follow it', () => {
    expect(sectionCaption(LANDSCAPE)).toContain('A4')
    expect(sectionCaption(DEFAULT_PAGE)).toContain('A4')
    expect(sectionCaption(LANDSCAPE)).not.toEqual(sectionCaption(DEFAULT_PAGE))
  })
})
