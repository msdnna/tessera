// «Ссылка на фрагмент» (#3119, PackageJ «Пузырь»): a link to a stretch of a task's
// description or of one of its comments. The address is the task's own link plus
// where the text lives and the text itself:
//   /board/<id>?task=<n>&frag=desc&q=<text>
//   /board/<id>?task=<n>&frag=c:<comment id>&q=<text>
// Opening it opens the task, scrolls the description or the comment into view
// and marks the text for a few seconds (the CSS Custom Highlight API — no DOM is
// rewritten, so a re-render can't leave a stray <mark>). Text that is gone (the
// comment was edited) still scrolls to its place.
import { taskLink } from './taskLink'

export const FRAGMENT_MAX = 200
const HIGHLIGHT = 't-fragment'

export function fragmentHref(task, scope, text, origin) {
  const base = taskLink(task, origin)
  if (!base || !scope) return null
  const q = String(text || '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, FRAGMENT_MAX)
  const params = new URLSearchParams({ frag: scope })
  if (q) params.set('q', q)
  return `${base}&${params.toString()}`
}

// The scope a node of the read text belongs to: a comment (its row carries
// data-comment-id) or, outside any comment, the description.
export function scopeOf(node) {
  const el = node?.nodeType === 1 ? node : node?.parentElement
  const c = el?.closest?.('[data-comment-id]')
  return c ? `c:${c.getAttribute('data-comment-id')}` : 'desc'
}

// readFragment — the fragment an address asks for: { scope, q } or null.
export function readFragment(query) {
  const scope = typeof query?.frag === 'string' ? query.frag : ''
  if (scope !== 'desc' && !/^c:[0-9a-zA-Z-]{1,64}$/.test(scope)) return null
  return { scope, q: typeof query.q === 'string' ? query.q : '' }
}

// findText — the Range of the first occurrence of `q` under `root`, whitespace
// collapsed on both sides (the text crosses inline tags and line breaks).
export function findText(root, q) {
  const needle = String(q || '')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()
  if (!root || !needle) return null
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  // The flattened text with a map from each of its characters back to a node.
  let flat = ''
  const map = []
  let lastSpace = true
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const s = n.nodeValue
    for (let i = 0; i < s.length; i++) {
      const ch = /\s/.test(s[i]) ? ' ' : s[i].toLowerCase()
      if (ch === ' ' && lastSpace) continue
      lastSpace = ch === ' '
      flat += ch
      map.push([n, i])
    }
  }
  const at = flat.indexOf(needle)
  if (at < 0) return null
  const [sn, so] = map[at]
  const [en, eo] = map[at + needle.length - 1]
  const range = document.createRange()
  range.setStart(sn, so)
  range.setEnd(en, eo + 1)
  return range
}

// markRange highlights a range for `ms` (where the browser has the Highlight
// API; elsewhere the scroll alone shows the place).
export function markRange(range, ms = 4000) {
  const hl = globalThis.CSS?.highlights
  if (!range || !hl || typeof globalThis.Highlight !== 'function') return false
  hl.set(HIGHLIGHT, new globalThis.Highlight(range))
  setTimeout(() => hl.delete(HIGHLIGHT), ms)
  return true
}
