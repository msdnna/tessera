import { computed, reactive, ref } from 'vue'
import {
  AlbumsOutline,
  SwapVerticalOutline,
  FlagOutline,
  PersonOutline,
  PricetagOutline,
  ListOutline,
  CalendarOutline,
  RibbonOutline,
  CreateOutline,
} from '@vicons/ionicons5'
import { PRIORITY_LABELS } from '@/styles/tokens'
import { tagNamespace, prefixLabel, tagParts, buildTagGroups } from '@/utils/tagGroups'
import { boardGitlabAuthors } from '@/utils/boardFilters'
import {
  emptyFilters,
  countActiveFilters,
  encodeFacet,
  decodeFacet,
  NO_MILESTONE,
  applyFilterFacet,
  removeFilterFacet,
  encodeGroup,
  decodeGroup,
  encodeSort,
  decodeSort,
} from '@/utils/facetKeys'

// The composer bar's facet layer: the filter set, the multi-level sort, the chips
// that render them and the "add" menu that creates them.
//
// All three are one mechanism read from both ends — a menu entry encodes a facet
// key, the handler decodes it back into the same `filters` object the chips render.
// The key protocol itself lives in `utils/facetKeys.js`; this composable owns the
// state those keys address and the menus/chips built over it.
//
// What it deliberately does NOT own: grouping mode (`groupMode`/`tagPrefix` drive
// column building, so the board keeps them and passes them in), the router, and the
// mobile drill-in rendering of the add menu — that one is pure presentation and
// stays in the component with the rest of the `h()` calls.
//
// The sprint scope lives in the URL, so instead of a router the composable takes
// `milestoneScope` + `clearMilestoneScope`: it stays testable without a router mock.

// Per-kind icon shown on composer chips in place of the text prefix
// («Группировка:», «Сорт:», «Приоритет:» …). The customize panel still uses
// the full `label` (with prefix) since it renders chips as plain text.
export const CHIP_ICONS = {
  group: AlbumsOutline,
  sort: SwapVerticalOutline,
  priority: FlagOutline,
  assignee: PersonOutline,
  author: CreateOutline,
  tag: PricetagOutline,
  status: ListOutline,
  milestone: RibbonOutline,
  due: CalendarOutline,
}

export const SORT_FIELD_OPTIONS = [
  { label: 'Приоритет', value: 'priority' },
  { label: 'Срок', value: 'due' },
  { label: 'Этап', value: 'milestone' },
  { label: 'Статус', value: 'status' },
  { label: 'Название', value: 'title' },
  { label: 'Номер', value: 'number' },
]

export const DUE_OPTIONS = [
  { label: 'Все', value: '' },
  { label: 'Просроченные', value: 'overdue' },
  { label: 'Сегодня', value: 'today' },
  { label: 'Ближайшая неделя', value: 'week' },
  { label: 'Со сроком', value: 'has' },
  { label: 'Без срока', value: 'none' },
]

export function useBoardFacets({
  // grouping state (owned by the board — it builds columns from it)
  groupMode,
  tagPrefix,
  // 'timeline' | 'gantt' — gates the facets that only make sense on a time axis
  timelineLike,
  // board data
  columns,
  allTasks,
  tagsList,
  membersList,
  milestonesList,
  gitlabMembersList,
  tagsMap,
  membersMap,
  milestonesMap,
  tagPrefixNames,
  glLoginByUserId,
  // sprint scope (?milestone=…), passed as value + callback instead of the router
  milestoneScope,
  clearMilestoneScope,
}) {
  // Multi-level sort: an ordered list of { field, dir }. Empty = manual order.
  const sortLevels = ref([])
  // The blank filter set and its shape live in `utils/facetKeys.js` alongside the
  // menu-key protocol, so a new facet is declared once instead of in six literals.
  const filters = reactive(emptyFilters())

  // ── grouping helpers ──

  // Detected namespaces from the project tags, for the prefix picker. Labels use
  // the configured friendly name (else the raw prefix), sorted alphabetically.
  const tagPrefixOptions = computed(() => {
    const set = new Set()
    for (const t of tagsList.value) {
      const ns = tagNamespace(t.name)
      if (ns) set.add(ns)
    }
    return [
      { label: 'Все теги', value: '' },
      ...[...set]
        .map((p) => ({ label: prefixLabel(p, tagPrefixNames), value: p }))
        .sort((a, b) => a.label.localeCompare(b.label, 'ru')),
    ]
  })
  // Friendly label for the current grouping (status / tag[·prefix] / assignee / none).
  const groupModeLabel = computed(() => {
    if (groupMode.value === 'assignee') return 'Исполнитель'
    if (groupMode.value === 'none') return 'Без группировки'
    if (groupMode.value === 'milestone') return 'Этап'
    if (groupMode.value === 'tag')
      return `Тег${tagPrefix.value ? ` · ${prefixLabel(tagPrefix.value, tagPrefixNames)}` : ''}`
    return 'Статус'
  })

  // ── sorting ──

  // Status sort/filter is offered only on the timeline for now (the board already
  // groups by status into columns, so sorting/filtering by it there is redundant).
  const sortFieldsForMenu = computed(() =>
    timelineLike.value
      ? SORT_FIELD_OPTIONS
      : SORT_FIELD_OPTIONS.filter((o) => o.value !== 'status'),
  )
  function sortFieldLabel(field) {
    return SORT_FIELD_OPTIONS.find((o) => o.value === field)?.label || field
  }
  // column id → position, for the status sort.
  const colPos = computed(() => {
    const m = {}
    columns.value.forEach((c) => (m[c.id] = c.position))
    return m
  })
  // Milestone sort order: by the milestone's due date (none last), then its title.
  const milestoneSortKey = (t) => {
    const m = t.milestone_id ? milestonesMap[t.milestone_id] : null
    if (!m) return { d: Number.POSITIVE_INFINITY, s: '' }
    return { d: m.due_date ? Date.parse(m.due_date) : Number.POSITIVE_INFINITY, s: m.title || '' }
  }
  // One sort level's comparison (direction applied; due-less tasks always last).
  function cmpLevel(a, b, { field, dir }) {
    const d = dir === 'desc' ? -1 : 1
    if (field === 'status')
      return d * ((colPos.value[a.column_id] ?? 0) - (colPos.value[b.column_id] ?? 0))
    if (field === 'due') {
      const av = a.due_date ? Date.parse(a.due_date) : null
      const bv = b.due_date ? Date.parse(b.due_date) : null
      if (av === null && bv === null) return 0
      if (av === null) return 1
      if (bv === null) return -1
      return d * (av - bv)
    }
    if (field === 'priority') return d * ((a.priority || 0) - (b.priority || 0))
    if (field === 'milestone') {
      const ka = milestoneSortKey(a)
      const kb = milestoneSortKey(b)
      if (ka.d !== kb.d) return d * (ka.d - kb.d)
      return d * ka.s.localeCompare(kb.s, 'ru')
    }
    if (field === 'title')
      return d * String(a.title || '').localeCompare(String(b.title || ''), 'ru')
    if (field === 'number') return d * ((a.number || 0) - (b.number || 0))
    return 0
  }

  // ── filter menus ──

  const priorityFilterOptions = PRIORITY_LABELS.map((label, value) => ({ label, value }))
  // Assignee filter menu — carries avatar hints so `renderAddLabel` can draw the
  // user's face (Tessera by `avatarUserId`, GitLab by `avatarSrc`), like the on-card
  // assignee picker. GitLab-only assignees sit under an inline «GitLab» group; their
  // filter value is prefixed `gl:` so it matches against gitlab_assignee_logins.
  const memberFilterMenu = computed(() => {
    const tessera = membersList.value.map((m) => ({
      label: m.name,
      key: encodeFacet('assignee', m.user_id),
      avatarUserId: m.user_id,
    }))
    const gl = gitlabMembersList.value.map((g) => ({
      label: g.gl_name || g.gl_username,
      key: encodeFacet('assignee', `gl:${g.gl_username}`),
      avatarSrc: g.gl_avatar_url,
    }))
    if (!gl.length) return tessera
    return [...tessera, { type: 'group', label: 'GitLab', key: 'fag', children: gl }]
  })
  // Author filter menu — same shape as the assignee one (avatar hints, `gl:`-prefixed
  // GitLab values, `fc.` = creator keys), plus the GitLab authors actually seen on the
  // board: an issue can be opened by someone outside the project's member roster.
  // Logins already represented by a Tessera row (linked accounts) are skipped so one
  // person never shows up twice.
  const authorFilterMenu = computed(() => {
    const tessera = membersList.value.map((m) => ({
      label: m.name,
      key: encodeFacet('author', m.user_id),
      avatarUserId: m.user_id,
    }))
    const seen = new Set(Object.values(glLoginByUserId.value))
    const gl = []
    const pushGl = (username, name, avatar) => {
      if (!username || seen.has(username)) return
      seen.add(username)
      gl.push({
        label: name || username,
        key: encodeFacet('author', `gl:${username}`),
        avatarSrc: avatar,
      })
    }
    gitlabMembersList.value.forEach((g) => pushGl(g.gl_username, g.gl_name, g.gl_avatar_url))
    boardGitlabAuthors(allTasks.value).forEach((a) =>
      pushGl(a.gl_username, a.gl_name, a.gl_avatar_url),
    )
    if (!gl.length) return tessera
    return [...tessera, { type: 'group', label: 'GitLab', key: 'fcg', children: gl }]
  })
  // Tag filter menu, grouped by prefix (friendly names). Naive `type:'group'`
  // renders inline section headers — works on desktop and the mobile drill alike.
  // A single prefix-less bucket stays flat (no redundant header).
  const tagFilterMenu = computed(() => {
    const groups = buildTagGroups(tagsList.value, tagPrefixNames)
    // Inside a group the header already names the scope, so entries show the bare
    // value; a flat (single-bucket) menu spells the scope out instead.
    const flatLabel = (t) => {
      const p = tagParts(t.name, tagPrefixNames)
      return p.hasScope ? `${p.scope}: ${p.label}` : p.label
    }
    if (groups.length <= 1) {
      return (groups[0]?.tags || []).map((t) => ({
        label: flatLabel(t),
        key: encodeFacet('tag', t.id),
      }))
    }
    return groups.map((g) => ({
      type: 'group',
      label: g.label,
      key: `ftg.${g.key}`,
      children: g.tags.map((t) => ({
        label: tagParts(t.name, tagPrefixNames).label,
        key: encodeFacet('tag', t.id),
      })),
    }))
  })
  // Status filter = which board columns to show (timeline-only facet).
  const statusFilterOptions = computed(() =>
    columns.value.map((c) => ({ label: c.name, value: c.id })),
  )
  // Milestone filter menu (+ an explicit "Без этапа" bucket).
  const milestoneFilterMenu = computed(() => [
    ...milestonesList.value.map((m) => ({ label: m.title, key: encodeFacet('milestone', m.id) })),
    { label: 'Без этапа', key: encodeFacet('milestone', NO_MILESTONE) },
  ])
  const activeFilterCount = computed(() => countActiveFilters(filters))
  function resetFilters() {
    Object.assign(filters, emptyFilters())
  }

  // ── chips ──
  // All facets render as chips over the existing state; the "add" dropdown mutates
  // the same refs. The search box lives in the bar too (filters.q).

  // A GitLab-only person is stored as `gl:<login>`; resolve the display name from
  // the member roster, else (board-only authors) from the synced task itself.
  function glDisplayName(value, withBoardFallback) {
    const u = value.slice(3)
    const g = gitlabMembersList.value.find((x) => x.gl_username === u)
    if (g) return g.gl_name || g.gl_username || u
    if (!withBoardFallback) return u
    const b = boardGitlabAuthors(allTasks.value).find((x) => x.gl_username === u)
    return b?.gl_name || u
  }
  function personName(value, withBoardFallback) {
    if (typeof value === 'string' && value.startsWith('gl:'))
      return glDisplayName(value, withBoardFallback)
    return membersMap[value]?.name || '—'
  }

  const facetChips = computed(() => {
    const out = []
    const g = groupModeLabel.value
    out.push({ kind: 'group', icon: CHIP_ICONS.group, text: g, label: `Группировка: ${g}` })
    sortLevels.value.forEach((l, i) => {
      const f = sortFieldLabel(l.field)
      const arrow = l.dir === 'desc' ? '↓' : '↑'
      out.push({
        kind: 'sort',
        i,
        icon: CHIP_ICONS.sort,
        text: `${f} ${arrow}`,
        label: `Сорт: ${f} ${arrow}`,
      })
    })
    filters.priorities.forEach((p) => {
      const t = PRIORITY_LABELS[p]
      out.push({
        kind: 'priority',
        value: p,
        icon: CHIP_ICONS.priority,
        text: t,
        label: `Приоритет: ${t}`,
      })
    })
    filters.assignees.forEach((a) => {
      const name = personName(a, false)
      out.push({
        kind: 'assignee',
        value: a,
        icon: CHIP_ICONS.assignee,
        text: name,
        label: `Исполнитель: ${name}`,
      })
    })
    filters.authors.forEach((a) => {
      const name = personName(a, true)
      out.push({
        kind: 'author',
        value: a,
        icon: CHIP_ICONS.author,
        text: name,
        label: `Автор: ${name}`,
      })
    })
    filters.tags.forEach((t) => {
      const nm = tagsMap[t]?.name || '—'
      out.push({ kind: 'tag', value: t, icon: CHIP_ICONS.tag, text: nm, label: `Тег: ${nm}` })
    })
    filters.statuses.forEach((s) => {
      const nm = columns.value.find((c) => c.id === s)?.name || '—'
      out.push({
        kind: 'status',
        value: s,
        icon: CHIP_ICONS.status,
        text: nm,
        label: `Статус: ${nm}`,
      })
    })
    filters.milestones.forEach((m) => {
      const nm = m === NO_MILESTONE ? 'без этапа' : milestonesMap[m]?.title || '—'
      out.push({
        kind: 'milestone',
        value: m,
        icon: CHIP_ICONS.milestone,
        text: nm,
        label: `Этап: ${nm}`,
      })
    })
    if (filters.due) {
      const nm = DUE_OPTIONS.find((o) => o.value === filters.due)?.label || filters.due
      out.push({ kind: 'due', icon: CHIP_ICONS.due, text: nm, label: `Срок: ${nm}` })
    }
    return out
  })
  // Composer renders group + sort separately (sort chips are drag-reorderable), so
  // the flat chip loop covers only the filter facets.
  const filterChips = computed(() =>
    facetChips.value.filter((c) => c.kind !== 'group' && c.kind !== 'sort'),
  )
  const groupChip = computed(() => facetChips.value.find((c) => c.kind === 'group'))

  // ── the "add" menu ──

  const addOptions = computed(() => {
    const grouping = [
      { label: 'По статусам', key: encodeGroup('status') },
      { label: 'По тегам (все)', key: encodeGroup('tag') },
      ...tagPrefixOptions.value
        .filter((o) => o.value)
        .map((o) => ({
          label: `По тегам · ${o.label}`,
          key: encodeGroup('tag', o.value),
        })),
      { label: 'По этапам', key: encodeGroup('milestone') },
    ]
    // Timeline swimlanes can also be per-assignee or ungrouped.
    if (timelineLike.value) {
      grouping.push(
        { label: 'По исполнителю', key: encodeGroup('assignee') },
        { label: 'Без группировки', key: encodeGroup('none') },
      )
    }
    const opts = [
      { label: 'Группировка', key: 'group', children: grouping },
      {
        label: 'Сортировка',
        key: 'sort',
        children: sortFieldsForMenu.value.map((o) => ({
          label: o.label,
          key: encodeSort(o.value),
        })),
      },
      {
        label: 'Фильтр: приоритет',
        key: 'fp',
        children: priorityFilterOptions.map((o) => ({
          label: o.label,
          key: encodeFacet('priority', o.value),
        })),
      },
      {
        label: 'Фильтр: исполнитель',
        key: 'fa',
        children: memberFilterMenu.value,
      },
      {
        label: 'Фильтр: автор',
        key: 'fc',
        children: authorFilterMenu.value,
      },
      { label: 'Фильтр: тег', key: 'ft', children: tagFilterMenu.value },
      { label: 'Фильтр: этап', key: 'fm', children: milestoneFilterMenu.value },
      {
        label: 'Фильтр: срок',
        key: 'fd',
        children: DUE_OPTIONS.filter((o) => o.value).map((o) => ({
          label: o.label,
          key: encodeFacet('due', o.value),
        })),
      },
    ]
    // Status (column) filter — timeline only, so the user can hide e.g. the «done»
    // column's completed cards that otherwise crowd the chart.
    if (timelineLike.value) {
      opts.splice(2, 0, {
        label: 'Фильтр: статус',
        key: 'fs',
        children: statusFilterOptions.value.map((o) => ({
          label: o.label,
          key: encodeFacet('status', o.value),
        })),
      })
    }
    return opts
  })

  // ── mutations driven by the menu and the chips ──

  function onAddFacet(key) {
    const g = decodeGroup(key)
    if (g) {
      groupMode.value = g.mode
      tagPrefix.value = g.prefix
      return
    }
    const sortField = decodeSort(key)
    if (sortField) {
      if (!sortLevels.value.some((l) => l.field === sortField))
        sortLevels.value.push({ field: sortField, dir: 'asc' })
      return
    }
    const facet = decodeFacet(key)
    if (!facet) return
    applyFilterFacet(filters, facet)
    // Building a custom multi-sprint filter supersedes the tree's single-sprint
    // scope — drop it so the full board loads and the client filter applies.
    if (facet.kind === 'milestone' && milestoneScope.value) clearMilestoneScope()
  }
  function removeChip(c) {
    if (c.kind === 'sort') sortLevels.value.splice(c.i, 1)
    else removeFilterFacet(filters, c.kind, c.value)
  }
  function onChipClick(c) {
    if (c.kind === 'group') {
      groupMode.value = groupMode.value === 'status' ? 'tag' : 'status'
    } else if (c.kind === 'sort') {
      const l = sortLevels.value[c.i]
      l.dir = l.dir === 'desc' ? 'asc' : 'desc'
    }
  }
  // Sort chips are drag-reorderable, so operate on the level object (not a stale
  // facetChips index): toggling direction / removing find it in the live array.
  function toggleSortDir(l) {
    l.dir = l.dir === 'desc' ? 'asc' : 'desc'
  }
  function removeSort(l) {
    sortLevels.value = sortLevels.value.filter((x) => x !== l)
  }
  const hasClearableFacets = computed(
    () => sortLevels.value.length > 0 || activeFilterCount.value > 0,
  )
  function clearAll() {
    resetFilters()
    sortLevels.value = []
  }

  return {
    filters,
    sortLevels,
    tagPrefixOptions,
    groupModeLabel,
    sortFieldsForMenu,
    sortFieldLabel,
    cmpLevel,
    activeFilterCount,
    resetFilters,
    facetChips,
    filterChips,
    groupChip,
    addOptions,
    onAddFacet,
    removeChip,
    onChipClick,
    toggleSortDir,
    removeSort,
    hasClearableFacets,
    clearAll,
  }
}
