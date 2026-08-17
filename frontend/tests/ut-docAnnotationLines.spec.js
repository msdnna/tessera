import { describe, it, expect } from 'vitest'
import { linkGeometry } from '@/utils/docAnnotationLines'

const block = (id, y, extra = {}) => ({ id, x: 100, y, visible: true, ...extra })
const card = (id, blockId, y, extra = {}) => ({ id, blockId, x: 300, y, visible: true, ...extra })

function ends(d) {
  const n = (d.match(/-?\d+(\.\d+)?/g) || []).map(Number)
  return { y1: n[1], y2: n[7] }
}

describe('linkGeometry', () => {
  it('links each card to the block it points at', () => {
    const out = linkGeometry({
      blocks: [block('b1', 10), block('b2', 60)],
      cards: [card('t1', 'b1', 20), card('t2', 'b2', 90)],
    })
    expect(out.map((l) => l.id)).toEqual(['t1', 't2'])
    expect(out.every((l) => l.d.startsWith('M '))).toBe(true)
  })

  it('skips a card whose block is not in the document', () => {
    // Detached threads live in their own section of the panel; there is nothing
    // in the text to draw a line to.
    const out = linkGeometry({ blocks: [block('b1', 10)], cards: [card('t1', 'gone', 20)] })
    expect(out).toEqual([])
  })

  it('skips a thread about the document as a whole', () => {
    const out = linkGeometry({ blocks: [block('b1', 10)], cards: [card('t1', '', 20)] })
    expect(out).toEqual([])
  })

  it('draws nothing when either end is scrolled out of view', () => {
    // A curve to a point behind the edge of a scroll box is an arrow into
    // nowhere — worse than no line, because it still claims to point at something.
    expect(
      linkGeometry({
        blocks: [block('b1', 10, { visible: false })],
        cards: [card('t1', 'b1', 20)],
      }),
    ).toEqual([])
    expect(
      linkGeometry({
        blocks: [block('b1', 10)],
        cards: [card('t1', 'b1', 20, { visible: false })],
      }),
    ).toEqual([])
  })

  it('drops the line of a settled thread, which keeps its card', () => {
    // Resolving already takes the underline and the margin counter off the
    // block; a line still pointing there would contradict the editor.
    const out = linkGeometry({
      blocks: [block('b1', 10), block('b2', 60)],
      cards: [card('t1', 'b1', 20, { resolved: true }), card('t2', 'b2', 90)],
    })
    expect(out.map((l) => l.id)).toEqual(['t2'])
  })

  it('marks only the active block’s links', () => {
    const out = linkGeometry({
      blocks: [block('b1', 10), block('b2', 60)],
      cards: [card('t1', 'b1', 20), card('t2', 'b2', 90)],
      activeBlockId: 'b2',
    })
    expect(out.map((l) => l.active)).toEqual([false, true])
  })

  it('does not cross when cards are in document order', () => {
    // This is the whole reason the panel sorts by document position: two
    // monotonic sequences cannot cross, so ordering the cards is the layout.
    const blocks = [block('b1', 10), block('b2', 50), block('b3', 120)]
    const cards = [card('t1', 'b1', 20), card('t2', 'b2', 70), card('t3', 'b3', 150)]
    const out = linkGeometry({ blocks, cards })
    const ys = out.map((l) => ends(l.d))
    for (let i = 1; i < ys.length; i += 1) {
      expect(ys[i].y1).toBeGreaterThan(ys[i - 1].y1)
      expect(ys[i].y2).toBeGreaterThan(ys[i - 1].y2)
    }
  })

  it('gives two threads on one block a line each', () => {
    const out = linkGeometry({
      blocks: [block('b1', 10)],
      cards: [card('t1', 'b1', 20), card('t2', 'b1', 60)],
    })
    expect(out).toHaveLength(2)
    expect(new Set(out.map((l) => l.blockId))).toEqual(new Set(['b1']))
  })

  it('survives empty input', () => {
    expect(linkGeometry()).toEqual([])
    expect(linkGeometry({ blocks: [], cards: [] })).toEqual([])
  })
})
