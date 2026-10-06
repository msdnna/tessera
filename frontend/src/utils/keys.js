// Keyboard helpers shared by shortcut hints and handlers (v2, #2951).

// Apple keyboards print ⌘ where everyone else uses Ctrl. Read once: the
// platform does not change under a running page.
const platform = (
  (typeof navigator !== 'undefined' &&
    (navigator.userAgentData?.platform || navigator.platform || navigator.userAgent)) ||
  ''
).toLowerCase()

export const isMac = /mac|iphone|ipad|ipod/.test(platform)

// The label of the primary modifier as printed on the key: «⌘» or «Ctrl».
export const modKeyLabel = isMac ? '⌘' : 'Ctrl'

// True for the platform's primary modifier (⌘ on Apple, Ctrl elsewhere).
export function hasModKey(e) {
  return isMac ? e.metaKey : e.ctrlKey
}

// True when a keystroke lands in a field that owns its own typing, so a global
// single-key shortcut must leave it alone.
export function isTypingTarget(el) {
  if (!el) return false
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable
}
