// Keyboard selection in the Eisenhower matrix (v2, #2973). Pure, so the rules
// are testable without mounting the grid.
//
// The four quadrants sit 2×2 — 0 «Сделать» (top left), 1 «Запланировать» (top
// right), 2 «Делегировать» (bottom left), 3 «Отложить» (bottom right), the
// backend's encoding (migration 0028). Inside a quadrant the cards flow in
// `cols` columns (two for compact cards, one otherwise).
//
// `quads` is [ids of q0, ids of q1, ids of q2, ids of q3]; a selection is
// { q, id }.

// The quadrant one step in `dir` ('left' | 'right' | 'up' | 'down'), or null at
// the edge of the grid.
export function neighbourQuad(q, dir) {
  const row = q >> 1
  const col = q & 1
  if (dir === 'left') return col ? q - 1 : null
  if (dir === 'right') return col ? null : q + 1
  if (dir === 'up') return row ? q - 2 : null
  if (dir === 'down') return row ? null : q + 2
  return null
}

export function firstInMatrix(quads) {
  const q = quads.findIndex((ids) => ids.length > 0)
  return q < 0 ? null : { q, id: quads[q][0] }
}

// One step of the selection. Inside a quadrant the arrows walk its grid; past
// its edge they cross into the neighbouring quadrant on the same row / column
// (an empty one is stepped over by staying put). No selection, or a stale one
// → the first card of the matrix.
export function matrixMove(quads, cols, cur, dir) {
  const ids = cur ? quads[cur.q] || [] : []
  const i = cur ? ids.indexOf(cur.id) : -1
  if (i < 0) return firstInMatrix(quads)
  const n = Math.max(1, cols)
  const row = Math.floor(i / n)
  const col = i % n
  const rows = Math.ceil(ids.length / n)
  const at = (q, j) => ({ q, id: quads[q][Math.max(0, Math.min(quads[q].length - 1, j))] })
  if (dir === 'left' && col > 0) return at(cur.q, i - 1)
  if (dir === 'right' && col < n - 1 && i + 1 < ids.length) return at(cur.q, i + 1)
  if (dir === 'up' && row > 0) return at(cur.q, i - n)
  if (dir === 'down' && row < rows - 1) return at(cur.q, Math.min(ids.length - 1, i + n))
  const q = neighbourQuad(cur.q, dir)
  if (q == null || !quads[q].length) return cur
  const len = quads[q].length
  const theirRows = Math.ceil(len / n)
  if (dir === 'left') return at(q, Math.min(row, theirRows - 1) * n + (n - 1))
  if (dir === 'right') return at(q, Math.min(row, theirRows - 1) * n)
  if (dir === 'up') return at(q, (theirRows - 1) * n + col)
  return at(q, col)
}
