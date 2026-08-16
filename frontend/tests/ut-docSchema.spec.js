import { describe, it, expect } from 'vitest'
import { getSchema } from '@tiptap/core'
import {
  ALLOWED_ATTRS,
  ALLOWED_MARKS,
  ALLOWED_NODES,
  EMPTY_DOC,
  docExtensions,
  docPlainText,
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
