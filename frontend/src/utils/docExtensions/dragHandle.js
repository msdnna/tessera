import { NodeSelection } from '@tiptap/pm/state'

// How far outside a block the pointer may sit and still address it. Without a
// tolerance the handle flickers off in the margin between two paragraphs; with
// an unbounded nearest-match it would hang around below the last block forever.
export const HANDLE_TOLERANCE = 28

/**
 * The top-level blocks of the document paired with their DOM nodes.
 *
 * @param {object} view ProseMirror editor view
 * @returns {Array<{pos:number,index:number,node:object,dom:*}>}
 */
export function topLevelBlocks(view) {
  const out = []
  view.state.doc.forEach((node, offset, index) => {
    out.push({ pos: offset, index, node, dom: view.nodeDOM(offset) })
  })
  return out
}

/**
 * The block a pointer at `y` addresses.
 *
 * Geometry rather than `view.posAtCoords`: the handle lives in a gutter to the
 * left of the editing surface, so the pointer is usually outside the content
 * box entirely and posAtCoords returns null there.
 *
 * @param {Array} blocks output of topLevelBlocks
 * @param {number} y clientY
 * @param {number} [tolerance] px of slack outside the block box
 * @returns {object|null} the block plus its rect, or null if none is near
 */
export function blockAtClientY(blocks, y, tolerance = HANDLE_TOLERANCE) {
  let best = null
  let bestDist = Infinity
  for (const b of blocks) {
    const rect = b.dom?.getBoundingClientRect?.()
    if (!rect) continue
    if (y >= rect.top && y <= rect.bottom) return { ...b, rect }
    const dist = y < rect.top ? rect.top - y : y - rect.bottom
    if (dist < bestDist) {
      bestDist = dist
      best = { ...b, rect }
    }
  }
  return bestDist <= tolerance ? best : null
}

/**
 * Hands a block to ProseMirror's own drag machinery.
 *
 * Setting `view.dragging` is the whole trick: from there the built-in drop
 * handler does the move (delete at the source, insert at the drop point) and
 * the dropcursor from StarterKit draws the insertion line. Reimplementing that
 * by hand is how drag handles end up dropping blocks into their own children.
 *
 * @param {object} view ProseMirror editor view
 * @param {number} pos start position of the block
 * @param {DragEvent} event the dragstart event
 * @returns {boolean} false when the block cannot be dragged
 */
export function startBlockDrag(view, pos, event) {
  const { state } = view
  if (pos < 0 || pos >= state.doc.content.size) return false
  const node = state.doc.nodeAt(pos)
  if (!node || !NodeSelection.isSelectable(node)) return false

  const selection = NodeSelection.create(state.doc, pos)
  view.dispatch(state.tr.setSelection(selection))

  const slice = selection.content()
  view.dragging = { slice, move: true }

  const dt = event.dataTransfer
  if (dt) {
    dt.effectAllowed = 'move'
    // Firefox refuses to start a drag whose dataTransfer carries no payload.
    dt.setData?.('text/plain', node.textBetween(0, node.content.size, '\n'))
    const dom = view.nodeDOM(pos)
    if (dom?.nodeType === 1) dt.setDragImage?.(dom, 0, 0)
  }
  return true
}

/**
 * Clears the drag state after a cancelled drag. ProseMirror clears it itself on
 * a real drop, but a drag released outside the editor never reaches that path
 * and would leave the next click pasting the stale slice.
 * @param {object} view ProseMirror editor view
 */
export function endBlockDrag(view) {
  if (view) view.dragging = null
}
