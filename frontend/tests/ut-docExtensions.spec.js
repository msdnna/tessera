import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import { MAX_INDENT } from '@/utils/docExtensions/blockStyle'

// Mirrors DocEditor: content is stamped before it reaches the editor.
function makeEditor(...texts) {
  return new Editor({
    element: document.createElement('div'),
    extensions: docExtensions(),
    content: ensureBlockIds({
      type: 'doc',
      content: texts.map((t) => ({ type: 'paragraph', content: [{ type: 'text', text: t }] })),
    }),
  })
}

function paragraphs(editor) {
  return editor.getJSON().content || []
}

describe('BlockId', () => {
  let editor

  beforeEach(() => {
    editor = makeEditor('раз', 'два')
  })

  afterEach(() => editor.destroy())

  // The editor's own create hook fires a tick late, so loaded content is
  // stamped on the JSON before it ever reaches the editor. That is what this
  // checks: a document must never be readable without ids, not even briefly —
  // D4 locks and D5 annotations address blocks by exactly this attribute.
  it('stamps an id on every block of loaded content', () => {
    const stamped = ensureBlockIds({
      type: 'doc',
      content: [
        { type: 'paragraph', content: [{ type: 'text', text: 'раз' }] },
        { type: 'paragraph', content: [{ type: 'text', text: 'два' }] },
      ],
    })
    const ids = stamped.content.map((n) => n.attrs.id)
    expect(ids.filter(Boolean)).toHaveLength(2)
    expect(new Set(ids).size).toBe(2)
  })

  it('re-issues an id repeated in loaded content', () => {
    const dup = { type: 'paragraph', attrs: { id: 'same' }, content: [{ type: 'text', text: 'x' }] }
    const stamped = ensureBlockIds({ type: 'doc', content: [dup, { ...dup }] })
    const [a, b] = stamped.content.map((n) => n.attrs.id)
    expect(a).toBe('same')
    expect(b).not.toBe('same')
  })

  it('stamps ids on the editor content', () => {
    const ids = paragraphs(editor).map((n) => n.attrs.id)
    expect(ids.filter(Boolean)).toHaveLength(2)
    expect(new Set(ids).size).toBe(2)
  })

  // The ids are what D4 locks and D5 annotates. Re-issuing them on every load
  // would silently move every comment to a different paragraph.
  it('keeps existing ids when the same content is loaded again', () => {
    const before = paragraphs(editor).map((n) => n.attrs.id)
    editor.commands.setContent(editor.getJSON())
    expect(paragraphs(editor).map((n) => n.attrs.id)).toEqual(before)
  })

  // Pasting a copied block brings its id along; leaving the duplicate in place
  // would make one anchor address two paragraphs.
  it('re-issues an id that arrives twice in one document', () => {
    const first = paragraphs(editor)[0]
    editor.commands.setContent({
      type: 'doc',
      content: [first, JSON.parse(JSON.stringify(first))],
    })
    const ids = paragraphs(editor).map((n) => n.attrs.id)
    expect(ids[0]).toBeTruthy()
    expect(ids[1]).toBeTruthy()
    expect(ids[0]).not.toBe(ids[1])
  })
})

describe('BlockStyle', () => {
  let editor

  beforeEach(() => {
    editor = makeEditor('текст')
  })

  afterEach(() => editor.destroy())

  it('sets and clears the line height on the block', () => {
    editor.commands.selectAll()
    editor.commands.setLineHeight('1.5')
    expect(paragraphs(editor)[0].attrs.lineHeight).toBe('1.5')
    editor.commands.unsetLineHeight()
    expect(paragraphs(editor)[0].attrs.lineHeight).toBeNull()
  })

  it('indents and outdents', () => {
    editor.commands.selectAll()
    expect(editor.commands.indent()).toBe(true)
    expect(paragraphs(editor)[0].attrs.indent).toBe(1)
    expect(editor.commands.outdent()).toBe(true)
    expect(paragraphs(editor)[0].attrs.indent).toBeNull()
  })

  // A command that cannot do anything must report false, or the toolbar button
  // looks like it worked.
  it('refuses to outdent below zero and to indent past the cap', () => {
    editor.commands.selectAll()
    expect(editor.commands.outdent()).toBe(false)
    for (let i = 0; i < MAX_INDENT; i++) editor.commands.indent()
    expect(paragraphs(editor)[0].attrs.indent).toBe(MAX_INDENT)
    expect(editor.commands.indent()).toBe(false)
  })
})
