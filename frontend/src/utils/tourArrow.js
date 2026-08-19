// Geometry of the curved gradient arrow that "draws" itself from a popover to
// the element it points at. Extracted from SidebarSpotlight.vue (#2749) so the
// Get Started tour (#2753) can reuse the exact same look on any DOM node.

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
export function layoutArrow(pathEl, from, to, head = ARROW_HEAD) {
  pathEl.setAttribute('d', arcPath(from, to))
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
