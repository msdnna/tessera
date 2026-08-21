// Client-side search over the help index (#2792).
//
// The whole corpus is a few dozen short articles shipped inside the bundle, so
// this runs entirely in the browser: no endpoint, no migration, works offline
// and in the desktop build. A prefix-matching inverted index is enough at this
// size — «доск» must already find «Доски и задачи», because a reader types the
// stem and stops.
//
// Ranking is field-weighted rather than frequency-based: in help text a word in
// the title says far more about relevance than the same word buried in a
// paragraph, and tf-idf over four-hundred-word articles mostly amplifies noise.

const WEIGHT = { title: 8, keywords: 4, heading: 3, text: 1 }

// A token is a run of letters/digits — Unicode-aware, so Cyrillic is not split
// apart and «e2e» stays one token. Hyphenated words break into parts on purpose:
// searching «код» should reach «QR-код».
const TOKEN_RE = /[\p{L}\p{N}]+/gu

export function tokenize(s) {
  return (
    String(s || '')
      .toLowerCase()
      .match(TOKEN_RE) || []
  )
}

// buildHelpSearch precomputes token → {slug → score} once per index. Articles
// come from helpIndex.json, so this happens a single time at module load in the
// store; the returned closure is what the UI calls on every keystroke.
export function buildHelpSearch(articles) {
  const postings = new Map() // token → Map(slug → accumulated weight)
  const bySlug = new Map()

  const add = (token, slug, weight) => {
    let entry = postings.get(token)
    if (!entry) postings.set(token, (entry = new Map()))
    entry.set(slug, (entry.get(slug) || 0) + weight)
  }

  for (const a of articles) {
    bySlug.set(a.slug, a)
    for (const t of tokenize(a.title)) add(t, a.slug, WEIGHT.title)
    for (const kw of a.keywords || []) for (const t of tokenize(kw)) add(t, a.slug, WEIGHT.keywords)
    for (const h of a.headings || [])
      for (const t of tokenize(h.text)) add(t, a.slug, WEIGHT.heading)
    // The body is scored once per distinct token: repeating a word ten times in
    // one article should not outrank an article that is actually about it.
    for (const t of new Set(tokenize(a.text))) add(t, a.slug, WEIGHT.text)
  }

  // Tokens sorted once so prefix lookup can scan a contiguous range instead of
  // testing every token in the corpus on each keystroke.
  const tokens = [...postings.keys()].sort()

  // matchPrefix returns the merged postings of every token starting with
  // `prefix` — an exact hit counts double, so «доска» beats «досках» when both
  // are present.
  const matchPrefix = (prefix) => {
    const hits = new Map()
    let lo = 0
    let hi = tokens.length
    while (lo < hi) {
      const mid = (lo + hi) >> 1
      if (tokens[mid] < prefix) lo = mid + 1
      else hi = mid
    }
    for (let i = lo; i < tokens.length && tokens[i].startsWith(prefix); i++) {
      const exact = tokens[i] === prefix ? 2 : 1
      for (const [slug, score] of postings.get(tokens[i])) {
        hits.set(slug, Math.max(hits.get(slug) || 0, score * exact))
      }
    }
    return hits
  }

  return function search(query, limit = 20) {
    const terms = tokenize(query)
    if (!terms.length) return []
    // Every term must match somewhere in the article (AND): with a corpus this
    // small, OR-matching turns a two-word query into "everything, ranked".
    let acc = null
    for (const term of terms) {
      const hits = matchPrefix(term)
      if (!hits.size) return []
      if (acc === null) {
        acc = hits
        continue
      }
      const next = new Map()
      for (const [slug, score] of hits) {
        if (acc.has(slug)) next.set(slug, acc.get(slug) + score)
      }
      if (!next.size) return []
      acc = next
    }
    return [...acc]
      .map(([slug, score]) => {
        const a = bySlug.get(slug)
        return {
          slug,
          score,
          title: a.title,
          category: a.category,
          excerpt: excerpt(a.text, terms),
        }
      })
      .sort((a, b) => b.score - a.score || a.title.localeCompare(b.title, 'ru'))
      .slice(0, limit)
  }
}

const EXCERPT_LEAD = 40
const EXCERPT_LEN = 160

// excerpt lifts the neighbourhood of the first matching term out of the
// flattened body, so a result row shows why it matched. Returns the opening of
// the article when the match is only in the title or keywords.
export function excerpt(text, terms) {
  const body = String(text || '')
  if (!body) return ''
  let at = -1
  for (const term of terms) {
    const found = body.indexOf(term)
    if (found >= 0 && (at < 0 || found < at)) at = found
  }
  if (at < 0) return body.slice(0, EXCERPT_LEN).trim() + (body.length > EXCERPT_LEN ? '…' : '')
  // Back up to a word boundary so the snippet does not start mid-word.
  let start = Math.max(0, at - EXCERPT_LEAD)
  if (start > 0) {
    const space = body.indexOf(' ', start)
    if (space >= 0 && space < at) start = space + 1
  }
  const end = Math.min(body.length, start + EXCERPT_LEN)
  return (start > 0 ? '…' : '') + body.slice(start, end).trim() + (end < body.length ? '…' : '')
}
