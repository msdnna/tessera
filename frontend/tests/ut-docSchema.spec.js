import { describe, it, expect } from 'vitest'
import { getSchema } from '@tiptap/core'
import {
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
