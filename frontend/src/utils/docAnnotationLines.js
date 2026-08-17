// Geometry for the lines tying a block to its annotation card (#2730).
//
// Not to be confused with DocLinks/useDocLinks, which are the document↔task
// links of #2732 — these are the drawn curves, that is a data relationship.
//
// Kept out of the view because this is the part that can be wrong on its own:
// which links exist, which are dropped, which one is highlighted. DOM geometry
// cannot be exercised in a unit test — the rules applied to it can.

import { sCurvePath } from '@/utils/curvePath'

/**
 * Builds the drawable links between block anchors and thread cards.
 *
 * A link is dropped, rather than clamped to the edge, whenever either end is
 * outside its scroll box: a curve running to a point the user cannot see is an
 * arrow into nowhere, and it is worse than no line at all because it still
 * claims to point at something.
 *
 * The order of `cards` is the order they are painted in the panel, and the
 * caller sorts that by document position. Two monotonic sequences cannot cross,
 * so ordering the panel is what keeps the lines untangled — no per-card layout
 * pass is needed for that, only the sort.
 *
 * A settled thread keeps its card but loses its line, for the same reason it
 * loses the underline and the margin counter: once the remark is answered it
 * should stop marking up the text. Leaving the curve behind would have the panel
 * still pointing at a paragraph the editor no longer flags.
 *
 * @param {object} args
 * @param {Array<{id: string, x: number, y: number, visible?: boolean}>} args.blocks
 *   block anchors, keyed by block id
 * @param {Array<{id: string, blockId: string, resolved?: boolean, x: number, y: number,
 *   visible?: boolean}>} args.cards thread cards in panel order
 * @param {string} [args.activeBlockId] the highlighted block, if any
 * @returns {Array<{id: string, blockId: string, d: string, active: boolean}>}
 */
export function linkGeometry({ blocks = [], cards = [], activeBlockId = '' } = {}) {
  const byId = new Map()
  for (const b of blocks || []) {
    if (b && b.id && !byId.has(b.id)) byId.set(b.id, b)
  }
  const out = []
  for (const c of cards || []) {
    if (!c || !c.blockId || c.visible === false || c.resolved) continue
    const b = byId.get(c.blockId)
    if (!b || b.visible === false) continue
    const d = sCurvePath(b.x, b.y, c.x, c.y)
    if (!d) continue
    out.push({
      id: c.id,
      blockId: c.blockId,
      d,
      active: !!activeBlockId && c.blockId === activeBlockId,
    })
  }
  return out
}
