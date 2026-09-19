import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import { applyBlockLocks, blockRanges } from '@/utils/docExtensions/blockLocks'

// Same shape as ut-docLocks.spec.js: ids are stamped before the content reaches
// the editor, because the id is what a lock addresses.
function makeEditor(onBlocked = () => {}) {
  return new Editor({
    element: document.createElement('div'),
    extensions: docExtensions({ onBlocked }),
    content: ensureBlockIds({
      type: 'doc',
      content: ['раз', 'два', 'три'].map((t) => ({
        type: 'paragraph',
        content: [{ type: 'text', text: t }],
      })),
    }),
  })
}

function idOf(editor, index) {
  return blockRanges(editor.state.doc)[index].id
}

function lockBlock(editor, index, name = 'Боб') {
  applyBlockLocks(editor.view, [
    { block_id: idOf(editor, index), user_id: 'user-2', conn_id: 'conn-2', name },
  ])
}

function unlockAll(editor) {
  applyBlockLocks(editor.view, [])
}

function textOf(editor, index) {
  return editor.state.doc.child(index).textContent
}

// Types into the block at `index`, which is what puts an item on the undo stack.
function editBlock(editor, index, text) {
  const { from } = blockRanges(editor.state.doc)[index]
  editor
    .chain()
    .setTextSelection(from + 1)
    .insertContent(text)
    .run()
}

function caretIn(editor, index) {
  editor.commands.setTextSelection(blockRanges(editor.state.doc)[index].from + 1)
}

describe('undo/redo against a colleague’s block lock', () => {
  let editor
  let onBlocked

  beforeEach(() => {
    onBlocked = vi.fn()
    editor = makeEditor(onBlocked)
  })
  afterEach(() => editor.destroy())

  it('refuses an undo that would rewrite a block someone else now holds', () => {
    editBlock(editor, 1, 'ЖЖ')
    expect(textOf(editor, 1)).toBe('ЖЖдва')
    // The caret moves on, the block is released, and a colleague takes it.
    caretIn(editor, 2)
    lockBlock(editor, 1)

    editor.commands.undo()

    expect(textOf(editor, 1)).toBe('ЖЖдва')
    expect(onBlocked).toHaveBeenCalledWith(expect.objectContaining({ name: 'Боб' }))
  })

  it('refuses a redo that would rewrite a block someone else now holds', () => {
    editBlock(editor, 1, 'ЖЖ')
    caretIn(editor, 2)
    editor.commands.undo()
    expect(textOf(editor, 1)).toBe('два')
    lockBlock(editor, 1)

    editor.commands.redo()

    expect(textOf(editor, 1)).toBe('два')
    expect(onBlocked).toHaveBeenCalledWith(expect.objectContaining({ name: 'Боб' }))
  })

  it('keeps the refused step on the stack, so the undo lands once the lock goes', () => {
    editBlock(editor, 1, 'ЖЖ')
    caretIn(editor, 2)
    lockBlock(editor, 1)
    editor.commands.undo()
    expect(textOf(editor, 1)).toBe('ЖЖдва')

    unlockAll(editor)
    editor.commands.undo()

    expect(textOf(editor, 1)).toBe('два')
  })

  it('undoes an edit in a free block while another block is locked', () => {
    editBlock(editor, 0, 'ЖЖ')
    expect(textOf(editor, 0)).toBe('ЖЖраз')
    lockBlock(editor, 2)

    editor.commands.undo()

    expect(textOf(editor, 0)).toBe('раз')
    expect(onBlocked).not.toHaveBeenCalled()
  })

  it('leaves undo alone when nobody holds anything', () => {
    editBlock(editor, 1, 'ЖЖ')

    editor.commands.undo()

    expect(textOf(editor, 1)).toBe('два')
    expect(onBlocked).not.toHaveBeenCalled()
  })

  it('does not refuse an ordinary edit in a free block', () => {
    lockBlock(editor, 1)

    editBlock(editor, 0, 'ЖЖ')

    expect(textOf(editor, 0)).toBe('ЖЖраз')
    expect(onBlocked).not.toHaveBeenCalled()
  })
})
