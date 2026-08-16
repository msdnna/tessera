import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import { MAX_INDENT } from '@/utils/docExtensions/blockStyle'
import {
  blockAtClientY,
  centerOffset,
  firstLineBox,
  topLevelBlocks,
} from '@/utils/docExtensions/dragHandle'
import { imageFilesFrom, uploadImagesAt } from '@/utils/docExtensions/imageDrop'

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

function texts(editor) {
  return paragraphs(editor).map((n) => n.content?.[0]?.text ?? n.type)
}

// jsdom gives every element a zero rect, so the geometry has to be supplied.
function fakeBlocks(...ranges) {
  return ranges.map(([top, bottom], index) => ({
    index,
    pos: index,
    dom: { getBoundingClientRect: () => ({ top, bottom }) },
  }))
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

describe('BlockMove', () => {
  let editor

  beforeEach(() => {
    editor = makeEditor('раз', 'два', 'три')
  })

  afterEach(() => editor.destroy())

  it('moves the block holding the cursor down and back up', () => {
    editor.commands.setTextSelection(2) // inside "раз"
    expect(editor.commands.moveBlockDown()).toBe(true)
    expect(texts(editor)).toEqual(['два', 'раз', 'три'])
    expect(editor.commands.moveBlockUp()).toBe(true)
    expect(texts(editor)).toEqual(['раз', 'два', 'три'])
  })

  // The command leaves the moved block selected, which is what lets the user
  // hold Alt+Shift+Down and walk a paragraph to the bottom.
  it('keeps addressing the same block across repeated moves', () => {
    editor.commands.setTextSelection(2)
    editor.commands.moveBlockDown()
    editor.commands.moveBlockDown()
    expect(texts(editor)).toEqual(['два', 'три', 'раз'])
  })

  // A no-op at the edge must report false, or the drag handle and the shortcut
  // look like they did something.
  it('refuses to move past either end', () => {
    editor.commands.setTextSelection(2)
    expect(editor.commands.moveBlockUp()).toBe(false)
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    expect(editor.commands.moveBlockDown()).toBe(false)
    expect(texts(editor)).toEqual(['раз', 'два', 'три'])
  })

  it('moves a block to an arbitrary index, as a drop does', () => {
    expect(editor.commands.moveBlockTo(2, 0)).toBe(true)
    expect(texts(editor)).toEqual(['три', 'раз', 'два'])
    expect(editor.commands.moveBlockTo(0, 5)).toBe(false)
  })
})

describe('drag handle geometry', () => {
  it('lists every top-level block with its position', () => {
    const editor = makeEditor('раз', 'два')
    const blocks = topLevelBlocks(editor.view)
    expect(blocks.map((b) => b.index)).toEqual([0, 1])
    expect(blocks[0].pos).toBe(0)
    expect(blocks[1].pos).toBe(editor.state.doc.child(0).nodeSize)
    editor.destroy()
  })

  it('picks the block the pointer is inside', () => {
    const blocks = fakeBlocks([0, 20], [30, 50])
    expect(blockAtClientY(blocks, 10).index).toBe(0)
    expect(blockAtClientY(blocks, 40).index).toBe(1)
  })

  // The gap between two paragraphs is still "on" the nearer one — otherwise the
  // handle flickers off every time the pointer crosses a margin.
  it('snaps to the nearest block within the tolerance', () => {
    const blocks = fakeBlocks([0, 20], [30, 50])
    expect(blockAtClientY(blocks, 26).index).toBe(1)
    expect(blockAtClientY(blocks, 22).index).toBe(0)
  })

  it('addresses nothing far below the last block', () => {
    expect(blockAtClientY(fakeBlocks([0, 20]), 400)).toBeNull()
    expect(blockAtClientY([], 10)).toBeNull()
  })

  // The bug this arithmetic replaces: the handle was anchored to the top of the
  // block's box, which on a heading is a margin above the text it labels.
  it('centres the handle on the line, not on the block top', () => {
    // A 20px line starting at 100 has its middle at 110; a 24px row centred on
    // it therefore starts at 98 — above the line top, which the old
    // "top of the box" anchor could never produce.
    expect(centerOffset(100, 120, 24)).toBe(98)
  })

  it('survives a gutter that has not been measured yet', () => {
    expect(centerOffset(100, 120, 0)).toBe(110)
    expect(centerOffset(100, 120, undefined)).toBe(110)
  })

  // An atom (a rule, an image, a PDF card) has no position inside it, so
  // coordsAtPos answers about the block after it. Trusting that answer would
  // park the handle on the wrong block entirely.
  it('falls back to the block box when the caret box lands outside it', () => {
    const rect = { top: 100, bottom: 140 }
    const outside = { coordsAtPos: () => ({ top: 200, bottom: 220 }) }
    expect(firstLineBox(outside, 0, rect)).toEqual({ top: 100, bottom: 140 })

    const throwing = {
      coordsAtPos: () => {
        throw new Error('view is gone')
      },
    }
    expect(firstLineBox(throwing, 0, rect)).toEqual({ top: 100, bottom: 140 })
  })

  it('uses the caret box when it sits inside the block', () => {
    const rect = { top: 100, bottom: 180 }
    const view = { coordsAtPos: () => ({ top: 104, bottom: 124 }) }
    expect(firstLineBox(view, 0, rect)).toEqual({ top: 104, bottom: 124 })
  })
})

describe('ImageDrop', () => {
  const png = () => new File(['x'], 'кот.png', { type: 'image/png' })

  it('keeps only image files', () => {
    const files = [png(), new File(['x'], 'a.txt', { type: 'text/plain' })]
    expect(imageFilesFrom(files).map((f) => f.name)).toEqual(['кот.png'])
    expect(imageFilesFrom(null)).toEqual([])
  })

  it('inserts the uploaded image at the drop position', async () => {
    const editor = makeEditor('раз')
    await uploadImagesAt(editor.view, [png()], 0, { upload: async () => '/api/assets/1.png' })
    const [first] = paragraphs(editor)
    expect(first.type).toBe('image')
    expect(first.attrs.src).toBe('/api/assets/1.png')
    expect(first.attrs.alt).toBe('кот.png')
    editor.destroy()
  })

  // The whole point of the placeholder decoration: it rides along with the
  // document, so edits made while the upload is in flight do not misplace the
  // image. A remembered offset would drop it into the newly typed paragraph.
  it('lands where it was dropped even when the document shifts mid-upload', async () => {
    const editor = makeEditor('раз')
    let finish
    const pending = uploadImagesAt(editor.view, [png()], editor.state.doc.content.size, {
      upload: () => new Promise((resolve) => (finish = resolve)),
    })
    editor.commands.insertContentAt(0, {
      type: 'paragraph',
      content: [{ type: 'text', text: 'новый' }],
    })
    finish('/api/assets/2.png')
    await pending
    expect(texts(editor)).toEqual(['новый', 'раз', 'image'])
    editor.destroy()
  })

  it('reports a failed upload and leaves no placeholder behind', async () => {
    const editor = makeEditor('раз')
    const onError = vi.fn()
    await uploadImagesAt(editor.view, [png()], 0, {
      upload: async () => {
        throw new Error('нет сети')
      },
      onError,
    })
    expect(onError).toHaveBeenCalledOnce()
    expect(texts(editor)).toEqual(['раз'])
    expect(editor.view.dom.querySelector('.doc-upload-placeholder')).toBeNull()
    editor.destroy()
  })
})
