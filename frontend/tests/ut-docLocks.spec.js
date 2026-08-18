import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import {
  applyBlockLocks,
  blockIdAtSelection,
  blockRanges,
  lockAt,
} from '@/utils/docExtensions/blockLocks'

// Mirrors DocEditor: content is stamped with block ids before it reaches the
// editor, because that is the attribute a lock addresses.
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

// Puts the caret inside the paragraph at `index`.
function caretIn(editor, index) {
  editor.commands.setTextSelection(blockRanges(editor.state.doc)[index].from + 1)
}

function press(editor, key, opts = {}) {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...opts })
  return editor.view.someProp('handleKeyDown', (f) => f(editor.view, event)) === true
}

describe('BlockLocks', () => {
  let editor
  let onBlocked

  beforeEach(() => {
    onBlocked = vi.fn()
    editor = makeEditor(onBlocked)
  })
  afterEach(() => editor.destroy())

  it('reports the block the caret sits in', () => {
    caretIn(editor, 1)
    expect(blockIdAtSelection(editor.state)).toBe(idOf(editor, 1))
  })

  it('finds the holder of a locked range and leaves free blocks alone', () => {
    lockBlock(editor, 1)
    const { from, to } = blockRanges(editor.state.doc)[1]
    expect(lockAt(editor.state, from + 1, from + 1).name).toBe('Боб')
    // The boundary belongs to the neighbour: a caret at the end of block 0 is
    // not inside block 1.
    expect(lockAt(editor.state, to, to)).toBeNull()
    const free = blockRanges(editor.state.doc)[0]
    expect(lockAt(editor.state, free.from + 1, free.from + 1)).toBeNull()
  })

  it('paints locked blocks with the holder’s name', () => {
    lockBlock(editor, 1, 'Алиса')
    const locked = editor.view.dom.querySelectorAll('.doc-block-locked')
    expect(locked).toHaveLength(1)
    // The name rides on the node so CSS can show it without a second component.
    expect(locked[0].getAttribute('data-locked-by')).toBe('Алиса')
  })

  it('swallows typing inside a block someone else holds', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)

    expect(press(editor, 'я')).toBe(true)
    expect(press(editor, 'Backspace')).toBe(true)
    expect(press(editor, 'Enter')).toBe(true)
    // Refusing silently reads as a broken keyboard, so the caller is told who
    // holds the block.
    expect(onBlocked).toHaveBeenCalledTimes(3)
    expect(onBlocked.mock.calls[0][0].name).toBe('Боб')
  })

  // A lock stops the person from typing in *that* block, not in the document:
  // note that `press` reports whether any plugin handled the key (Enter is
  // claimed by the keymap), so the guard is judged by onBlocked, not by that.
  it('leaves the rest of the document editable', () => {
    lockBlock(editor, 1)
    caretIn(editor, 0)
    press(editor, 'я')
    press(editor, 'Enter')
    expect(onBlocked).not.toHaveBeenCalled()
  })

  // Being unable to read, select or copy a paragraph someone is editing would
  // cost more than the lock is worth.
  it('still allows navigation and copying inside a locked block', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)
    press(editor, 'ArrowRight')
    press(editor, 'End')
    press(editor, 'c', { ctrlKey: true })
    expect(onBlocked).not.toHaveBeenCalled()
    // Paste does change the document, so it is refused.
    expect(press(editor, 'v', { ctrlKey: true })).toBe(true)
    expect(onBlocked).toHaveBeenCalledTimes(1)
  })

  it('releasing the lock makes the block editable again', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)
    expect(press(editor, 'я')).toBe(true)
    applyBlockLocks(editor.view, [])
    expect(press(editor, 'я')).toBe(false)
    expect(editor.view.dom.querySelectorAll('.doc-block-locked')).toHaveLength(0)
  })

  // Loading a document rewrites every block at once. A transaction-level guard
  // would refuse that while anyone held a lock — which is why the guard sits at
  // the input level instead.
  it('does not block programmatic content changes', () => {
    lockBlock(editor, 1)
    editor.commands.setContent(
      ensureBlockIds({
        type: 'doc',
        content: [{ type: 'paragraph', content: [{ type: 'text', text: 'новое' }] }],
      }),
      { emitUpdate: false },
    )
    expect(editor.getText()).toContain('новое')
  })

  it('marks nothing when the roster is empty', () => {
    applyBlockLocks(editor.view, [])
    caretIn(editor, 1)
    expect(lockAt(editor.state, editor.state.selection.from)).toBeNull()
    expect(editor.view.dom.querySelectorAll('.doc-block-locked')).toHaveLength(0)
  })
})
