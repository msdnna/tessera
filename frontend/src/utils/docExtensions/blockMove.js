import { Extension } from '@tiptap/core'
import { NodeSelection } from '@tiptap/pm/state'
import { lockInMoveSpan, reportBlocked } from './blockLocks'

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
 * Whether both indexes address blocks of this document and differ.
 *
 * Split out of moveBlockTransaction because the commands have to answer "is
 * this move in range?" *before* building the transaction: under TipTap
 * `state.tr` is not a fresh transaction but the chain's shared one, so merely
 * constructing the move already edits the document, and a command that then
 * returns false still leaves its steps to be dispatched. Every check that can
 * refuse a move therefore runs on this, ahead of the builder.
 *
 * @param {object} doc ProseMirror document node
 * @param {number} from source index
 * @param {number} to destination index
 * @returns {boolean}
 */
export function canMoveBlock(doc, from, to) {
  if (from === to) return false
  if (from < 0 || from >= doc.childCount) return false
  return to >= 0 && to < doc.childCount
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
  if (!canMoveBlock(doc, from, to)) return null

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
 * Whether a lock stands in the way of this move, telling the user if so.
 *
 * Moving a block is an edit to it, and the drag half of the same feature has
 * always treated it as one — BlockLocks.handleDrop refuses a drop whose target
 * or source is held. The keyboard half used to go around that guard: isEditingKey
 * answers `false` for anything with Alt held, so Alt+Shift+Arrow walked a
 * paragraph someone else was typing in straight out from under them, and the
 * badge saying who held it never appeared (#2932).
 *
 * Checked in the command rather than in the shortcut so `moveBlockTo` — which
 * the drag handle may dispatch, and which jumps instead of stepping — is behind
 * the same guard.
 *
 * @param {object} state editor state
 * @param {number} from source index
 * @param {number} to destination index
 * @returns {boolean} true when the move must not happen
 */
function movesLockedBlock(state, from, to) {
  const held = lockInMoveSpan(state, from, to)
  if (!held) return false
  reportBlocked(state, held)
  return true
}

/**
 * The body every move command shares: refuse what is out of range, refuse what
 * a lock holds, and only then build and dispatch.
 *
 * @param {object} state editor state
 * @param {Function|undefined} dispatch ProseMirror dispatch, absent on a `can()` probe
 * @param {number} from source index
 * @param {number} to destination index
 * @returns {boolean} whether the move happened
 */
function moveBlock(state, dispatch, from, to) {
  // An out-of-range move is nothing happening, not a refusal, so it is checked
  // first: walking into the top of the document should not accuse a lock.
  if (!canMoveBlock(state.doc, from, to)) return false
  if (movesLockedBlock(state, from, to)) return false
  const tr = moveBlockTransaction(state, from, to)
  if (!tr) return false
  if (dispatch) dispatch(tr)
  return true
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
        return moveBlock(state, dispatch, from, from + delta)
      }

    return {
      moveBlockUp: byDelta(-1),
      moveBlockDown: byDelta(1),
      moveBlockTo:
        (from, to) =>
        ({ state, dispatch }) =>
          moveBlock(state, dispatch, from, to),
    }
  },

  addKeyboardShortcuts() {
    return {
      'Alt-Shift-ArrowUp': () => this.editor.commands.moveBlockUp(),
      'Alt-Shift-ArrowDown': () => this.editor.commands.moveBlockDown(),
    }
  },
})
