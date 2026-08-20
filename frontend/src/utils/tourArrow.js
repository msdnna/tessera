// Geometry of the curved gradient arrow that "draws" itself from a popover to
// the element it points at. Extracted from SidebarSpotlight.vue (#2749) so the
// Get Started tour (#2753) can reuse the exact same look on any DOM node.

// ── Popover placement (pure, so it can be unit-tested against real layouts) ──

export function unionRect(rs) {
  const f = rs.filter(Boolean)
  if (!f.length) return null
  let l = Infinity,
    t = Infinity,
    r = -Infinity,
    b = -Infinity
  for (const x of f) {
    l = Math.min(l, x.left)
    t = Math.min(t, x.top)
    r = Math.max(r, x.right)
    b = Math.max(b, x.bottom)
  }
  return { left: l, top: t, right: r, bottom: b, width: r - l, height: b - t }
}

function intersects(a, b, m = 0) {
  return (
    a.left < b.right + m && a.right > b.left - m && a.top < b.bottom + m && a.bottom > b.top - m
  )
}
const clampN = (v, lo, hi) => Math.max(lo, Math.min(hi, v))

// How the anchors are arranged, from the spread of their centres — NOT the shape
// of the bounding box. A vertical stack of wide rows (the nav links) has a wide
// union yet is clearly a column, and must get a side popover, not one underneath.
export function groupIsHorizontal(anchors, u) {
  if (anchors.length < 2) return u.width >= u.height
  const cx = anchors.map((r) => r.left + r.width / 2)
  const cy = anchors.map((r) => r.top + r.height / 2)
  return Math.max(...cx) - Math.min(...cx) >= Math.max(...cy) - Math.min(...cy)
}

/**
 * Choose where the popover sits relative to the group of anchors.
 *
 * A horizontal row of targets is served from below/above, a column from the
 * side; within that axis we take the first side, down the gap ladder, that fits
 * the viewport AND clears every `avoid` rect (anchors, open pickers, `cut`
 * controls) — so the popover never covers something the user must press. Only if
 * nothing clears at any distance do we fall back to the roomiest side, clamped
 * in. The generous default gap keeps the arrow a real line, not a stub.
 *
 * @returns {{ left, top, side }}
 */
export function choosePlacement(
  u,
  anchors,
  avoid,
  { popW, popH, vw, vh, gaps, edge, panels = [] },
) {
  let order = groupIsHorizontal(anchors, u)
    ? ['bottom', 'top', 'right', 'left']
    : ['right', 'left', 'bottom', 'top']

  // An open picker (calendar/tags dropdown) always opens on one side of its
  // field; the popover must not sit on that side even if their boxes don't quite
  // touch, or it lands between the field and its own dropdown (#2753 rework). Ban
  // the side each panel juts out on (by which edge it clears the union furthest),
  // keeping at least one candidate.
  if (panels.length) {
    const ucx = (u.left + u.right) / 2
    const ucy = (u.top + u.bottom) / 2
    const banned = new Set()
    for (const p of panels) {
      const off = {
        bottom: p.top - u.bottom,
        top: u.top - p.bottom,
        right: p.left - u.right,
        left: u.left - p.right,
      }
      let side = ['bottom', 'top', 'right', 'left'].reduce(
        (a, b) => (off[b] > off[a] ? b : a),
        'bottom',
      )
      // Fully overlapping the union (nothing juts out) → fall back to whichever
      // way the panel's centre leans.
      if (off[side] <= 0) {
        const dx = (p.left + p.right) / 2 - ucx
        const dy = (p.top + p.bottom) / 2 - ucy
        side =
          Math.abs(dy) >= Math.abs(dx) ? (dy > 0 ? 'bottom' : 'top') : dx > 0 ? 'right' : 'left'
      }
      banned.add(side)
    }
    const kept = order.filter((s) => !banned.has(s))
    if (kept.length) order = kept
  }

  const cand = (side, gap) => {
    if (side === 'bottom' || side === 'top') {
      const left = clampN(u.left + u.width / 2 - popW / 2, edge, vw - popW - edge)
      const top = side === 'bottom' ? u.bottom + gap : u.top - gap - popH
      return { left, top, side }
    }
    const top = clampN(u.top + u.height / 2 - popH / 2, edge, vh - popH - edge)
    const left = side === 'right' ? u.right + gap : u.left - gap - popW
    return { left, top, side }
  }
  const inViewport = (c) =>
    c.left >= edge && c.top >= edge && c.left + popW <= vw - edge && c.top + popH <= vh - edge
  const clearsAvoid = (c) => {
    const box = { left: c.left, top: c.top, right: c.left + popW, bottom: c.top + popH }
    return !avoid.some((a) => intersects(box, a, 8))
  }

  for (const gap of gaps) {
    for (const side of order) {
      const c = cand(side, gap)
      if (inViewport(c) && clearsAvoid(c)) return c
    }
  }
  const space = { bottom: vh - u.bottom, top: u.top, right: vw - u.right, left: u.left }
  const side = order.reduce((a, b) => (space[b] > space[a] ? b : a), order[0])
  const c = cand(side, gaps[gaps.length - 1])
  return {
    left: clampN(c.left, edge, vw - popW - edge),
    top: clampN(c.top, edge, vh - popH - edge),
    side,
  }
}

// px of the tip left undrawn by the stroke: the arrowhead triangle spans that
// gap, so the round line-cap never pokes through the point (the "blunted" look).
export const ARROW_HEAD = 11
const HALF_W = 5
const MAX_BOW = 20

// Cubic bezier from `from` to `to` with a shallow bow. The control points stay
// close to the endpoints so the arc reads as a tight hook across a narrow gap
// rather than a wide loop.
export function arcPath(from, to) {
  const dx = from.x - to.x
  const dy = from.y - to.y
  // Bow away from the popover: signed so a popover *above* the target bows the
  // other way instead of inheriting a huge negative radius from Math.min.
  const bow = Math.sign(dy || 1) * Math.min(MAX_BOW, Math.abs(dy) * 0.35)
  const cx1 = from.x - dx * 0.55
  const cy1 = from.y - bow
  const cx2 = to.x + dx * 0.16
  const cy2 = to.y - bow * 0.7
  return `M ${from.x} ${from.y} C ${cx1} ${cy1}, ${cx2} ${cy2}, ${to.x} ${to.y}`
}

// Bowed cubic that always curves off the straight chord, by an amount that grows
// with length — so even a short, horizontal arrow reads as a proper drawn line
// with a gentle arc rather than a straight stub (#2753 rework). The bow is on a
// consistent perpendicular side, which fans a group of arrows out of one popover
// edge into a tidy sheaf. Used by the Get Started tour; the What's-New spotlight
// keeps arcPath so its look is unchanged.
export function arcPathBowed(from, to) {
  const dx = to.x - from.x
  const dy = to.y - from.y
  const len = Math.hypot(dx, dy) || 1
  const nx = -dy / len // unit perpendicular to the chord
  const ny = dx / len
  const bow = Math.min(28, len * 0.16)
  const c1x = from.x + dx / 3 + nx * bow
  const c1y = from.y + dy / 3 + ny * bow
  const c2x = from.x + (dx * 2) / 3 + nx * bow
  const c2y = from.y + (dy * 2) / 3 + ny * bow
  return `M ${from.x} ${from.y} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${to.x} ${to.y}`
}

// Hybrid of the two above, keyed on the side the popover sits on: it *leaves* the
// popover with arcPathBowed's gentle perpendicular bow (so the line is a visible
// curve, never a degenerate zero-width vertical), but *arrives* at the target
// along that side's axis — the end control point sits straight out from the tip —
// so the arrowhead points square at the element instead of skidding in sideways
// when the target is far off to one side (a toolbar item vs a centred popover,
// #2753 rework).
export function tourArc(from, to, side) {
  const dx = to.x - from.x
  const dy = to.y - from.y
  const len = Math.hypot(dx, dy) || 1
  const nx = -dy / len // perpendicular to the chord, for the leaving bow
  const ny = dx / len
  const bow = Math.min(28, len * 0.16)
  const c1x = from.x + dx / 3 + nx * bow
  const c1y = from.y + dy / 3 + ny * bow
  const k = Math.min(52, len * 0.36) // how straight the final approach runs
  let c2x, c2y
  if (side === 'bottom') {
    c2x = to.x
    c2y = to.y + k
  } else if (side === 'top') {
    c2x = to.x
    c2y = to.y - k
  } else if (side === 'right') {
    c2x = to.x + k
    c2y = to.y
  } else {
    c2x = to.x - k
    c2y = to.y
  }
  return `M ${from.x} ${from.y} C ${c1x} ${c1y}, ${c2x} ${c2y}, ${to.x} ${to.y}`
}

// Triangle spanning the last `head` px of the path, pointing at `tip`.
export function headPoints(tip, base, halfW = HALF_W) {
  const ang = Math.atan2(tip.y - base.y, tip.x - base.x)
  const nx = Math.cos(ang + Math.PI / 2)
  const ny = Math.sin(ang + Math.PI / 2)
  return `${tip.x},${tip.y} ${base.x + nx * halfW},${base.y + ny * halfW} ${base.x - nx * halfW},${base.y - ny * halfW}`
}

// Lay the arrow out on a live <path>: sets `d` and measures the head. Returns
// the stroke length + head polygon points for the draw animation.
//
// getTotalLength/getPointAtLength are unimplemented in jsdom, so a missing
// measurement degrades to "no head, no dash animation" instead of throwing —
// component tests mount the overlay for real.
export function layoutArrow(pathEl, from, to, head = ARROW_HEAD, arc = arcPath) {
  pathEl.setAttribute('d', arc(from, to))
  let len
  try {
    len = pathEl.getTotalLength()
  } catch {
    return { len: 0, head: '' }
  }
  if (!len || typeof pathEl.getPointAtLength !== 'function') return { len: 0, head: '' }
  const tip = pathEl.getPointAtLength(len)
  const base = pathEl.getPointAtLength(Math.max(0, len - head))
  return { len, head: headPoints(tip, base) }
}
