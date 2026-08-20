import { describe, it, expect } from 'vitest'
import { unionRect, groupIsHorizontal, choosePlacement } from '@/utils/tourArrow'

// Pure placement logic for the Get Started popover (#2753 rework). These lock in
// the side/gap decisions the author called out: a row of targets gets the
// popover below (with a real gap so the arrow is a line), a column of wide rows
// gets it to the side (not shoved underneath), and it never lands on a control
// the user must press (an open picker, a modal's «Создать» button).

const r = (left, top, w, h) => ({
  left,
  top,
  width: w,
  height: h,
  right: left + w,
  bottom: top + h,
})
const OPTS = { popW: 260, popH: 140, vw: 1440, vh: 900, gaps: [92, 60, 32], edge: 16 }
const place = (anchors, avoid = anchors) =>
  choosePlacement(unionRect(anchors), anchors, avoid, OPTS)

describe('tour popover placement', () => {
  it('reads a row of pills as horizontal and a column of rows as vertical', () => {
    const pills = [r(60, 150, 22, 20), r(90, 150, 22, 20), r(120, 150, 22, 20), r(150, 150, 22, 20)]
    const rows = [r(20, 100, 200, 36), r(20, 150, 200, 36), r(20, 200, 200, 36)]
    expect(groupIsHorizontal(pills, unionRect(pills))).toBe(true)
    expect(groupIsHorizontal(rows, unionRect(rows))).toBe(false)
  })

  it('puts the popover below a horizontal row of pills, a full gap away', () => {
    const pills = [r(60, 150, 22, 20), r(90, 150, 22, 20), r(120, 150, 22, 20), r(150, 150, 22, 20)]
    const p = place(pills)
    expect(p.side).toBe('bottom')
    // Preferred gap kept → the arrow is a real line, not a stub.
    expect(p.top).toBe(unionRect(pills).bottom + 92)
  })

  it('puts the popover to the side of a vertical column of wide rows (nav links)', () => {
    // The regression: a wide union made these read as "horizontal" and the
    // popover was shoved under the tree. It must sit beside them instead.
    const rows = [r(20, 100, 200, 36), r(20, 150, 200, 36), r(20, 200, 200, 36)]
    const p = place(rows)
    expect(p.side).toBe('right')
    expect(p.left).toBe(unionRect(rows).right + 92)
  })

  it('does not cover a «Создать»-style control sitting below the fields', () => {
    // project-create: name field + slug are anchors, the submit button is a
    // `cut` rect right under them. Popover must clear it.
    const name = r(500, 120, 340, 34)
    const slug = r(500, 170, 200, 22)
    const submit = r(760, 250, 90, 34) // the «Создать» button
    const p = place([name, slug], [name, slug, submit])
    const box = { left: p.left, top: p.top, right: p.left + OPTS.popW, bottom: p.top + OPTS.popH }
    const hits = (a, b) =>
      a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
    expect(hits(box, submit)).toBe(false)
    expect(hits(box, name)).toBe(false)
  })

  it('keeps clear of an open picker below the field (calendar)', () => {
    const field = r(120, 140, 300, 30)
    const calendar = r(120, 180, 300, 260) // opens under the field
    const p = place([field], [field, calendar])
    expect(p.side).not.toBe('bottom')
    const box = { left: p.left, top: p.top, right: p.left + OPTS.popW, bottom: p.top + OPTS.popH }
    const hits = (a, b) =>
      a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
    expect(hits(box, calendar)).toBe(false)
  })

  it('always returns a box inside the viewport, even when nothing clears', () => {
    // A target hugging every edge with avoids all around → fallback clamps in.
    const a = r(0, 0, 1440, 900)
    const p = place([a], [a])
    expect(p.left).toBeGreaterThanOrEqual(OPTS.edge)
    expect(p.top).toBeGreaterThanOrEqual(OPTS.edge)
    expect(p.left + OPTS.popW).toBeLessThanOrEqual(OPTS.vw - OPTS.edge)
    expect(p.top + OPTS.popH).toBeLessThanOrEqual(OPTS.vh - OPTS.edge)
  })
})
