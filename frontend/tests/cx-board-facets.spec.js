import { describe, it, expect, vi } from 'vitest'
import { ref, reactive } from 'vue'

import { useBoardFacets } from '@/composables/useBoardFacets'
import { NO_MILESTONE } from '@/utils/facetKeys'

// The composer bar's facet layer: filters, multi-level sort, the chips that render
// them and the «+» menu that creates them. No lifecycle hooks, so it runs outside a
// component. Fixtures stay minimal — each test fills in the board data it needs.

function facets(over = {}) {
  const ctx = {
    groupMode: ref('status'),
    tagPrefix: ref(''),
    timelineLike: ref(false),
    columns: ref([
      { id: 'c1', name: 'К работе', position: 1 },
      { id: 'c2', name: 'Готово', position: 2 },
    ]),
    allTasks: ref([]),
    tagsList: ref([]),
    membersList: ref([]),
    milestonesList: ref([]),
    gitlabMembersList: ref([]),
    tagsMap: reactive({}),
    membersMap: reactive({}),
    milestonesMap: reactive({}),
    tagPrefixNames: reactive({}),
    glLoginByUserId: ref({}),
    milestoneScope: ref(''),
    clearMilestoneScope: vi.fn(),
    ...over,
  }
  const api = useBoardFacets(ctx)
  return { api, filters: api.filters, ...ctx }
}
// Chip texts of one kind — what the composer bar actually shows.
const chipText = (api, kind) =>
  api.facetChips.value.filter((c) => c.kind === kind).map((c) => c.text)

describe('useBoardFacets — chips', () => {
  it('always leads with the grouping chip and keeps it out of the filter chips', () => {
    const { api, groupMode, tagPrefix, tagPrefixNames, filters } = facets()
    expect(api.facetChips.value[0]).toMatchObject({ kind: 'group', text: 'Статус' })
    expect(api.groupChip.value.label).toBe('Группировка: Статус')

    groupMode.value = 'tag'
    tagPrefix.value = 'area/'
    tagPrefixNames['area/'] = 'Область'
    expect(api.groupChip.value.text).toBe('Тег · Область')

    filters.priorities.push(2)
    // The composer renders group + sort itself (sort chips are drag-reorderable),
    // so the flat chip loop must not repeat them.
    expect(api.filterChips.value.map((c) => c.kind)).toEqual(['priority'])
  })

  it('carries the sort level index on the chip so «×» removes the right one', () => {
    const { api } = facets()
    api.onAddFacet('s.priority')
    api.onAddFacet('s.due')
    expect(api.facetChips.value.filter((c) => c.kind === 'sort')).toEqual([
      expect.objectContaining({ i: 0, text: 'Приоритет ↑' }),
      expect.objectContaining({ i: 1, text: 'Срок ↑' }),
    ])
    api.removeChip({ kind: 'sort', i: 0 })
    expect(api.sortLevels.value).toEqual([{ field: 'due', dir: 'asc' }])
  })

  it('names a GitLab-only assignee from the member roster, else shows the bare login', () => {
    const { api, gitlabMembersList, filters } = facets()
    gitlabMembersList.value = [{ gl_username: 'ivan', gl_name: 'Иван П.' }]
    filters.assignees.push('gl:ivan', 'gl:ghost')
    expect(chipText(api, 'assignee')).toEqual(['Иван П.', 'ghost'])
  })

  it('falls back to the synced task for an author outside the member roster', () => {
    // An issue can be opened by someone who is not a project member — the roster
    // has no row for them, but the synced task itself carries the display name.
    const { api, allTasks, filters } = facets()
    allTasks.value = [{ gitlab_author: 'outsider', gitlab_author_name: 'Внешний А.' }]
    filters.authors.push('gl:outsider')
    expect(chipText(api, 'author')).toEqual(['Внешний А.'])
    // The assignee facet has no such source: an assignee is always a roster row.
    filters.assignees.push('gl:outsider')
    expect(chipText(api, 'assignee')).toEqual(['outsider'])
  })

  it('renders the «без этапа» sentinel and unknown ids without blowing up', () => {
    const { api, milestonesMap, tagsMap, filters } = facets()
    milestonesMap.m1 = { title: 'Спринт 1' }
    tagsMap.t1 = { name: 'bug' }
    filters.milestones.push('m1', NO_MILESTONE, 'gone')
    filters.tags.push('t1', 'gone')
    filters.statuses.push('c2', 'gone')
    expect(chipText(api, 'milestone')).toEqual(['Спринт 1', 'без этапа', '—'])
    expect(chipText(api, 'tag')).toEqual(['bug', '—'])
    expect(chipText(api, 'status')).toEqual(['Готово', '—'])
  })

  it('shows the due filter as a single chip with its menu label', () => {
    const { api, filters } = facets()
    filters.due = 'overdue'
    expect(chipText(api, 'due')).toEqual(['Просроченные'])
    // An unknown value still renders (raw) rather than dropping the chip and
    // leaving an active filter with nothing on screen to remove it.
    filters.due = 'bogus'
    expect(chipText(api, 'due')).toEqual(['bogus'])
  })
})

describe('useBoardFacets — the add menu', () => {
  it('offers the status facets only on a time axis', () => {
    const { api, timelineLike } = facets()
    const keys = () => api.addOptions.value.map((o) => o.key)
    const sortFields = () =>
      api.addOptions.value.find((o) => o.key === 'sort').children.map((c) => c.key)

    expect(keys()).not.toContain('fs')
    expect(sortFields()).not.toContain('s.status')

    timelineLike.value = true
    // The board already groups by status into columns, so both facets are
    // redundant there and only show up on timeline/gantt.
    expect(keys()).toContain('fs')
    expect(sortFields()).toContain('s.status')
    expect(api.addOptions.value.find((o) => o.key === 'group').children.map((c) => c.key)).toEqual(
      expect.arrayContaining(['g.assignee', 'g.none']),
    )
  })

  it('builds one grouping entry per detected tag namespace', () => {
    const { api, tagsList, tagPrefixNames } = facets()
    tagsList.value = [
      { id: 't1', name: 'area::api' },
      { id: 't2', name: 'plain' },
    ]
    tagPrefixNames['area::'] = 'Область'
    const group = api.addOptions.value.find((o) => o.key === 'group')
    expect(group.children.map((c) => c.label)).toContain('По тегам · Область')
  })

  it('groups GitLab people under an inline header, and only when there are any', () => {
    const { api, membersList, gitlabMembersList } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    expect(api.addOptions.value.find((o) => o.key === 'fa').children).toEqual([
      expect.objectContaining({ key: 'fa.u1', avatarUserId: 'u1' }),
    ])

    gitlabMembersList.value = [{ gl_username: 'ivan', gl_name: 'Иван П.' }]
    const assignees = api.addOptions.value.find((o) => o.key === 'fa').children
    expect(assignees[1]).toMatchObject({ type: 'group', label: 'GitLab', key: 'fag' })
    expect(assignees[1].children[0].key).toBe('fa.gl:ivan')
  })

  it('lists a linked GitLab account once, not twice', () => {
    // A Tessera user with a linked GitLab login already has a roster row; the
    // GitLab bucket must skip that login or the same person appears twice.
    const { api, membersList, gitlabMembersList, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    gitlabMembersList.value = [{ gl_username: 'anya', gl_name: 'Аня' }]
    glLoginByUserId.value = { u1: 'anya' }
    const authors = api.addOptions.value.find((o) => o.key === 'fc').children
    expect(authors).toEqual([expect.objectContaining({ key: 'fc.u1' })])
  })
})

describe('useBoardFacets — mutations', () => {
  it('routes a menu key to grouping, sort or filter', () => {
    const { api, groupMode, tagPrefix, filters } = facets()
    api.onAddFacet('g.tagp.area%2F')
    expect([groupMode.value, tagPrefix.value]).toEqual(['tag', 'area/'])

    api.onAddFacet('s.due')
    api.onAddFacet('s.due') // the same field twice must not stack two levels
    expect(api.sortLevels.value).toEqual([{ field: 'due', dir: 'asc' }])

    api.onAddFacet('fp.2')
    api.onAddFacet('fp.2')
    expect(filters.priorities).toEqual([2])
    // Naive group headers and the mobile drill's nav keys are not facets.
    api.onAddFacet('fag')
    api.onAddFacet('nav.group')
    expect(filters.priorities).toEqual([2])
    expect(groupMode.value).toBe('tag')
  })

  it('drops the sprint scope when a milestone filter is built over it', () => {
    // The tree's single-sprint scope is server-side; a multi-sprint filter is
    // client-side over the full board, so the two cannot both be on.
    const { api, milestoneScope, clearMilestoneScope } = facets()
    api.onAddFacet('fm.m1')
    expect(clearMilestoneScope).not.toHaveBeenCalled()

    milestoneScope.value = 'sprint-1'
    api.onAddFacet('fm.m2')
    expect(clearMilestoneScope).toHaveBeenCalledTimes(1)
    // Other facets leave the scope alone.
    api.onAddFacet('fp.1')
    expect(clearMilestoneScope).toHaveBeenCalledTimes(1)
  })

  it('toggles grouping and sort direction from a chip click', () => {
    const { api, groupMode } = facets()
    api.onChipClick({ kind: 'group' })
    expect(groupMode.value).toBe('tag')
    api.onChipClick({ kind: 'group' })
    expect(groupMode.value).toBe('status')

    api.onAddFacet('s.due')
    api.onChipClick({ kind: 'sort', i: 0 })
    expect(api.sortLevels.value[0].dir).toBe('desc')
  })

  it('removes a sort level by identity, so a reorder cannot hit the wrong one', () => {
    // Sort chips are drag-reorderable: an index captured when the chip rendered
    // may point at a different level by the time the «×» is clicked.
    const { api } = facets()
    api.onAddFacet('s.priority')
    api.onAddFacet('s.due')
    const due = api.sortLevels.value[1]
    api.sortLevels.value = [due, api.sortLevels.value[0]] // dragged into place
    api.removeSort(due)
    expect(api.sortLevels.value).toEqual([{ field: 'priority', dir: 'asc' }])

    api.toggleSortDir(api.sortLevels.value[0])
    expect(api.sortLevels.value[0].dir).toBe('desc')
  })

  it('clears filters and sort together, and reports when there is anything to clear', () => {
    const { api, filters } = facets()
    expect(api.hasClearableFacets.value).toBe(false)

    filters.q = '  ' // a whitespace-only search narrows nothing
    expect(api.hasClearableFacets.value).toBe(false)
    filters.q = 'api'
    expect(api.hasClearableFacets.value).toBe(true)

    api.onAddFacet('fa.u1')
    api.onAddFacet('s.due')
    api.clearAll()
    expect(api.hasClearableFacets.value).toBe(false)
    expect(filters.assignees).toEqual([])
    expect(filters.q).toBe('')
    expect(api.sortLevels.value).toEqual([])
  })

  it('removes one filter value per chip and clears a single-valued facet whole', () => {
    const { api, filters } = facets()
    filters.tags.push('t1', 't2')
    filters.due = 'today'
    api.removeChip({ kind: 'tag', value: 't1' })
    api.removeChip({ kind: 'due', value: 'today' })
    expect(filters.tags).toEqual(['t2'])
    expect(filters.due).toBe('')
  })
})

describe('useBoardFacets — sort comparison', () => {
  const cmp = (api, a, b, lvl) => Math.sign(api.cmpLevel(a, b, lvl))

  it('keeps date-less tasks last in both directions', () => {
    const { api } = facets()
    const dated = { due_date: '2026-01-01' }
    const undated = {}
    for (const dir of ['asc', 'desc']) {
      expect(cmp(api, dated, undated, { field: 'due', dir })).toBe(-1)
      expect(cmp(api, undated, dated, { field: 'due', dir })).toBe(1)
    }
    expect(cmp(api, undated, {}, { field: 'due', dir: 'asc' })).toBe(0)
  })

  it('sorts by milestone due date first, then title, with no milestone last', () => {
    const { api, milestonesMap } = facets()
    milestonesMap.early = { title: 'Б', due_date: '2026-01-01' }
    milestonesMap.late = { title: 'А', due_date: '2026-06-01' }
    milestonesMap.undated = { title: 'А' }
    const lvl = { field: 'milestone', dir: 'asc' }
    expect(cmp(api, { milestone_id: 'early' }, { milestone_id: 'late' }, lvl)).toBe(-1)
    // Same (missing) date → the title decides.
    expect(cmp(api, { milestone_id: 'undated' }, {}, lvl)).toBe(1)
    expect(cmp(api, {}, {}, lvl)).toBe(0)
  })

  it('sorts by the column position, not the column id', () => {
    const { api, timelineLike } = facets()
    timelineLike.value = true
    const lvl = { field: 'status', dir: 'asc' }
    expect(cmp(api, { column_id: 'c2' }, { column_id: 'c1' }, lvl)).toBe(1)
    // An unknown column sorts as position 0 rather than throwing.
    expect(cmp(api, { column_id: 'gone' }, { column_id: 'c1' }, lvl)).toBe(-1)
    expect(api.cmpLevel({}, {}, { field: 'unknown', dir: 'asc' })).toBe(0)
  })
})
