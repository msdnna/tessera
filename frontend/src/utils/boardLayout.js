// How wide one kanban column is drawn (#2932), kept out of KanbanBoard.vue so the
// arithmetic is unit-testable.
//
// The board lays its columns out itself rather than leaning on flex: the row has to
// fill the viewport when it comfortably can, and start scrolling only once columns
// would get too narrow to hold their fields. That means every fixed thing standing
// in the row — the gaps, the collapsed strips, the «+ колонка» tile — has to be
// subtracted from the container before the rest is divided among the expanded
// columns, and each of those reservations has to match what is actually rendered.
// The tile is the one that drifted: the template stopped drawing it in the archive
// (a read-only view must not create a live column) while this maths kept reserving
// its width there, so archived boards drew narrower columns with a dead band on the
// right. Hence `addTile` is passed in rather than re-derived from the group mode.

export const BOARD_GAP = 12 // gap between columns (keep in sync with .cols)
// Width of the add-column tile («+ колонка»).
export const ADD_COL_W = 220
export const STRIP_W = 44 // collapsed-column strip width (keep in sync with .col.collapsed)
const SLACK = 4 // a few px so sub-pixel rounding can't trip the horizontal scrollbar
const PRE_MEASURE_W = 280 // before the container has been measured
const MOBILE_PEEK = 28 // how much of the next column shows on a phone
const MOBILE_MIN = 200

/**
 * Width in px for one expanded column.
 *
 * @param {Object} o
 * @param {number} o.containerWidth scroller width; 0 before it is measured
 * @param {boolean} [o.isMobile] one column a little under full width (swipe layout)
 * @param {number} [o.count] columns drawn, collapsed ones included
 * @param {number} [o.collapsed] how many of them are collapsed to a strip
 * @param {boolean} [o.addTile] is the «+ колонка» tile drawn in this row
 * @param {number} o.min narrowest the chosen card size may get
 * @param {number} o.cap widest it may get
 */
export function columnWidth({
  containerWidth,
  isMobile = false,
  count = 0,
  collapsed = 0,
  addTile = false,
  min,
  cap,
}) {
  const cw = containerWidth || 0
  if (!cw) return PRE_MEASURE_W
  // Mobile: one column a little under full width so the next one peeks and the
  // swipe (scroll-snap, see CSS) reads as a smooth page-turn.
  if (isMobile) return Math.max(cw - MOBILE_PEEK, MOBILE_MIN)
  // Collapsed columns render as a 44px strip — reserve their fixed width and divide
  // the rest among the EXPANDED columns only, so collapsing a column hands its width
  // to the neighbours instead of leaving a gap on the right.
  const n = count || 1
  const nCollapsed = Math.min(Math.max(collapsed, 0), n)
  const nExpanded = Math.max(n - nCollapsed, 1)
  const reserved =
    (addTile ? ADD_COL_W + BOARD_GAP : 0) + (n - 1) * BOARD_GAP + SLACK + nCollapsed * STRIP_W
  const fit = Math.floor((cw - reserved) / nExpanded)
  // Fill the viewport while comfortable; the width band comes from the chosen card
  // size (compact narrower, large wider), so columns never get too narrow to hold
  // their fields and scroll horizontally once they'd shrink past the band's min.
  //
  // A `fit`-wide column always fits the row (that's how `reserved` is derived);
  // clamping UP to the band min is what can overrun the board by a couple px and
  // raise a full-width horizontal scrollbar for columns that essentially fit. So only
  // clamp up — and let the board scroll — once columns would get *meaningfully*
  // narrower than the min; within a gap's worth of it, keep the fitting width so no
  // stray scrollbar appears when everything is already visible (#2743).
  if (fit > cap) return cap
  if (fit >= min - BOARD_GAP) return fit
  return min
}
