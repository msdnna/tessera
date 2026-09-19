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

  // A modifier does not make an erasing key harmless — it makes it hungrier:
  // Ctrl/Alt+Backspace eats the previous word, Cmd+Backspace the rest of the
  // line. The guard used to consult only its chord table once any modifier was
  // down, so every one of these walked straight through the lock.
  it('swallows word- and line-wise erases inside a locked block', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)

    // Judged by onBlocked, not by the return of `press`: a chord the guard lets
    // pass can still be claimed by some other plugin's keymap, which would read
    // as a green.
    for (const opts of [
      { ctrlKey: true },
      { altKey: true },
      { metaKey: true },
      { ctrlKey: true, shiftKey: true },
    ]) {
      onBlocked.mockClear()
      expect(press(editor, 'Backspace', opts)).toBe(true)
      expect(onBlocked).toHaveBeenCalledTimes(1)
      expect(onBlocked.mock.calls[0][0].name).toBe('Боб')
    }
    for (const key of ['Delete', 'Enter']) {
      onBlocked.mockClear()
      expect(press(editor, key, { ctrlKey: true })).toBe(true)
      expect(onBlocked).toHaveBeenCalledTimes(1)
    }
  })

  // The other half of the same rule: outside the lock those chords must still
  // reach the document, and the browser's own Ctrl+Tab must never be eaten.
  it('leaves modified erases alone outside the lock', () => {
    lockBlock(editor, 1)
    caretIn(editor, 0)
    press(editor, 'Backspace', { ctrlKey: true })
    press(editor, 'Delete', { metaKey: true })
    expect(onBlocked).not.toHaveBeenCalled()

    caretIn(editor, 1)
    expect(press(editor, 'Tab', { ctrlKey: true })).toBe(false)
    expect(onBlocked).not.toHaveBeenCalled()
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

  // Relocating a block is an edit to it, and the drag-and-drop half of the same
  // feature has always said so (handleDrop refuses a drop whose target or
  // source is held). The keyboard half went around the guard entirely: Alt is
  // not an editing modifier as far as isEditingKey is concerned, so
  // Alt+Shift+Arrow never reached it (#2932).
  it('refuses to relocate a block someone else holds', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)
    const before = editor.getJSON()
    expect(editor.commands.moveBlockUp()).toBe(false)
    expect(editor.getJSON()).toEqual(before)
    expect(onBlocked).toHaveBeenCalledTimes(1)
    expect(onBlocked.mock.calls[0][0].name).toBe('Боб')
  })

  // The block under the caret is free, but the one it swaps with is not: a move
  // relocates both, so both have to be.
  it('refuses a move that would displace a locked neighbour', () => {
    lockBlock(editor, 1)
    caretIn(editor, 0)
    expect(editor.commands.moveBlockDown()).toBe(false)
    expect(editor.getText()).toBe('раз\n\nдва\n\nтри')
    expect(onBlocked).toHaveBeenCalledTimes(1)
  })

  // moveBlockTo jumps rather than steps, and every block it passes is pushed by
  // one, so the whole span is what the lock is checked against.
  it('refuses a jump across a locked block', () => {
    lockBlock(editor, 1)
    expect(editor.commands.moveBlockTo(2, 0)).toBe(false)
    expect(onBlocked).toHaveBeenCalledTimes(1)
  })

  it('goes through the shortcut, not only the command', () => {
    lockBlock(editor, 1)
    caretIn(editor, 1)
    const before = editor.getJSON()
    press(editor, 'ArrowUp', { altKey: true, shiftKey: true })
    expect(editor.getJSON()).toEqual(before)
    expect(onBlocked).toHaveBeenCalled()
  })

  // The guard is about locks, not about moving: blocks clear of every lock keep
  // working, or the shortcut would be dead the moment anyone opened the page.
  it('still moves blocks clear of every lock', () => {
    lockBlock(editor, 0)
    caretIn(editor, 2)
    expect(editor.commands.moveBlockUp()).toBe(true)
    expect(editor.getText()).toBe('раз\n\nтри\n\nдва')
    expect(onBlocked).not.toHaveBeenCalled()
  })

  it('marks nothing when the roster is empty', () => {
    applyBlockLocks(editor.view, [])
    caretIn(editor, 1)
    expect(lockAt(editor.state, editor.state.selection.from)).toBeNull()
    expect(editor.view.dom.querySelectorAll('.doc-block-locked')).toHaveLength(0)
  })
})
