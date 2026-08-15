import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export const blockLocksKey = new PluginKey('blockLocks')

// Meta carrying the new lock roster into the plugin state.
const SET_LOCKS = 'blockLocks$set'

/**
 * Top-level blocks with their positions and ids.
 *
 * Locks are taken on top-level blocks only: that is the granularity the user
 * sees (a paragraph, a list, a table), and the one BlockId stamps ids on.
 *
 * @param {object} doc ProseMirror document node
 * @returns {Array<{id: string, from: number, to: number}>}
 */
export function blockRanges(doc) {
  const out = []
  let pos = 0
  doc.forEach((node) => {
    out.push({ id: node.attrs?.id || '', from: pos, to: pos + node.nodeSize })
    pos += node.nodeSize
  })
  return out
}

/**
 * The id of the top-level block the caret sits in, or '' when there is none.
 * This is what the editor claims a lock on as the caret moves.
 *
 * @param {object} state ProseMirror editor state
 * @returns {string}
 */
export function blockIdAtSelection(state) {
  const { from } = state.selection
  for (const b of blockRanges(state.doc)) {
    if (from >= b.from && from <= b.to) return b.id
  }
  return ''
}

/**
 * Whether a position range overlaps a block held by someone else.
 *
 * @param {object} state ProseMirror editor state
 * @param {number} from range start
 * @param {number} to range end
 * @returns {{block_id: string, name: string}|null} the holder, or null
 */
export function lockAt(state, from, to = from) {
  const { locks } = blockLocksKey.getState(state) || { locks: [] }
  if (!locks.length) return null
  const byId = new Map(locks.map((l) => [l.block_id, l]))
  for (const b of blockRanges(state.doc)) {
    if (!byId.has(b.id)) continue
    // Touching the boundary of a block is not editing it: `to === b.from` is the
    // caret sitting at the end of the *previous* block.
    if (from < b.to && to > b.from) return byId.get(b.id)
  }
  return null
}

/** Pushes a new lock roster into a live editor. */
export function applyBlockLocks(view, locks) {
  if (!view) return
  view.dispatch(view.state.tr.setMeta(SET_LOCKS, locks || []).setMeta('addToHistory', false))
}

// Keys that change the document. Navigation, selection and the read-only
// shortcuts stay allowed inside a locked block — being unable to *read* or copy
// a paragraph someone else is editing would be a strictly worse experience than
// the lock is worth.
const EDITING_KEYS = new Set(['Enter', 'Backspace', 'Delete', 'Tab'])
const EDITING_CHORDS = new Set(['v', 'x', 'z', 'y', 'b', 'i', 'u', 'd', 'k'])

function isEditingKey(event) {
  if (event.ctrlKey || event.metaKey) return EDITING_CHORDS.has(event.key.toLowerCase())
  if (event.altKey) return false
  if (EDITING_KEYS.has(event.key)) return true
  // A single printable character; named keys ("ArrowLeft", "Shift") are longer.
  return event.key.length === 1
}

/**
 * BlockLocks paints the blocks other people are editing and keeps this client
 * out of them.
 *
 * The block is guarded at the *input* level (keystrokes, paste, drop) rather
 * than by filtering transactions. Two reasons, both learned the hard way:
 * loading a document (`setContent`) rewrites every block at once and would be
 * refused by a transaction filter while anyone holds a lock; and a soft lock is
 * supposed to stop a person from typing, not to make the editor reject its own
 * programmatic edits.
 *
 * That is also the honest boundary of the MVP: this is cooperative locking, not
 * a merge. Two clients that both ignore the badge can still write the same block
 * — what makes them not do that is that neither is given the chance to type in
 * it, not a guarantee at the data layer.
 */
export const BlockLocks = Extension.create({
  name: 'blockLocks',

  addOptions() {
    return {
      // Called when an edit is refused, so the UI can say who holds the block
      // instead of looking like the keyboard died.
      onBlocked: () => {},
    }
  },

  addProseMirrorPlugins() {
    const options = this.options
    return [
      new Plugin({
        key: blockLocksKey,
        state: {
          init: () => ({ locks: [] }),
          apply(tr, value) {
            const next = tr.getMeta(SET_LOCKS)
            return next ? { locks: next } : value
          },
        },
        props: {
          decorations(state) {
            const { locks } = blockLocksKey.getState(state) || { locks: [] }
            if (!locks.length) return DecorationSet.empty
            const byId = new Map(locks.map((l) => [l.block_id, l]))
            const decos = []
            for (const b of blockRanges(state.doc)) {
              const held = byId.get(b.id)
              if (!held) continue
              decos.push(
                Decoration.node(b.from, b.to, {
                  class: 'doc-block-locked',
                  'data-locked-by': held.name || '',
                }),
              )
            }
            return DecorationSet.create(state.doc, decos)
          },
          handleKeyDown(view, event) {
            if (!isEditingKey(event)) return false
            const { from, to } = view.state.selection
            const held = lockAt(view.state, from, to)
            if (!held) return false
            options.onBlocked(held)
            return true
          },
          handleTextInput(view, from, to) {
            const held = lockAt(view.state, from, to)
            if (!held) return false
            options.onBlocked(held)
            return true
          },
          handlePaste(view) {
            const { from, to } = view.state.selection
            const held = lockAt(view.state, from, to)
            if (!held) return false
            options.onBlocked(held)
            return true
          },
          handleDrop(view, event, _slice, moved) {
            const at = view.posAtCoords({ left: event.clientX, top: event.clientY })
            // A drop lands in one block and, when it is a move, empties another —
            // both have to be free.
            const target = at ? lockAt(view.state, at.pos, at.pos) : null
            const { from, to } = view.state.selection
            const source = moved ? lockAt(view.state, from, to) : null
            const held = target || source
            if (!held) return false
            options.onBlocked(held)
            return true
          },
        },
      }),
    ]
  },
})
