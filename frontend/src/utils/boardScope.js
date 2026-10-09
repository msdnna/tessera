// The archive is a *scope* of the board itself (`?archived=1`), not a separate
// page: it keeps every filter, the grouping and the layout, and only swaps which
// cards the server lists. So the control that opens it is a toggle, and the query
// maths behind both halves lives here.
//
// It used to live in three copies, each writing only the half its own component
// needed — and the two that opened the archive never learned to close it:
//
//   * BoardActions (desktop «Архив») renders itself PRESSED while the scope is on
//     (`type="primary"` + `secondary`) and then ignores the click that would
//     un-press it — `isArchive ? null : openArchive()`.
//   * BoardMobileMenu is the phone's only board menu (Topbar renders one or the
//     other, never both), and its «Архив» entry carried no state at all: no tick,
//     while every layout entry in the same dropdown gets one. Selecting it from
//     inside the archive re-pushed the identical query — Vue Router answers a
//     duplicated navigation with a resolved NavigationFailure, so the entry was
//     silently dead exactly where the reader expects it to take them back.
//
// The only way out was the scope chip's «×» down in the board composer, which is
// a different control in a different place from the one that got you in. Both
// menus now draw one row, composables/useBoardActions (#3112).

// Query for entering the archive.
//
// The sprint scope goes with it: `?milestone=` narrows the board to one sprint's
// cards, the archive lists archived ones, and the two together routinely resolve
// to an empty board.
export function archiveQuery(query) {
  const q = { ...query, archived: '1' }
  delete q.milestone
  return q
}

// Query for leaving it again. Only the scope key goes — filters, grouping and the
// rest of the address survive the round trip.
export function withoutArchiveQuery(query) {
  const q = { ...query }
  delete q.archived
  return q
}

// Is the board being *looked at* rather than worked on? The flag is the string
// `'1'`, not any truthy value: `?archived=0` must read as "no".
export function isArchiveQuery(query) {
  return query?.archived === '1'
}
