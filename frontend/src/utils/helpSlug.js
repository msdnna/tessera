// Heading anchors for help articles (#2788).
//
// Deliberately dependency-free and importable from Node: the build script
// (scripts/build-help-index.mjs) puts these ids into helpIndex.json for the
// table of contents, and the renderer stamps the same ids onto the <h2>/<h3>
// it produces. If the two ever disagreed, every TOC link would scroll nowhere —
// so both sides call this one function.
//
// Ids keep Cyrillic as-is (valid in HTML5, and transliteration would only make
// the anchors unreadable); a URL fragment percent-encodes them on its own.

export function slugifyHeading(text) {
  const base = String(text)
    .toLowerCase()
    .replace(/[`*_~]/g, '') // inline markdown that survives into the heading text
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
  return base || 'section'
}

// uniqueHeadingId keeps ids unique within one article: two «Как это работает»
// headings would otherwise both answer to the same anchor and the second one
// would be unreachable. `seen` is a Map of base id → how many times used.
export function uniqueHeadingId(text, seen) {
  const base = slugifyHeading(text)
  const n = seen.get(base) || 0
  seen.set(base, n + 1)
  return n ? `${base}-${n + 1}` : base
}
