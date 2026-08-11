// Composer-bar facet protocol.
//
// Every entry in the composer's «+» menu carries a string key (`fa.<user-id>`,
// `g.tagp.<prefix>`, `s.due`, …). The menu builds those keys in one place and the
// select handler parses them back in another, ~350 lines apart, tied together by
// nothing but matching string literals: a typo in a prefix produced a menu entry
// that silently did nothing — no error, no failing test, no visible clue.
//
// Both ends now go through the table below, so the prefix, the chip `kind` and the
// field it writes in `filters` are declared once. `encodeFacet` throws on an unknown
// kind, which turns that class of typo into a loud failure at the call site.

// Filter facets, in the order they appear in a blank filter set.
// - `prefix` — menu-key namespace (null = no menu entry; the search box is state-only)
// - `kind`   — chip kind (also the key into the composer's icon map)
// - `field`  — the property it owns in the filters object
// - `multi`  — array-valued (a chip per element) vs a single value
export const FILTER_FACETS = [
  { kind: 'priority', prefix: 'fp', field: 'priorities', multi: true, parse: Number },
  { kind: 'assignee', prefix: 'fa', field: 'assignees', multi: true },
  { kind: 'author', prefix: 'fc', field: 'authors', multi: true },
  { kind: 'tag', prefix: 'ft', field: 'tags', multi: true },
  { kind: 'status', prefix: 'fs', field: 'statuses', multi: true },
  { kind: 'milestone', prefix: 'fm', field: 'milestones', multi: true },
  { kind: 'due', prefix: 'fd', field: 'due', multi: false },
  { kind: 'search', prefix: null, field: 'q', multi: false },
]

const BY_KIND = new Map(FILTER_FACETS.map((f) => [f.kind, f]))
const BY_PREFIX = new Map(FILTER_FACETS.filter((f) => f.prefix).map((f) => [f.prefix, f]))

// Sentinel for the milestone filter's «Без этапа» bucket. Shared with
// `utils/taskFilter.js`, which matches tasks carrying no milestone against it —
// another two-ends-one-literal pair before this module existed.
export const NO_MILESTONE = '__none__'

export const GROUP_PREFIX = 'g'
export const SORT_PREFIX = 's'
// Column grouping. `tag` optionally narrows to one namespace (`g.tagp.<prefix>`);
// `assignee`/`none` are timeline-only swimlane modes.
export const GROUP_MODES = ['status', 'tag', 'milestone', 'assignee', 'none']
const TAG_PREFIX_KEY = 'tagp.'

// A blank filter set. Everything that needs one (live state, reset, toolbar
// defaults/snapshot/restore, saved-view apply, legacy-blob migration) derives from
// here — adding a facet used to mean editing the same literal in six places, and
// forgetting one only showed up on the migration path, i.e. at users.
export function emptyFilters() {
  const out = {}
  for (const f of FILTER_FACETS) out[f.field] = f.multi ? [] : ''
  return out
}

// Copy `src` onto a fresh blank set: unknown keys are dropped and arrays are copied
// (never shared with a parsed localStorage blob or a saved view config).
export function cloneFilters(src) {
  const out = emptyFilters()
  for (const f of FILTER_FACETS) {
    const v = src?.[f.field]
    if (v === undefined || v === null) continue
    out[f.field] = f.multi ? (Array.isArray(v) ? [...v] : []) : v
  }
  return out
}

// How many facets are actually narrowing the board (drives the «Сбросить» button
// and the customize panel's badge).
export function countActiveFilters(filters) {
  let n = 0
  for (const f of FILTER_FACETS) {
    const v = filters?.[f.field]
    if (f.multi) n += Array.isArray(v) ? v.length : 0
    else if (typeof v === 'string' ? v.trim() : v) n += 1
  }
  return n
}

// Menu key for one filter value. Throws on an unknown kind: a mistyped kind is a
// bug in the menu, and failing here beats shipping an entry that does nothing.
export function encodeFacet(kind, value) {
  const f = BY_KIND.get(kind)
  if (!f || !f.prefix) throw new Error(`facetKeys: no menu key for facet kind "${kind}"`)
  return `${f.prefix}.${value}`
}

// Parse a filter menu key. Returns the facet descriptor plus the decoded `value`,
// or null for anything that isn't one (group/sort keys, Naive group headers like
// `fag`/`ftg.<key>`, the mobile drill's `nav.*`).
export function decodeFacet(key) {
  if (typeof key !== 'string') return null
  const dot = key.indexOf('.')
  if (dot < 0) return null
  const f = BY_PREFIX.get(key.slice(0, dot))
  if (!f) return null
  const raw = key.slice(dot + 1)
  const value = f.parse ? f.parse(raw) : raw
  if (typeof value === 'number' && Number.isNaN(value)) return null
  return { ...f, value }
}

// Write a decoded facet into a filters object. Returns whether anything changed
// (picking a value that's already active is a no-op, not a duplicate chip).
export function applyFilterFacet(filters, decoded) {
  if (!decoded) return false
  const { field, multi, value } = decoded
  if (!multi) {
    if (filters[field] === value) return false
    filters[field] = value
    return true
  }
  if (!Array.isArray(filters[field])) filters[field] = []
  if (filters[field].includes(value)) return false
  filters[field].push(value)
  return true
}

// Drop one facet value (the chip's «×»). Single-valued facets clear entirely.
export function removeFilterFacet(filters, kind, value) {
  const f = BY_KIND.get(kind)
  if (!f) return false
  if (!f.multi) {
    filters[f.field] = ''
    return true
  }
  filters[f.field] = (filters[f.field] || []).filter((x) => x !== value)
  return true
}

export function encodeGroup(mode, prefix = '') {
  if (mode === 'tag' && prefix)
    return `${GROUP_PREFIX}.${TAG_PREFIX_KEY}${encodeURIComponent(prefix)}`
  return `${GROUP_PREFIX}.${mode}`
}

// → { mode, prefix } or null. An unknown mode decodes to null rather than being
// assigned blindly, so a bad key leaves the board's grouping untouched.
export function decodeGroup(key) {
  if (typeof key !== 'string' || !key.startsWith(`${GROUP_PREFIX}.`)) return null
  const rest = key.slice(GROUP_PREFIX.length + 1)
  if (rest.startsWith(TAG_PREFIX_KEY)) {
    return { mode: 'tag', prefix: decodeURIComponent(rest.slice(TAG_PREFIX_KEY.length)) }
  }
  return GROUP_MODES.includes(rest) ? { mode: rest, prefix: '' } : null
}

export function encodeSort(field) {
  return `${SORT_PREFIX}.${field}`
}

export function decodeSort(key) {
  if (typeof key !== 'string' || !key.startsWith(`${SORT_PREFIX}.`)) return null
  const field = key.slice(SORT_PREFIX.length + 1)
  return field || null
}
