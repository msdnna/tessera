// The tag table of the «Проект» window (#2990): groups by prefix, the GitLab
// lock, the filter, row selection and the new-tag field. Pure helpers — the
// table component keeps only the state.
import { buildTagGroups, canonPrefix, splitTag, tagNamespace } from '@/utils/tagGroups'

// Canonical prefixes a GitLab label rule of this project governs — every
// prefix-type rule, whatever its action: a label mapped to a field (status,
// priority…) or kept as a tag, its name and colour come from GitLab, so editing
// the tag here would only be undone by the next sync. Regex rules cannot be
// reduced to a prefix and lock nothing. Only enabled integrations of the
// project's own boards count.
export function ruledPrefixes(integrations, projectId) {
  const out = new Set()
  for (const b of integrations || []) {
    if (b.enabled === false) continue
    if (projectId && b.project_id && b.project_id !== projectId) continue
    for (const r of b.label_rules?.rules || []) {
      if ((r.match_type || 'prefix') !== 'prefix') continue
      const key = canonPrefix(r.match)
      if (key) out.add(key)
    }
  }
  return out
}

// Is this tag under a GitLab rule (read-only in the table)?
export function isRuled(tag, locked) {
  if (!locked || !locked.size) return false
  const key = canonPrefix(tagNamespace(tag?.name))
  return !!key && locked.has(key)
}

// Does a tag pass the table's filter? Matched against the name and the
// group's friendly name, case-insensitively.
export function matchesFilter(tag, filter, prefixNames = {}) {
  const q = (filter || '').trim().toLowerCase()
  if (!q) return true
  const name = (tag?.name || '').toLowerCase()
  if (name.includes(q)) return true
  const label = prefixNames[canonPrefix(tagNamespace(tag?.name))] || ''
  return label.toLowerCase().includes(q)
}

// Groups of the table: buildTagGroups (prefix groups alphabetically, the
// ungrouped bucket last), narrowed by the filter, each with its lock and count.
export function tagTableGroups(tags, { prefixNames = {}, filter = '', locked = null } = {}) {
  const shown = (tags || []).filter((t) => matchesFilter(t, filter, prefixNames))
  return buildTagGroups(shown, prefixNames).map((g) => ({
    ...g,
    named: !!(g.key && prefixNames[g.key]),
    ruled: !!g.tags.length && g.tags.every((t) => isRuled(t, locked)),
  }))
}

// Flat order of the selectable rows (what ↑↓ walk and ⇧ ranges span), minus
// the collapsed groups.
export function rowOrder(groups, collapsed = new Set()) {
  const out = []
  for (const g of groups) {
    if (collapsed.has(g.key)) continue
    for (const t of g.tags) out.push(t.id)
  }
  return out
}

// toggle adds or drops one id; returns a new Set.
export function toggleSelected(sel, id) {
  const next = new Set(sel)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

// selectRange adds every row between the anchor and `id` (inclusive) in the
// table's order; returns a new Set. Without an anchor it is just `id`.
export function selectRange(sel, order, anchor, id) {
  const next = new Set(sel)
  const a = order.indexOf(anchor)
  const b = order.indexOf(id)
  if (a < 0 || b < 0) {
    next.add(id)
    return next
  }
  const [lo, hi] = a < b ? [a, b] : [b, a]
  for (let i = lo; i <= hi; i++) next.add(order[i])
  return next
}

// The neighbour of `id` in the order, `step` rows away (clamped); the first row
// when `id` is not in it.
export function stepRow(order, id, step) {
  if (!order.length) return null
  const i = order.indexOf(id)
  if (i < 0) return order[step < 0 ? order.length - 1 : 0]
  return order[Math.max(0, Math.min(order.length - 1, i + step))]
}

// The new-tag field takes «префикс::имя» or a bare name. A prefix with nothing
// after it is not a tag (it would be an empty value in a group). Returns the
// trimmed name or '' when it cannot be created.
export function newTagName(input) {
  const s = (input || '').trim()
  if (!s) return ''
  const { ns, label } = splitTag(s)
  if (ns && !label) return ''
  return s
}

// The tag a merge keeps by default: the most used of the selection (ties →
// the shortest name, then alphabetical) — the duplicates fold into it.
export function defaultMergeTarget(tags) {
  const list = [...(tags || [])]
  list.sort(
    (a, b) =>
      (b.task_count || 0) - (a.task_count || 0) ||
      (a.name || '').length - (b.name || '').length ||
      (a.name || '').localeCompare(b.name || ''),
  )
  return list[0] || null
}

// Where a tag came from, for the «Источник» column: a GitLab rule, a GitLab
// label, or the project itself.
export function tagSource(tag, locked) {
  if (isRuled(tag, locked)) return 'rule'
  return tag?.source === 'gitlab' ? 'gitlab' : 'user'
}
