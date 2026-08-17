// The connector curve shared by every "this links to that" line in the app.
//
// Gantt dependency arrows (#2592) and document annotation links (#2730) draw the
// same kind of thing: a line between two points that leaves its source and
// enters its target horizontally, so the ends read as attached rather than
// stabbing in at an angle. They share the function rather than each carrying a
// copy of the formula — two lookalike curves drift apart at the first tweak, and
// then the same relationship looks like two different ones depending on which
// screen you are on.

/**
 * An S-curve from (x1,y1) to (x2,y2) with a horizontal stub at each end.
 *
 * The stub grows with the horizontal gap so long links bow generously and short
 * ones stay tight, but never shrinks below `minStub` — without a floor, two
 * points on the same vertical would join with a straight diagonal and lose the
 * flat entry that makes the ends look anchored.
 *
 * @param {number} x1 source x
 * @param {number} y1 source y
 * @param {number} x2 target x
 * @param {number} y2 target y
 * @param {{minStub?: number, ratio?: number}} [opts] stub floor and gap ratio
 * @returns {string} an SVG path, or '' when a coordinate is not a finite number
 */
export function sCurvePath(x1, y1, x2, y2, { minStub = 22, ratio = 0.4 } = {}) {
  // A NaN anywhere produces a path string the browser silently refuses to draw,
  // which debugs as "the line is missing" rather than "the input was garbage".
  // Better to hand back nothing and let the caller skip the line.
  if (![x1, y1, x2, y2].every((n) => Number.isFinite(n))) return ''
  const dx = Math.max(minStub, Math.abs(x2 - x1) * ratio)
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
}
