// Ranking and highlighting for the ⌘K palette (v2, #2957).
//
// Deliberately simple: palette lists are short (a few dozen candidates per
// section after the server's own ILIKE), so a readable score beats a clever
// one. Order of preference: the whole text, a prefix of it, the start of a
// word inside it, any substring, and finally every word of the query found
// somewhere (in any order).

const WORD_CHAR = /[\p{L}\p{N}]/u

export function matchScore(text, query) {
  const q = (query || '').trim().toLowerCase()
  if (!q) return 1
  const t = (text || '').toLowerCase()
  if (!t) return 0
  if (t === q) return 100
  if (t.startsWith(q)) return 80
  const i = t.indexOf(q)
  if (i >= 0) {
    const atWord = !WORD_CHAR.test(t[i - 1])
    return (atWord ? 60 : 40) - Math.min(i, 30) / 3
  }
  const terms = q.split(/\s+/).filter(Boolean)
  if (terms.length > 1 && terms.every((term) => t.includes(term))) return 20
  return 0
}

// bestScore is the highest score over several fields of one candidate (title,
// e-mail, project name …), weighting the secondary ones down.
export function bestScore(fields, query) {
  let best = 0
  fields.forEach((f, i) => {
    const s = matchScore(f, query) * (i === 0 ? 1 : 0.6)
    if (s > best) best = s
  })
  return best
}

// highlight splits `text` around the first occurrence of `query` (case-
// insensitive) into [{ text, hit }] segments for rendering. No occurrence → one
// plain segment; the row still shows, it just matched on another field.
export function highlight(text, query) {
  const s = text || ''
  const q = (query || '').trim()
  if (!q) return [{ text: s, hit: false }]
  const i = s.toLowerCase().indexOf(q.toLowerCase())
  if (i < 0) return [{ text: s, hit: false }]
  const out = []
  if (i > 0) out.push({ text: s.slice(0, i), hit: false })
  out.push({ text: s.slice(i, i + q.length), hit: true })
  if (i + q.length < s.length) out.push({ text: s.slice(i + q.length), hit: false })
  return out
}

// rank filters candidates by score and sorts them best-first, keeping the
// incoming order among equals (the server's own order is meaningful).
export function rank(items, fieldsOf, query, limit = Infinity) {
  return items
    .map((item, i) => ({ item, i, s: bestScore(fieldsOf(item), query) }))
    .filter((x) => x.s > 0)
    .sort((a, b) => b.s - a.s || a.i - b.i)
    .slice(0, limit)
    .map((x) => x.item)
}
