import { i18n } from '@/i18n'

// Priority labels (index = the stored 0..4 value), localized.
//
// The old `PRIORITY_LABELS` in styles/tokens.js was a module-level array of
// Russian strings — frozen at import time, so a language switch would never have
// reached it. Here the text is produced on every call instead, and since
// `i18n.global.t` reads the locale ref, a computed that calls this re-runs when
// the language changes (pitfall 1 of the #2799 plan).
//
// The colour palette stays in styles/tokens.js: it is a token, not a string.
export const PRIORITY_KEYS = ['none', 'low', 'normal', 'high', 'urgent']

export function priorityLabel(value) {
  const key = PRIORITY_KEYS[Number(value) || 0] || PRIORITY_KEYS[0]
  return i18n.global.t(`common.priority.${key}`)
}

// The whole ladder, ordered by value — for pickers and filter menus that render
// every level ([{ label, value }] is the shape Naive's option lists want).
export function priorityOptions() {
  return PRIORITY_KEYS.map((_, value) => ({ label: priorityLabel(value), value }))
}
