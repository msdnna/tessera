import { describe, it, expect, afterEach, vi } from 'vitest'
import { ref, reactive } from 'vue'
import { i18n, setI18nLocale } from '@/i18n'
import { useBoardFacets } from '@/composables/useBoardFacets'
import { useChartLanes } from '@/composables/useChartLanes'
import { priorityLabel } from '@/utils/priority'

// The board's labels used to live in module-level constants, evaluated once at
// import time (pitfall 1 of the #2799 plan): translating them «in place» would
// have looked right until the user switched language, at which point every menu
// and chip stayed on the language of the first render. These tests switch the
// locale at runtime and demand the text follow.

afterEach(async () => {
  await setI18nLocale('ru')
})

function facets() {
  return useBoardFacets({
    groupMode: ref('status'),
    tagPrefix: ref(''),
    timelineLike: ref(false),
    columns: ref([]),
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
  })
}

describe('board labels follow a language switch', () => {
  it('re-renders composer chips and the «+» menu in the new language', async () => {
    const api = facets()
    api.onAddFacet('s.priority')
    expect(api.groupChip.value.label).toBe('Группировка: Статус')
    expect(api.facetChips.value[1].text).toBe('Приоритет ↑')
    expect(api.addOptions.value[0].label).toBe('Группировка')

    await setI18nLocale('en')
    expect(api.groupChip.value.label).toBe('Grouping: Status')
    expect(api.facetChips.value[1].text).toBe('Priority ↑')
    expect(api.addOptions.value[0].label).toBe('Grouping')
  })

  it('re-labels the fallback swimlane of an unassigned task', async () => {
    const lanes = useChartLanes({
      source: ref([{ id: 't1', assignee_ids: [] }]),
      statusColumns: ref([]),
      membersMap: ref({}),
      tagsMap: ref({}),
      groupMode: ref('assignee'),
      tagPrefix: ref(''),
      estCfg: ref({}),
    }).lanes
    expect(lanes.value[0].label).toBe('Не назначено')

    await setI18nLocale('en')
    expect(lanes.value[0].label).toBe('Unassigned')
  })

  it('re-labels a priority', async () => {
    expect(priorityLabel(4)).toBe('Срочный')
    await setI18nLocale('en')
    expect(priorityLabel(4)).toBe('Urgent')
  })
})

describe('Russian plural forms on the chart counters', () => {
  // `${n} связей` was hardcoded — right for 5, wrong for 1 and 2.
  it.each([
    [1, '1 связь'],
    [2, '2 связи'],
    [5, '5 связей'],
    [21, '21 связь'],
  ])('%i → %s', (n, expected) => {
    expect(i18n.global.t('board.chart.counter.deps', n, { named: { n } })).toBe(expected)
  })
})
