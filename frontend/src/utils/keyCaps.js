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
}
