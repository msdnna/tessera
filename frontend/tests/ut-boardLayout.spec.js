// Kanban column width (#2932).
//
// The regression this pins: the «+ колонка» tile is not drawn in the archive
// (cx-archive-no-create.spec.js pins that half — a read-only view must not create a
// live column), but colWidth went on reserving its 220px + a gap there anyway. The
// board therefore divided 232px less than it had among the archived columns: each
// one came out narrower than it could be and a dead band sat along the right edge.
// The reservation now comes from the same flag the tile's v-if reads.
import { describe, it, expect } from 'vitest'
import { columnWidth, BOARD_GAP, ADD_COL_W, STRIP_W } from '@/utils/boardLayout'

const MEDIUM = { min: 220, cap: 420 }
const base = (o) => columnWidth({ ...MEDIUM, ...o })

describe('columnWidth', () => {
  it('reserves the «+ колонка» tile when it is drawn', () => {
    // 1120 − (220 + 12) − 3·12 − 4 = 848 → ⌊848/4⌋ = 212
    expect(base({ containerWidth: 1120, count: 4, addTile: true })).toBe(212)
  })

  it('hands the tile width back to the columns when it is not drawn (archive)', () => {
    // 1120 − 3·12 − 4 = 1080 → ⌊1080/4⌋ = 270
    expect(base({ containerWidth: 1120, count: 4, addTile: false })).toBe(270)
  })

  it('leaves no dead band on the right in the archive', () => {
    const cw = 1120
    const n = 4
    const w = base({ containerWidth: cw, count: n, addTile: false })
    const used = n * w + (n - 1) * BOARD_GAP
    expect(used).toBeLessThanOrEqual(cw)
    // The whole point: what is left over is slack, not a missing tile's worth.
    expect(cw - used).toBeLessThan(ADD_COL_W)
  })

  it('never overruns the row when the tile IS drawn', () => {
    const cw = 1120
    const n = 4
    const w = base({ containerWidth: cw, count: n, addTile: true })
    expect(n * w + (n - 1) * BOARD_GAP + BOARD_GAP + ADD_COL_W).toBeLessThanOrEqual(cw)
  })

  it('reserves a strip per collapsed column and divides the rest among the rest', () => {
    // 1120 − 3·12 − 4 − 2·44 = 992 → ⌊992/2⌋ = 420 → capped at 420
    expect(base({ containerWidth: 1120, count: 4, collapsed: 2, addTile: false })).toBe(420)
    // A wider band shows the division itself rather than the cap.
    expect(
      columnWidth({
        containerWidth: 1120,
        count: 4,
        collapsed: 2,
        addTile: false,
        min: 300,
        cap: 560,
      }),
    ).toBe(496)
    expect(STRIP_W).toBe(44)
  })

  it('does not divide by zero when every column is collapsed', () => {
    const w = base({ containerWidth: 1120, count: 3, collapsed: 3, addTile: false })
    expect(Number.isFinite(w)).toBe(true)
    expect(w).toBeGreaterThan(0)
  })

  it('clamps to the card size band', () => {
    // Wide and nearly empty → the cap, not the whole viewport.
    expect(base({ containerWidth: 2000, count: 2, addTile: false })).toBe(420)
    // Narrow and crowded → the min, and the board scrolls.
    expect(base({ containerWidth: 600, count: 6, addTile: false })).toBe(220)
  })

  it('keeps a fitting width within a gap of the min rather than raising a scrollbar', () => {
    // ⌊(1000 − 5·12 − 4)/6⌋ = 156 for the compact band (min 160): within a gap, so
    // the fitting width is kept instead of clamping up and overrunning the row.
    expect(
      columnWidth({ containerWidth: 1000, count: 6, addTile: false, min: 160, cap: 230 }),
    ).toBe(156)
  })

  it('gives a phone one column with the next one peeking', () => {
    expect(base({ containerWidth: 390, isMobile: true, count: 4, addTile: true })).toBe(362)
    expect(base({ containerWidth: 150, isMobile: true, count: 4, addTile: true })).toBe(200)
  })

  it('falls back to a fixed width before the container is measured', () => {
    expect(base({ containerWidth: 0, count: 4, addTile: true })).toBe(280)
  })
})
