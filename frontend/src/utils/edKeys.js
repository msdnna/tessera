// The Markdown editor's own keys (#3118, PackageJ «Композер» / «Полноэкранный»).
//
// A text field owns its typing, so the global dispatcher leaves these alone; the
// editor matches them itself, against the same catalog entries the cheat sheet
// and the bubble's tooltips are drawn from — the hint can't name one key while
// the field answers to another. Matched by physical key (keymap.matchChord), so
// ⌘B still works with the Russian layout on, and ⌘⇧7 still is «7» although
// Shift turned it into «&» (or «?» in Russian).
import { byId } from '@/shortcuts/catalog'
import { parseKeys, matchChord } from '@/shortcuts/keymap'

// The formatting actions of the field, in the order the bubble shows them.
export const FORMAT_ACTIONS = [
  'bold',
  'italic',
  'strike',
  'code',
  'link',
  'orderedList',
  'bulletList',
]
// The marks the selection bubble carries (a subset: no lists over a selection).
export const BUBBLE_ACTIONS = ['bold', 'italic', 'strike', 'code', 'link']
export const MODE_ACTIONS = ['write', 'split', 'preview']

const chordCache = new Map()
function chordOf(id) {
  if (!chordCache.has(id)) {
    const entry = byId.get(id)
    chordCache.set(id, entry ? parseKeys(entry.keys)[0] || null : null)
  }
  return chordCache.get(id)
}

function firstMatch(actions, e) {
  if (!e || e.isComposing) return null
  for (const a of actions) {
    const chord = chordOf(`editor.${a}`)
    if (chord && matchChord(chord, e)) return a
  }
  return null
}

// formatAction names the formatting a keystroke asks for ('bold', 'link', …),
// or null.
export function formatAction(e) {
  return firstMatch(FORMAT_ACTIONS, e)
}

// modeAction names the fullscreen editor mode a keystroke picks
// ('write' | 'split' | 'preview'), or null.
export function modeAction(e) {
  return firstMatch(MODE_ACTIONS, e)
}

// The catalog id whose caps a tooltip shows for an action.
export const keyIdOf = (action) => `editor.${action}`
