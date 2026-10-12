// The «Проект» window's tag table helpers (#2990): GitLab lock, filter, groups,
// selection, the new-tag field and the default merge target.
import { describe, it, expect } from 'vitest'
import {
  ruledPrefixes,
  isRuled,
  matchesFilter,
  tagTableGroups,
  rowOrder,
  toggleSelected,
  selectRange,
  stepRow,
  newTagName,
  defaultMergeTarget,
  tagSource,
} from '@/utils/tagTable'

const TAGS = [
  { id: '1', name: 'app::frontend', task_count: 48 },
  { id: '2', name: 'app::backend', task_count: 37 },
  { id: '3', name: 'type::bug', task_count: 23, source: 'gitlab' },
  { id: '4', name: 'design', task_count: 17 },
  { id: '5', name: 'дизайн', task_count: 3 },
  { id: '6', name: 'ui', task_count: 5, source: 'gitlab' },
]
const INTEG = [
  {
    project_id: 'p1',
    enabled: true,
    label_rules: {
      rules: [
        { match: 'Type::', match_type: 'prefix', action: 'tag' },
        { match: '^prio', match_type: 'regex', action: 'priority' },
      ],
    },
  },
  { project_id: 'p2', enabled: true, label_rules: { rules: [{ match: 'app::' }] } },
  { project_id: 'p1', enabled: false, label_rules: { rules: [{ match: 'app::' }] } },
]

describe('tagTable', () => {
  it('locks the prefixes of the project’s enabled prefix rules, whatever the action', () => {
    const locked = ruledPrefixes(INTEG, 'p1')
    expect([...locked]).toEqual(['type::'])
    expect(isRuled(TAGS[2], locked)).toBe(true)
    expect(isRuled(TAGS[0], locked)).toBe(false)
    expect(tagSource(TAGS[2], locked)).toBe('rule')
    expect(tagSource(TAGS[5], locked)).toBe('gitlab')
    expect(tagSource(TAGS[3], locked)).toBe('user')
  })

  it('filters by name and by the group’s friendly name', () => {
    expect(matchesFilter(TAGS[0], 'FRONT')).toBe(true)
    expect(matchesFilter(TAGS[0], 'прил', { 'app::': 'Приложение' })).toBe(true)
    expect(matchesFilter(TAGS[3], 'прил', { 'app::': 'Приложение' })).toBe(false)
    expect(matchesFilter(TAGS[3], '')).toBe(true)
  })

  it('groups by prefix with the lock and the friendly name flag', () => {
    const groups = tagTableGroups(TAGS, {
      prefixNames: { 'app::': 'Приложение' },
      locked: ruledPrefixes(INTEG, 'p1'),
    })
    expect(groups.map((g) => g.key)).toEqual(['app::', 'type::', ''])
    expect(groups[0].named).toBe(true)
    expect(groups[1].ruled).toBe(true)
    expect(groups[2].ruled).toBe(false)
    expect(groups[2].tags.map((t) => t.name).sort()).toEqual(['design', 'ui', 'дизайн'].sort())
  })

  it('walks, toggles and ranges rows in table order', () => {
    const groups = tagTableGroups(TAGS)
    const order = rowOrder(groups, new Set(['type::']))
    expect(order).not.toContain('3')
    expect(stepRow(order, null, 1)).toBe(order[0])
    expect(stepRow(order, order[0], -1)).toBe(order[0])
    expect(stepRow(order, order[0], 2)).toBe(order[2])
    let sel = toggleSelected(new Set(), order[1])
    expect(sel.has(order[1])).toBe(true)
    sel = toggleSelected(sel, order[1])
    expect(sel.size).toBe(0)
    sel = selectRange(new Set(), order, order[3], order[1])
    expect([...sel].sort()).toEqual([order[1], order[2], order[3]].sort())
  })

  it('reads the new-tag field', () => {
    expect(newTagName('  app::mobile ')).toBe('app::mobile')
    expect(newTagName('app::')).toBe('')
    expect(newTagName('urgent')).toBe('urgent')
    expect(newTagName('   ')).toBe('')
  })

  it('merges into the most used tag by default', () => {
    expect(defaultMergeTarget([TAGS[4], TAGS[3], TAGS[5]]).name).toBe('design')
    expect(defaultMergeTarget([])).toBe(null)
  })
})
