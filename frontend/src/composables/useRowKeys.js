import { byId } from '@/shortcuts/catalog'
import { matchChord, parseKeys } from '@/shortcuts/keymap'

// Keys of a focused tree row (v2 interaction model, #2969). The row handles
// them itself rather than through the global dispatcher: they mean something
// only while that row has the focus, and the same letters (E, M, R) mean other
// things on the board. The bindings still come from the catalog, so the menu
// hints and the cheat sheet show exactly these keys.
//
//   handleRowKeys(e, { 'row.rename': start, 'row.open': open, … })
// Returns true (and stops the event) when one of them matched.
export function handleRowKeys(e, map) {
  if (e.isComposing) return false
  for (const [id, run] of Object.entries(map)) {
    if (!run) continue
    const entry = byId.get(id)
    if (!entry?.keys) continue
    const chords = parseKeys(entry.keys)
    if (chords.length === 1 && matchChord(chords[0], e)) {
      e.preventDefault()
      e.stopPropagation()
      run(e)
      return true
    }
  }
  return false
}

// Arrow navigation over the visible rows of a tree container: ↑/↓ move the
// focus, Home/End jump to the ends. ←/→ are the row's own (fold/unfold).
export function handleTreeNav(e, container) {
  if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(e.key)) return false
  if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return false
  const rows = [...(container?.querySelectorAll('[data-tree-row]') || [])].filter(
    (el) => el.offsetParent !== null,
  )
  if (!rows.length) return false
  const i = rows.indexOf(document.activeElement)
  let next
  if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = rows.length - 1
  else if (i < 0) next = 0
  else next = Math.min(rows.length - 1, Math.max(0, i + (e.key === 'ArrowDown' ? 1 : -1)))
  e.preventDefault()
  rows[next].focus()
  rows[next].scrollIntoView?.({ block: 'nearest' })
  return true
}
