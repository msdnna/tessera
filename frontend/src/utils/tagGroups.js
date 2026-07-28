// Tag-prefix grouping helpers. Tags follow a "<prefix>: value" / "<prefix>::value"
// naming convention; this module derives the prefix namespace, canonicalises it
// to a key, and groups a tag list under friendly display names.

// Namespace of a tag name ("T: bug" → "T: ", "effort::small" → "effort::").
// Returns '' when the tag has no prefix.
export function tagNamespace(name) {
  const s = name || ''
  const i = s.indexOf('::')
  if (i >= 0) return s.slice(0, i + 2)
  const j = s.indexOf(': ')
  if (j >= 0) return s.slice(0, j + 2)
  return ''
}

// Canonical prefix key — must mirror the backend (handlers/tag_prefixes.go
// canonPrefix) so a namespace derived from a tag ("S: ") lines up with what the
// GitLab rule editor stores ("S:").
export function canonPrefix(p) {
  return (p || '').trim().toLowerCase()
}

// Display label for a namespace: the configured friendly name, else the trimmed
// raw prefix (e.g. "S:"), else 'Вне группы' for prefix-less tags.
export function prefixLabel(ns, prefixNames = {}) {
  if (!ns) return 'Вне группы'
  return prefixNames[canonPrefix(ns)] || ns.trim()
}

// Canonical prefixes governed by a non-"tag" GitLab label rule (status / priority /
// group / board / ignore). Those labels drive a task field (or are dropped on sync),
// so the matching tags shouldn't be manually addable in the tag picker — selecting
// one just desyncs from the field. Only prefix-type rules qualify; regex rules can't
// be reduced to a stable prefix, so their tags stay visible.
export function metaPrefixesFromRules(rules) {
  const out = new Set()
  for (const r of rules?.rules || []) {
    if ((r.match_type || 'prefix') !== 'prefix') continue
    if (!r.action || r.action === 'tag') continue
    const key = canonPrefix(r.match)
    if (key) out.add(key)
  }
  return out
}

// Group a tag list by prefix namespace. Returns ordered groups
// [{ key, label, prefix, tags }] sorted alphabetically by label, with the
// prefix-less "Вне группы" bucket always last. Tags within a group are sorted
// by name. prefixNames maps canonical prefix → friendly label. hidePrefixes (a Set of
// canonical prefixes, e.g. from metaPrefixesFromRules) drops those tags entirely.
export function buildTagGroups(tags, prefixNames = {}, hidePrefixes = null) {
  const groups = new Map()
  for (const t of tags || []) {
    const ns = tagNamespace(t.name)
    const key = canonPrefix(ns)
    if (hidePrefixes && key && hidePrefixes.has(key)) continue
    if (!groups.has(key)) {
      groups.set(key, { key, label: prefixLabel(ns, prefixNames), prefix: ns, tags: [] })
    }
    groups.get(key).tags.push(t)
  }
  const arr = [...groups.values()]
  for (const g of arr) g.tags.sort((a, b) => (a.name || '').localeCompare(b.name || '', 'ru'))
  arr.sort((a, b) => {
    if (a.key === '') return 1
    if (b.key === '') return -1
    return a.label.localeCompare(b.label, 'ru')
  })
  return arr
}
