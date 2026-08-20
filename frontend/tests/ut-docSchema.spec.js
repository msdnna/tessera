import { describe, it, expect } from 'vitest'
import { Editor, generateHTML, getSchema } from '@tiptap/core'
import { GapCursor } from '@tiptap/pm/gapcursor'
import {
  contrastRatio,
  darkSheetFill,
  darkSheetInk,
  darkSheetLine,
  parseCssColor,
} from '@/utils/docColor'
import { htmlToDoc } from '@/utils/docImport'
import { DARK } from '@/styles/tokens'
import {
  ALLOWED_ATTRS,
  ALLOWED_MARKS,
  ALLOWED_NODES,
  EMPTY_DOC,
  docExtensions,
  docPlainText,
  editableDoc,
  isDocJSON,
  isEmptyDoc,
  toDocJSON,
} from '@/utils/docSchema'

const schema = getSchema(docExtensions())

describe('document schema', () => {
  // The allow-list is what the backend validator mirrors. If an extension is
  // added and this list is not updated, the server starts rejecting content the
  // editor produces — a silent, hard-to-trace break. Hence the parity check
  // rather than a hand-written enumeration of "expected" names.
  it('declares exactly the node types the extensions provide', () => {
    expect(Object.keys(schema.nodes).sort()).toEqual([...ALLOWED_NODES].sort())
  })

  it('declares exactly the marks the extensions provide', () => {
    expect(Object.keys(schema.marks).sort()).toEqual([...ALLOWED_MARKS].sort())
  })

  // The node and mark checks above did not catch the case that actually broke:
  // TipTap puts 'align' on table cells and 'type' on ordered lists by itself, so
  // neither name appears anywhere in docSchema.js, and the server rejected every
  // document containing a table (#2728). Deriving the set from the schema is the
  // only version of this check that cannot drift.
  it('declares exactly the attributes the extensions provide', () => {
    const attrs = new Set()
    for (const type of [...Object.values(schema.nodes), ...Object.values(schema.marks)]) {
      Object.keys(type.spec.attrs || {}).forEach((a) => attrs.add(a))
    }
    expect([...attrs].sort()).toEqual([...ALLOWED_ATTRS].sort())
  })

  it('drops a node type that is not in the schema when parsing', () => {
    const node = schema.nodeFromJSON({
      type: 'doc',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'ok' }] }],
    })
    expect(node.type.name).toBe('doc')
    expect(() => schema.nodeFromJSON({ type: 'doc', content: [{ type: 'scriptBlock' }] })).toThrow()
  })

  it('accepts the empty document', () => {
    expect(isDocJSON(EMPTY_DOC)).toBe(true)
    expect(() => schema.nodeFromJSON(EMPTY_DOC)).not.toThrow()
  })

  it('normalises junk into an empty document', () => {
    expect(toDocJSON(null)).toEqual(EMPTY_DOC)
    expect(toDocJSON({ type: 'paragraph' })).toEqual(EMPTY_DOC)
    expect(toDocJSON('nope')).toEqual(EMPTY_DOC)
  })
})

// Text colour on the dark sheet (задача 2755). An imported Word document brings
// colours picked against white paper, so the mark renders the original *and* a
// lightness-lifted variant the dark theme switches to. The stored attribute has
// to stay the author's colour — baking the correction into the document would
// export it back to .docx as a colour nobody chose.
describe('text colour', () => {
  const render = (color) =>
    generateHTML(
      {
        type: 'doc',
        content: [
          {
            type: 'paragraph',
            content: [
              { type: 'text', marks: [{ type: 'textStyle', attrs: { color } }], text: 'т' },
            ],
          },
        ],
      },
      docExtensions(),
    )

  it('renders the document colour and the dark-theme variant beside it', () => {
    const html = render('#1f4e79')
    expect(html).toContain('--doc-ink-dark: ' + darkSheetInk('#1f4e79'))
  })

  // The colour is written as the *fallback* of a custom property, and that is
  // the whole mechanism rather than a detail of formatting: an inline
  // declaration outranks any stylesheet rule, so the dark theme cannot repaint
  // `color` — it can only supply the property the value hangs off. Asserting
  // the plain form here would pass while the dark sheet showed the original,
  // unreadable colour.
  it('leaves the painted value for the theme to take over', () => {
    expect(render('#1f4e79')).toContain('color: var(--doc-ink, #1f4e79)')
  })

  it('reads its own output back as the document colour, not as the var()', () => {
    const doc = htmlToDoc(render('#1f4e79'))
    const mark = doc.content[0].content[0].marks.find((m) => m.type === 'textStyle')
    expect(mark.attrs.color).toBe('#1f4e79')
  })

  it('emits nothing extra for text that carries no colour', () => {
    expect(render(null)).not.toContain('--doc-ink-dark')
  })
})

// Table fills and borders from an imported document (задача 2756). LibreOffice
// writes them on the cell as `bgcolor` plus a `background:`/`border:` shorthand,
// and before this the schema had nowhere to put either, so every imported table
// arrived as the sheet's own grey grid.
describe('table cell styling', () => {
  // Exactly as the sidecar writes it for the document in the task.
  const CELL =
    '<table><tbody><tr>' +
    '<td width="231" bgcolor="#d9e2f3" style="background: #d9e2f3; border: 1px solid #000000; padding: 0in 0.08in">Шапка</td>' +
    '<td width="306" style="border: 1px solid #000000; padding: 0in 0.08in">Тело</td>' +
    '</tr></tbody></table>'

  const cells = (html) => htmlToDoc(html).content[0].content[0].content

  it('keeps the fill and the border colour the document declared', () => {
    const [filled, plain] = cells(CELL)
    expect(filled.attrs.backgroundColor).toBe('#d9e2f3')
    expect(filled.attrs.borderColor).toBe('#000000')
    // A cell with no fill of its own must not gain one, or every table in every
    // document would carry an attribute that means "no colour".
    expect(plain.attrs.backgroundColor).toBe(null)
    expect(plain.attrs.borderColor).toBe('#000000')
  })

  it('carries a fill declared only as the legacy bgcolor attribute', () => {
    const [cell] = cells('<table><tbody><tr><td bgcolor="#d9e2f3">Шапка</td></tr></tbody></table>')
    expect(cell.attrs.backgroundColor).toBe('#d9e2f3')
  })

  it('ignores the spellings that mean "no fill"', () => {
    const [cell] = cells(
      '<table><tbody><tr><td style="background: transparent; border-color: rgba(0, 0, 0, 0)">т</td></tr></tbody></table>',
    )
    expect(cell.attrs.backgroundColor).toBe(null)
    expect(cell.attrs.borderColor).toBe(null)
  })

  it('renders both colours with a dark-sheet variant beside them', () => {
    const html = generateHTML(htmlToDoc(CELL), docExtensions())
    expect(html).toContain('background-color: var(--doc-fill, #d9e2f3)')
    expect(html).toContain('--doc-fill-dark: ' + darkSheetFill('#d9e2f3'))
    expect(html).toContain('border-color: var(--doc-line, #000000)')
    expect(html).toContain('--doc-line-dark: ' + darkSheetLine('#000000'))
  })

  it('reads its own output back as the document colours', () => {
    const [cell] = cells(generateHTML(htmlToDoc(CELL), docExtensions()))
    expect(cell.attrs.backgroundColor).toBe('#d9e2f3')
    expect(cell.attrs.borderColor).toBe('#000000')
  })

  // The dark variants are what make the import readable rather than merely
  // faithful: a pale Word header band under the dark theme's light text is the
  // one combination that cannot be read at all.
  it('darkens a fill enough to carry the dark sheet ink', () => {
    expect(
      contrastRatio(parseCssColor(darkSheetFill('#d9e2f3')), parseCssColor(DARK.text1)),
    ).toBeGreaterThanOrEqual(4.5)
  })
})

// An empty `doc` (content: []) leaves the editor with nowhere to put the caret,
// so ProseMirror shows a gap-cursor and the placeholder never renders (#2761).
// editableDoc seeds one empty paragraph for the value handed to the editor.
describe('editableDoc', () => {
  it('seeds one empty paragraph for a blockless document', () => {
    expect(editableDoc(EMPTY_DOC)).toEqual({ type: 'doc', content: [{ type: 'paragraph' }] })
  })

  it('seeds a paragraph for null and junk too', () => {
    const seeded = { type: 'doc', content: [{ type: 'paragraph' }] }
    expect(editableDoc(null)).toEqual(seeded)
    expect(editableDoc('nope')).toEqual(seeded)
    expect(editableDoc({ type: 'paragraph' })).toEqual(seeded)
  })

  it('leaves a document that already has blocks untouched', () => {
    const doc = {
      type: 'doc',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'ok' }] }],
    }
    expect(editableDoc(doc)).toBe(doc)
  })

  it('produces a document the schema accepts', () => {
    expect(() => schema.nodeFromJSON(editableDoc(EMPTY_DOC))).not.toThrow()
  })
})

// Regression on a real editor: a document built from the canonical empty form
// must land on a normal text caret with the placeholder showing, not the
// gap-cursor that used to require pressing Enter to escape (#2761).
describe('empty document caret', () => {
  it('gives an empty document a text caret and the placeholder, not a gap-cursor', () => {
    const editor = new Editor({
      element: document.createElement('div'),
      extensions: docExtensions(),
      content: editableDoc(EMPTY_DOC),
    })
    try {
      expect(editor.state.doc.childCount).toBe(1)
      expect(editor.state.selection instanceof GapCursor).toBe(false)
      // isEmpty is the condition TipTap uses to draw the placeholder decoration.
      expect(editor.isEmpty).toBe(true)
    } finally {
      editor.destroy()
    }
  })
})

// The insertion line drawn while a block is dragged (#2728). Worth pinning
// because the failure is invisible to every other check: the default colour is
// `currentColor`, which resolves against the body text and paints a black line
// that no theme can reach. `color: false` is the only value that stops
// prosemirror-dropcursor writing an inline background-color, and the moment it
// does, the CSS rule in DocEditor.vue is dead and the line is black again.
describe('drop cursor', () => {
  // Read off StarterKit rather than off a top-level extension: the sub-extension
  // is only instantiated when an Editor is built, and the options object is
  // where the configuration either arrived or silently did not.
  const starterKit = docExtensions().find((e) => e.name === 'starterKit')

  it('is still bundled in StarterKit under the key being configured', () => {
    expect(starterKit).toBeDefined()
    expect(starterKit.options.dropcursor).not.toBe(false)
  })

  it('draws no colour of its own and carries the class the theme styles', () => {
    expect(starterKit.options.dropcursor.color).toBe(false)
    expect(starterKit.options.dropcursor.class).toBe('doc-dropcursor')
  })
})

describe('docPlainText', () => {
  const doc = {
    type: 'doc',
    content: [
      { type: 'paragraph', content: [{ type: 'text', text: 'первый' }] },
      { type: 'paragraph', content: [{ type: 'text', text: 'второй' }] },
    ],
  }

  it('joins blocks with a space rather than gluing words together', () => {
    expect(docPlainText(doc)).toBe('первый второй')
  })

  it('truncates to the limit', () => {
    expect(docPlainText(doc, 6)).toBe('первый')
  })

  it('returns empty text for an empty document', () => {
    expect(docPlainText(EMPTY_DOC)).toBe('')
  })
})

describe('isEmptyDoc', () => {
  it('is true for the empty document and for junk', () => {
    expect(isEmptyDoc(EMPTY_DOC)).toBe(true)
    expect(isEmptyDoc(null)).toBe(true)
  })

  it('is false when there is text', () => {
    expect(
      isEmptyDoc({
        type: 'doc',
        content: [{ type: 'paragraph', content: [{ type: 'text', text: 'x' }] }],
      }),
    ).toBe(false)
  })

  // A document holding only an image has no text but is not empty — showing
  // "пустой документ" on its tile would be wrong.
  it('is false for a document with only an image', () => {
    expect(
      isEmptyDoc({ type: 'doc', content: [{ type: 'image', attrs: { src: '/a.png' } }] }),
    ).toBe(false)
  })
})
