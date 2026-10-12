// When does a click on a read description open its editor (#3113)?
//
// The v2 description is prose that turns into the editor on a click — but the
// prose is not inert: links open, checkboxes tick, @-mentions show a card, #N
// chips open the task, pictures and diagrams are looked at, a spoiler unfolds.
// A click on any of those does its own thing and must not also swap the text
// out from under the pointer. Neither does a click that ends a text selection:
// the reader was copying, not editing.

const LIVE = [
  'a',
  'button',
  'input',
  'label',
  'summary',
  'img',
  'video',
  'svg',
  '.mention',
  '[data-type="mention"]',
  '[data-task-ref]',
  '.mermaid-diagram',
  '.mermaid-error',
].join(', ')

export function isEditClick(
  e,
  sel = typeof window !== 'undefined' ? window.getSelection?.() : null,
) {
  if (!e || e.defaultPrevented || e.button > 0) return false
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return false
  const el = e.target instanceof Element ? e.target : e.target?.parentElement
  if (!el || el.closest(LIVE)) return false
  if (sel && !sel.isCollapsed && String(sel).trim()) return false
  return true
}
