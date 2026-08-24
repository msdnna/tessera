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
import { i18n, uiCollator } from '@/i18n'
import { priorityLabel, priorityOptions } from '@/utils/priority'
import { tagNamespace, prefixLabel, tagParts, buildTagGroups } from '@/utils/tagGroups'
import { boardGitlabAuthors } from '@/utils/boardFilters'
import { columnName } from '@/utils/defaultNames'
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

// Anchors for the e2e suite on the entries it drives. Menu options are plain
// objects, so the hook Playwright needs is `props`, not an attribute in a
// template — and it has to be one that survives the labels being translated.
const TESTID = {
  group: { 'data-testid': 'facet-group' },
  groupStatus: { 'data-testid': 'facet-group-status' },
  groupTag: { 'data-testid': 'facet-group-tag' },
}

// Both tables used to be module-level arrays with Russian labels in them. A
// label computed once at import time never changes language again (pitfall 1 of
// the #2799 plan), so what stays constant here is the *order and the values* —
// the text is looked up on call, inside the computeds that render it.
//
// `t` comes from `i18n.global`, not from `useI18n()`: this composable also runs
// outside a component (its own spec calls it directly), and useI18n() is only
// legal inside setup. The global `t` reads the locale ref just the same, so a
// computed that calls it still re-runs on a language switch.
const t = (key, named) => i18n.global.t(key, named)

export const SORT_FIELD_VALUES = ['priority', 'due', 'milestone', 'status', 'title', 'number']
export const DUE_VALUES = ['', 'overdue', 'today', 'week', 'has', 'none']

// Locale-aware option lists (`{ label, value }`), rebuilt per call.
export function sortFieldOptions() {
  return SORT_FIELD_VALUES.map((value) => ({ label: t(`board.facet.sortField.${value}`), value }))
}
export function dueOptions() {
  return DUE_VALUES.map((value) => ({
    label: t(`board.facet.due.${value || 'all'}`),
    value,
  }))
}

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
    // `tag`, not `t`: the module-level `t` is the translator now, and shadowing
    // it inside a loop is exactly how a stray Russian label sneaks back in.
    for (const tag of tagsList.value) {
      const ns = tagNamespace(tag.name)
      if (ns) set.add(ns)
    }
    return [
      { label: t('board.facet.allTags'), value: '' },
      ...[...set]
        .map((p) => ({ label: prefixLabel(p, tagPrefixNames), value: p }))
        .sort((a, b) => uiCollator().compare(a.label, b.label)),
    ]
  })
  // Friendly label for the current grouping (status / tag[·prefix] / assignee / none).
  const groupModeLabel = computed(() => {
    if (groupMode.value === 'assignee') return t('board.facet.group.assignee')
    if (groupMode.value === 'none') return t('board.facet.group.none')
    if (groupMode.value === 'milestone') return t('board.facet.group.milestone')
    if (groupMode.value === 'tag')
      return tagPrefix.value
        ? t('board.facet.group.tagPrefix', { prefix: prefixLabel(tagPrefix.value, tagPrefixNames) })
        : t('board.facet.group.tag')
    return t('board.facet.group.status')
  })

  // ── sorting ──

  // Status sort/filter is offered only on the timeline for now (the board already
  // groups by status into columns, so sorting/filtering by it there is redundant).
  const sortFieldsForMenu = computed(() =>
    timelineLike.value
      ? sortFieldOptions()
      : sortFieldOptions().filter((o) => o.value !== 'status'),
  )
  function sortFieldLabel(field) {
    return SORT_FIELD_VALUES.includes(field) ? t(`board.facet.sortField.${field}`) : field
  }
  // column id → position, for the status sort.
  const colPos = computed(() => {
    const m = {}
    columns.value.forEach((c) => (m[c.id] = c.position))
    return m
  })
  // Milestone sort order: by the milestone's due date (none last), then its title.
  const milestoneSortKey = (task) => {
    const m = task.milestone_id ? milestonesMap[task.milestone_id] : null
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
      return d * uiCollator().compare(ka.s, kb.s)
    }
    if (field === 'title')
      return d * uiCollator().compare(String(a.title || ''), String(b.title || ''))
    if (field === 'number') return d * ((a.number || 0) - (b.number || 0))
    return 0
  }

  // ── filter menus ──

  const priorityFilterOptions = computed(() => priorityOptions())
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
    const flatLabel = (tag) => {
      const p = tagParts(tag.name, tagPrefixNames)
      return p.hasScope ? `${p.scope}: ${p.label}` : p.label
    }
    if (groups.length <= 1) {
      return (groups[0]?.tags || []).map((tag) => ({
        label: flatLabel(tag),
        key: encodeFacet('tag', tag.id),
      }))
    }
    return groups.map((g) => ({
      type: 'group',
      label: g.label,
      key: `ftg.${g.key}`,
      children: g.tags.map((tag) => ({
        label: tagParts(tag.name, tagPrefixNames).label,
        key: encodeFacet('tag', tag.id),
      })),
    }))
  })
  // Status filter = which board columns to show (timeline-only facet).
  const statusFilterOptions = computed(() =>
    columns.value.map((c) => ({ label: columnName(c), value: c.id })),
  )
  // Milestone filter menu (+ an explicit "no milestone" bucket).
  const milestoneFilterMenu = computed(() => [
    ...milestonesList.value.map((m) => ({ label: m.title, key: encodeFacet('milestone', m.id) })),
    { label: t('board.facet.noMilestone'), key: encodeFacet('milestone', NO_MILESTONE) },
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
    return membersMap[value]?.name || t('board.facet.unknown')
  }

  const facetChips = computed(() => {
    const out = []
    const unknown = t('board.facet.unknown')
    // Chip text is the bare value (an icon stands in for the prefix); `label` is
    // the spelled-out "kind: value" the customize panel renders instead.
    const chip = (kind, text, rest = {}) => ({
      kind,
      icon: CHIP_ICONS[kind],
      text,
      label: t(`board.facet.chip.${kind}`, { value: text }),
      ...rest,
    })
    out.push(chip('group', groupModeLabel.value))
    sortLevels.value.forEach((l, i) => {
      const arrow = l.dir === 'desc' ? '↓' : '↑'
      out.push(chip('sort', `${sortFieldLabel(l.field)} ${arrow}`, { i }))
    })
    filters.priorities.forEach((p) => out.push(chip('priority', priorityLabel(p), { value: p })))
    filters.assignees.forEach((a) => out.push(chip('assignee', personName(a, false), { value: a })))
    filters.authors.forEach((a) => out.push(chip('author', personName(a, true), { value: a })))
    filters.tags.forEach((tag) =>
      out.push(chip('tag', tagsMap[tag]?.name || unknown, { value: tag })),
    )
    filters.statuses.forEach((s) =>
      out.push(
        chip('status', columnName(columns.value.find((c) => c.id === s)) || unknown, {
          value: s,
        }),
      ),
    )
    filters.milestones.forEach((m) => {
      const nm =
        m === NO_MILESTONE ? t('board.facet.noMilestoneChip') : milestonesMap[m]?.title || unknown
      out.push(chip('milestone', nm, { value: m }))
    })
    if (filters.due) {
      const nm = dueOptions().find((o) => o.value === filters.due)?.label || filters.due
      out.push(chip('due', nm))
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
      // testid rather than the wording: the e2e suite reaches these through the
      // menu, and the labels are moving into the locale files (#2799).
      {
        label: t('board.facet.groupOption.status'),
        key: encodeGroup('status'),
        props: TESTID.groupStatus,
      },
      {
        label: t('board.facet.groupOption.tagAll'),
        key: encodeGroup('tag'),
        props: TESTID.groupTag,
      },
      ...tagPrefixOptions.value
        .filter((o) => o.value)
        .map((o) => ({
          label: t('board.facet.groupOption.tagPrefix', { prefix: o.label }),
          key: encodeGroup('tag', o.value),
        })),
      { label: t('board.facet.groupOption.milestone'), key: encodeGroup('milestone') },
    ]
    // Timeline swimlanes can also be per-assignee or ungrouped.
    if (timelineLike.value) {
      grouping.push(
        { label: t('board.facet.groupOption.assignee'), key: encodeGroup('assignee') },
        { label: t('board.facet.groupOption.none'), key: encodeGroup('none') },
      )
    }
    const opts = [
      { label: t('board.facet.menu.group'), key: 'group', children: grouping, props: TESTID.group },
      {
        label: t('board.facet.menu.sort'),
        key: 'sort',
        children: sortFieldsForMenu.value.map((o) => ({
          label: o.label,
          key: encodeSort(o.value),
        })),
      },
      {
        label: t('board.facet.menu.priority'),
        key: 'fp',
        children: priorityFilterOptions.value.map((o) => ({
          label: o.label,
          key: encodeFacet('priority', o.value),
        })),
      },
      {
        label: t('board.facet.menu.assignee'),
        key: 'fa',
        children: memberFilterMenu.value,
      },
      {
        label: t('board.facet.menu.author'),
        key: 'fc',
        children: authorFilterMenu.value,
      },
      { label: t('board.facet.menu.tag'), key: 'ft', children: tagFilterMenu.value },
      { label: t('board.facet.menu.milestone'), key: 'fm', children: milestoneFilterMenu.value },
      {
        label: t('board.facet.menu.due'),
        key: 'fd',
        children: dueOptions()
          .filter((o) => o.value)
          .map((o) => ({
            label: o.label,
            key: encodeFacet('due', o.value),
          })),
      },
    ]
    // Status (column) filter — timeline only, so the user can hide e.g. the «done»
    // column's completed cards that otherwise crowd the chart.
    if (timelineLike.value) {
      opts.splice(2, 0, {
        label: t('board.facet.menu.status'),
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
