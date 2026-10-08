// Chord parsing, matching and labelling for the shortcut layer (v2, #2956).
//
// A binding is written as a space-separated sequence of chords: `'g b'` is «G,
// then B», `'shift+n'` one chord with a modifier, `'p digit'` «P, then any
// digit» (the digit is handed to the handler). `mod` is the platform's primary
// modifier — ⌘ on Apple, Ctrl elsewhere.
//
// Letters and digits are matched by physical key (`e.code`), not by the
// character the layout produced: with the Russian layout on, G types «п», and
// a shortcut that silently stops working when the user is mid-sentence in
// Russian is worse than none. Punctuation that only exists as a character
// (`?`) is written as such and matched by `e.key`.
import { isMac, hasModKey } from '@/utils/keys'

const NAMED_CODES = {
  Slash: '/',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Comma: ',',
  Period: '.',
  Minus: '-',
  Equal: '=',
  Semicolon: ';',
  Quote: "'",
  Backquote: '`',
  Space: 'space',
  Enter: 'enter',
  NumpadEnter: 'enter',
  Escape: 'escape',
  Tab: 'tab',
  Backspace: 'backspace',
  Delete: 'delete',
  ArrowUp: 'arrowup',
  ArrowDown: 'arrowdown',
  ArrowLeft: 'arrowleft',
  ArrowRight: 'arrowright',
}

// Characters typed with Shift on most layouts; written bare in a binding.
const CHAR_KEYS = new Set(['?', '#', '@', '>'])

// keyOf names the physical key of a keyboard event: 'g', '7', 'arrowup', '[',
// 'f2'. Null for keys a binding never names (bare modifiers, media keys).
// Function keys arrived with the v2 interaction model (#2969): F2 renames,
// F6 jumps to the undo toast, ⇧F10 opens a row's menu.
export function keyOf(e) {
  const code = e.code || ''
  if (/^Key[A-Z]$/.test(code)) return code.slice(3).toLowerCase()
  if (/^F([1-9]|1[0-2])$/.test(code)) return code.toLowerCase()
  const digit = /^(?:Digit|Numpad)(\d)$/.exec(code)
  if (digit) return digit[1]
  if (NAMED_CODES[code]) return NAMED_CODES[code]
  // Synthetic events (tests, some IMEs) may carry no code: fall back to key.
  const k = (e.key || '').toLowerCase()
  if (/^[a-z0-9]$/.test(k) || /^f([1-9]|1[0-2])$/.test(k)) return k
  return Object.values(NAMED_CODES).includes(k) ? k : null
}

export function parseChord(text) {
  const parts = text.toLowerCase().split('+')
  const key = parts.pop()
  const chord = { key, mod: false, shift: false, alt: false }
  for (const m of parts) chord[m] = true
  if (CHAR_KEYS.has(key)) chord.char = key
  return chord
}

// Palette-only commands (`form.*`, `go.milestones`) have no `keys`: they parse
// to no chords, so a hint renders nothing and the dispatcher never matches them.
export function parseKeys(keys) {
  const text = typeof keys === 'string' ? keys.trim() : ''
  return text ? text.split(/\s+/).map(parseChord) : []
}

// matchChord tests one event against one chord. Returns null on a miss, or
// `{ arg }` on a hit — `arg` is the digit for a `digit` chord, else undefined.
export function matchChord(chord, e) {
  const otherMod = isMac ? e.ctrlKey : e.metaKey
  if (otherMod) return null
  if (chord.char) {
    // `?` is Shift+/ on a US layout and Shift+7 on a Russian one: trust the
    // character, not the key, and let Shift be whatever produced it.
    if (e.key !== chord.char || hasModKey(e) || e.altKey) return null
    return {}
  }
  if (hasModKey(e) !== chord.mod || e.shiftKey !== chord.shift || e.altKey !== chord.alt) {
    return null
  }
  const key = keyOf(e)
  if (!key) return null
  if (chord.key === 'digit') return /^\d$/.test(key) ? { arg: Number(key) } : null
  return key === chord.key ? {} : null
}

const KEY_CAPS = {
  arrowup: '↑',
  arrowdown: '↓',
  arrowleft: '←',
  arrowright: '→',
  enter: '↵',
  escape: 'Esc',
  tab: 'Tab',
  backspace: '⌫',
  delete: 'Del',
  space: 'Space',
}

// capsOf turns a chord into the labels of its key caps, modifiers first:
// 'mod+shift+c' → ['⌘', '⇧', 'C']. `digitLabel` names the digit range a
// `digit` chord accepts («0–4»).
export function capsOf(chord, digitLabel = '0–9') {
  const caps = []
  if (chord.mod) caps.push(isMac ? '⌘' : 'Ctrl')
  if (chord.alt) caps.push(isMac ? '⌥' : 'Alt')
  if (chord.shift) caps.push('⇧')
  const k = chord.key
  if (k === 'digit') caps.push(digitLabel)
  else if (KEY_CAPS[k]) caps.push(KEY_CAPS[k])
  else caps.push(k.length === 1 || /^f\d+$/.test(k) ? k.toUpperCase() : k)
  return caps
}

// capsOfDisplay labels a hand-written display chord (a list of raw tokens such
// as ['mod', '←/→']) — for entries that stand for several bindings at once.
export function capsOfDisplay(tokens) {
  return tokens.map((t) => {
    if (t === 'mod') return isMac ? '⌘' : 'Ctrl'
    if (t === 'shift') return '⇧'
    if (t === 'alt') return isMac ? '⌥' : 'Alt'
    return KEY_CAPS[t] || t
  })
}
