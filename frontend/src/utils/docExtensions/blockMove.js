import { Extension } from '@tiptap/core'
import { NodeSelection } from '@tiptap/pm/state'

/**
 * Start positions of every top-level block, in document order.
 * @param {object} doc ProseMirror document node
 * @returns {number[]} position of each child
 */
export function topLevelPositions(doc) {
  const out = []
  doc.forEach((_node, offset) => out.push(offset))
  return out
}

/**
 * Index of the top-level block the selection sits in.
 *
 * `index(0)` is right for both selection kinds: a text cursor resolves inside
 * the block, a NodeSelection resolves immediately before it, and in both cases
 * depth 0 addresses the same child of the document.
 *
 * @param {object} state editor state
 * @returns {number} index into doc.content
 */
export function selectedBlockIndex(state) {
  return state.selection.$from.index(0)
}

/**
 * Builds the transaction that relocates a top-level block, or null when the
 * move is out of range (the caller reports that as a failed command so the
 * drag handle and the keyboard shortcut do not look like they did something).
 *
 * @param {object} state editor state
 * @param {number} from source index
 * @param {number} to destination index, in coordinates of the *original* doc
 * @returns {object|null} transaction to dispatch
 */
export function moveBlockTransaction(state, from, to) {
  const doc = state.doc
  if (from === to) return null
  if (from < 0 || from >= doc.childCount) return null
  if (to < 0 || to >= doc.childCount) return null

  const node = doc.child(from)
  const start = topLevelPositions(doc)[from]
  const tr = state.tr.delete(start, start + node.nodeSize)

  // After the delete the document is one block shorter, so index `to` now
  // means "before whatever ended up there" for a downward move and "before the
  // old occupant" for an upward one — which is what both directions want. The
  // one index that no longer exists is the last, and that is the end of the doc.
  const after = topLevelPositions(tr.doc)
  const insertAt = to < after.length ? after[to] : tr.doc.content.size
  tr.insert(insertAt, node)

  // Selecting the block that just moved keeps repeated Alt+Shift+Arrow working:
  // the next call reads its index off the selection.
  tr.setSelection(NodeSelection.create(tr.doc, insertAt))
  return tr.scrollIntoView()
}

/**
 * BlockMove relocates whole blocks — the keyboard half of item 5 of #2718, and
 * the command the drag handle in DocEditor dispatches once a drop resolves to
 * an index.
 *
 * Alt+Shift+Arrow rather than Ctrl/Cmd+Shift+Arrow: the latter is taken by
 * "extend selection by line" in browsers and by tab switching on Linux.
 */
export const BlockMove = Extension.create({
  name: 'blockMove',

  addCommands() {
    const byDelta =
      (delta) =>
      () =>
      ({ state, dispatch }) => {
        const from = selectedBlockIndex(state)
        const tr = moveBlockTransaction(state, from, from + delta)
        if (!tr) return false
        if (dispatch) dispatch(tr)
        return true
      }

    return {
      moveBlockUp: byDelta(-1),
      moveBlockDown: byDelta(1),
      moveBlockTo:
        (from, to) =>
        ({ state, dispatch }) => {
          const tr = moveBlockTransaction(state, from, to)
          if (!tr) return false
          if (dispatch) dispatch(tr)
          return true
        },
    }
  },

  addKeyboardShortcuts() {
    return {
      'Alt-Shift-ArrowUp': () => this.editor.commands.moveBlockUp(),
      'Alt-Shift-ArrowDown': () => this.editor.commands.moveBlockDown(),
    }
  },
})
