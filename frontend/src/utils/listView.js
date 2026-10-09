// The «Список» view (v2, #3020): the pure half — row geometry for the windowed
// rendering, the keyboard cursor and its multi-selection, the neighbours of a
// ⌘↑/↓ step. Kept out of BoardListView so it stays unit-testable.
//
// The list is a column of groups (the board's lanes: status, tag or stage).
// Each group is a header and, unless folded, a tray of equal-height rows — so
// the geometry is exact arithmetic, and only the rows near the viewport are
// mounted (a board of 500+ tasks scrolls like a short one).

// A spot the keyboard cursor can stand on: a group header (`id: null`) or a row
// of that group. With tag grouping one task sits in several groups, hence the
// group key on every spot.
export const spotKey = (s) => (s ? `${s.g}::${s.id ?? ''}` : '')

// Geometry of the whole list: for every group its header top, its tray (top and
// height) and the top of each row inside the tray (relative to the tray).
//   groups   — [{ key }] in display order
//   lists    — { [key]: task[] }
//   folded   — (key) => boolean
//   extra    — (key) => px of an extra slot at the end of the tray (the inline
//              «new task» row), 0 when none
//   m        — metrics: { head, headGap, row, gap, pad, empty }
export function listLayout({ groups, lists, folded, extra = () => 0, m }) {
  let y = 0
  const blocks = []
  for (const g of groups) {
    const rows = lists[g.key] || []
    const headTop = y + m.headGap
    y = headTop + m.head
    const isFolded = folded(g.key)
    let tray = null
    if (!isFolded) {
      const add = extra(g.key)
      const n = rows.length
      const body = n ? n * m.row + (n - 1) * m.gap : add ? 0 : m.empty
      const h = m.pad * 2 + body + (add ? (n ? m.gap : 0) + add : 0)
      tray = { top: y, height: h, addTop: m.pad + (n ? n * (m.row + m.gap) : 0) }
      y += h
    }
    blocks.push({ key: g.key, headTop, folded: isFolded, tray, count: rows.length })
  }
  return { blocks, height: y }
}

// The rows of one tray that intersect [lo, hi] (absolute px), as an index range.
export function trayWindow(block, lo, hi, m) {
  if (!block.tray || !block.count) return [0, 0]
  const step = m.row + m.gap
  const start = block.tray.top + m.pad
  const i0 = Math.max(0, Math.floor((lo - start) / step))
  const i1 = Math.min(block.count, Math.ceil((hi - start) / step) + 1)
  return i0 < i1 ? [i0, i1] : [0, 0]
}

// Absolute top of a spot (header or row), for keeping the cursor in view.
export function spotTop(layout, spot, lists, m) {
  const b = layout.blocks.find((x) => x.key === spot?.g)
  if (!b) return null
  if (spot.id == null || !b.tray) return { top: b.headTop, height: m.head }
  const i = (lists[b.key] || []).findIndex((t) => t.id === spot.id)
  if (i < 0) return { top: b.headTop, height: m.head }
  return { top: b.tray.top + m.pad + i * (m.row + m.gap), height: m.row }
}

// Every spot in reading order: each header, then the rows of an open group.
export function navOrder(groups, lists, folded) {
  const out = []
  for (const g of groups) {
    out.push({ g: g.key, id: null })
    if (folded(g.key)) continue
    for (const t of lists[g.key] || []) out.push({ g: g.key, id: t.id })
  }
  return out
}

// One step of the cursor. No cursor (or a stale one) → the first row of the
// list, else the first header.
export function stepCursor(order, cursor, dir) {
  if (!order.length) return null
  const i = cursor ? order.findIndex((s) => spotKey(s) === spotKey(cursor)) : -1
  if (i < 0) return order.find((s) => s.id != null) || order[0]
  const j = Math.max(0, Math.min(order.length - 1, i + dir))
  return order[j]
}

// The rows between two spots, inclusive — ⇧↑/↓ and ⇧-click grow the selection
// from the anchor to the cursor. Headers are not selectable.
export function rangeBetween(order, a, b) {
  const i = order.findIndex((s) => spotKey(s) === spotKey(a))
  const j = order.findIndex((s) => spotKey(s) === spotKey(b))
  if (i < 0 || j < 0) return b && b.id != null ? [b] : []
  const [lo, hi] = i < j ? [i, j] : [j, i]
  return order.slice(lo, hi + 1).filter((s) => s.id != null)
}

// Where a ⌘↑/↓ step puts a row inside its own list: the API's neighbours
// (`beforeId` = the row that ends up above, `afterId` = below), or null at the
// end of the list.
export function reorderNeighbours(list, id, dir) {
  const i = list.findIndex((t) => t.id === id)
  if (i < 0) return null
  if (dir < 0) {
    if (i === 0) return null
    return { beforeId: list[i - 2]?.id ?? null, afterId: list[i - 1].id }
  }
  if (i === list.length - 1) return null
  return { beforeId: list[i + 1].id, afterId: list[i + 2]?.id ?? null }
}

// How many tag pills fit a tags cell of `width` px before «+N» takes over:
// at least one, about 80px each.
export function tagsThatFit(width, count) {
  if (!count) return 0
  const fit = Math.max(1, Math.floor((width - 22) / 80))
  return Math.min(count, fit)
}
