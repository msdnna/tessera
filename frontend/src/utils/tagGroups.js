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

// Group a tag list by prefix namespace. Returns ordered groups
// [{ key, label, prefix, tags }] sorted alphabetically by label, with the
// prefix-less "Вне группы" bucket always last. Tags within a group are sorted
// by name. prefixNames maps canonical prefix → friendly label.
export function buildTagGroups(tags, prefixNames = {}) {
  const groups = new Map()
  for (const t of tags || []) {
    const ns = tagNamespace(t.name)
    const key = canonPrefix(ns)
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
