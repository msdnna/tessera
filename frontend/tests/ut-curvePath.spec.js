import { describe, it, expect } from 'vitest'
import { sCurvePath } from '@/utils/curvePath'

// Numbers out of an SVG path string, so the assertions talk about geometry
// rather than about formatting.
function nums(d) {
  return (d.match(/-?\d+(\.\d+)?/g) || []).map(Number)
}

describe('sCurvePath', () => {
  it('starts and ends exactly at the points it was given', () => {
    const d = sCurvePath(10, 20, 200, 80)
    const n = nums(d)
    expect([n[0], n[1]]).toEqual([10, 20])
    expect([n[6], n[7]]).toEqual([200, 80])
  })

  it('keeps a horizontal stub when the two points share a vertical', () => {
    // No horizontal gap means the ratio contributes nothing; without the floor
    // the curve would collapse to a straight line and lose its flat ends.
    const d = sCurvePath(50, 10, 50, 90)
    const n = nums(d)
    expect(n[2]).toBe(50 + 22)
    expect(n[4]).toBe(50 - 22)
  })

  it('grows the stub with the horizontal gap', () => {
    const wide = nums(sCurvePath(0, 0, 1000, 0))
    expect(wide[2]).toBe(400)
  })

  it('produces no NaN when both ends coincide', () => {
    const d = sCurvePath(7, 7, 7, 7)
    expect(d).not.toContain('NaN')
    expect(nums(d)).toHaveLength(8)
  })

  it('returns an empty path rather than a broken one on bad input', () => {
    expect(sCurvePath(NaN, 0, 10, 10)).toBe('')
    expect(sCurvePath(0, 0, undefined, 10)).toBe('')
    expect(sCurvePath(0, Infinity, 10, 10)).toBe('')
  })
})
