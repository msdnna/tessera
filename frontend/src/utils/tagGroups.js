// Tag-prefix grouping helpers. Tags follow a "<prefix>: value" / "<prefix>::value"
// naming convention; this module derives the prefix namespace, canonicalises it
// to a key, and groups a tag list under friendly display names.
import { i18n } from '@/i18n'

// Resolved per call: the module is imported outside a setup context, so a label
// taken at import time would freeze on the first render's language (#2799).
const t = (key, ...rest) => i18n.global.t(key, ...rest)

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
// raw prefix (e.g. "S:"), else the "ungrouped" bucket name for prefix-less tags.
export function prefixLabel(ns, prefixNames = {}) {
  if (!ns) return t('board.tag.ungrouped')
  return prefixNames[canonPrefix(ns)] || ns.trim()
}

// Split a tag name into its namespace, bare scope and value:
//   "effort::small" → { ns: 'effort::', scope: 'effort', label: 'small' }
//   "T: bug"        → { ns: 'T: ',      scope: 'T',      label: 'bug'   }
//   "urgent"        → { ns: '',         scope: '',       label: 'urgent'}
// `scope` drops the separator (prefixLabel keeps it — that one titles a *group*,
// this one titles a pill segment).
export function splitTag(name) {
  const s = name || ''
  const ns = tagNamespace(s)
  if (!ns) return { ns: '', scope: '', label: s.trim() }
  return {
    ns,
    scope: ns.replace(/:+\s*$/, '').trim(),
    label: s.slice(ns.length).trim(),
  }
}

// Presentation parts of a scoped tag pill: the friendly scope name (configured
// label if any, else the raw prefix) and the value. A tag whose scope or value
// side is empty ("effort::", "::small") has no usable split — it falls back to
// the whole raw name so nothing silently disappears from the UI.
export function tagParts(name, prefixNames = {}) {
  const { ns, scope, label } = splitTag(name)
  if (!ns || !scope || !label) {
    return { scope: '', label: (name || '').trim(), hasScope: false }
  }
  return { scope: prefixNames[canonPrefix(ns)] || scope, label, hasScope: true }
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
// prefix-less "ungrouped" bucket always last. Tags within a group are sorted
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
