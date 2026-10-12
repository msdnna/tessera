import { byId } from './catalog'
import { parseKeys, capsOf, capsOfDisplay } from './keymap'

// chordsOf labels a catalog entry as a list of chords, each a list of key-cap
// strings: 'g b' → [['G'], ['B']], 'mod+shift+c' → [['⌘', '⇧', 'C']].
export function chordsOf(id) {
  const entry = byId.get(id)
  if (!entry) return []
  if (entry.display) return entry.display.map(capsOfDisplay)
  return parseKeys(entry.keys).map((c) => capsOf(c, entry.digit))
}

// keysText is the same as plain text («G B», «⌘ ⇧ C») — for search in the cheat
// sheet and for places that can't render key caps (native title attributes).
export function keysText(id) {
  return chordsOf(id)
    .map((c) => c.join(''))
    .join(' ')
}
