// Key cap labels for hints that aren't catalog shortcuts (form footers, inline
// edit tooltips) — kept out of templates so they read the platform once and
// pass the "no bare strings" lint (#2969).
import { isMac } from '@/utils/keys'

export const CAPS = {
  enter: '↵',
  escape: 'Esc',
  tab: 'Tab',
  arrows: '↑↓',
  altEnter: isMac ? '⌥↵' : 'Alt ↵',
  modEnter: isMac ? '⌘↵' : 'Ctrl ↵',
  backspace: '⌫',
}

// A single letter or digit key cap («B», «1») — the key itself, not a word to
// translate. A function so templates stay free of bare strings.
export const cap = (k) => String(k).toUpperCase()
