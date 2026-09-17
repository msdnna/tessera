import { describe, it, expect, vi } from 'vitest'
import { ref, reactive } from 'vue'

import { useBoardFacets } from '@/composables/useBoardFacets'
import { GROUP_MODES, decodeGroup } from '@/utils/facetKeys'

// The composer's grouping chip is a quick switch: one click steps the board to the
// next grouping. It used to be a status⇄tag *flip*, written when those were the only
// two modes. The board has five now — milestone lanes are a first-class column mode
// of the ordinary board (a task typed into one is stamped with that milestone), and
// the time-axis layouts default to `assignee` swimlanes. A flip dropped any of the
// other three on the first click and offered no click back.

function facets(over = {}) {
  const ctx = {
    groupMode: ref('status'),
    tagPrefix: ref(''),
    timelineLike: ref(false),
    columns: ref([{ id: 'c1', name: 'К работе', position: 1 }]),
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
  return { api: useBoardFacets(ctx), ...ctx }
}
// Click the grouping chip n times, collecting the mode after each click.
function walk(api, groupMode, n) {
  const seen = []
  for (let i = 0; i < n; i++) {
    api.onChipClick({ kind: 'group' })
    seen.push(groupMode.value)
  }
  return seen
}

describe('grouping chip — cycles instead of flipping', () => {
  it('walks the board layout modes and returns to where it started', () => {
    const { api, groupMode } = facets()
    expect(walk(api, groupMode, 4)).toEqual(['tag', 'milestone', 'status', 'tag'])
  })

  it('leaves milestone grouping reachable again by clicking', () => {
    // The trap this guards: «Группировка: Этап» flipped to tags and then to
    // statuses, and no number of further clicks ever brought milestone lanes back.
    const { api, groupMode } = facets({ groupMode: ref('milestone') })
    expect(walk(api, groupMode, 3)).toEqual(['status', 'tag', 'milestone'])
  })

  it('walks the swimlane modes on a time-axis layout', () => {
    const { api, groupMode } = facets({ timelineLike: ref(true) })
    expect(walk(api, groupMode, 5)).toEqual(['tag', 'milestone', 'assignee', 'none', 'status'])
  })

  it('restores the timeline default grouping, which a flip could not', () => {
    // `assignee` is what defaultToolbar() hands a timeline/gantt view, so the very
    // first chip click on a fresh timeline used to leave its own default behind.
    const { api, groupMode } = facets({ groupMode: ref('assignee'), timelineLike: ref(true) })
    expect(groupMode.value).toBe('assignee')
    expect(walk(api, groupMode, 5)).toEqual(['none', 'status', 'tag', 'milestone', 'assignee'])
  })

  it('offers exactly the groupings the «+» menu does for the layout', () => {
    // Two ends, one list: a mode the menu can set but the cycle skips is a mode the
    // chip would silently drop (that is how milestone got stranded).
    for (const timeline of [false, true]) {
      const { api, groupMode } = facets({ timelineLike: ref(timeline) })
      const menu = api.addOptions.value
        .find((o) => o.key === 'group')
        .children.map((c) => decodeGroup(c.key).mode)
      const cycle = walk(api, groupMode, GROUP_MODES.length + 1)
      expect(new Set(cycle)).toEqual(new Set(menu))
    }
  })

  it('keeps the tag namespace, so leaving and coming back lands on the same lanes', () => {
    // `tagPrefix` narrows the tag lanes and is not a grouping of its own; the cycle
    // must not quietly widen «Тег · Область» into every tag on the way round.
    const { api, groupMode, tagPrefix, tagPrefixNames } = facets({
      groupMode: ref('tag'),
      tagPrefix: ref('area/'),
    })
    tagPrefixNames['area/'] = 'Область'
    expect(api.groupChip.value.text).toBe('Тег · Область')

    walk(api, groupMode, 3) // milestone → status → tag
    expect(groupMode.value).toBe('tag')
    expect(tagPrefix.value).toBe('area/')
    expect(api.groupChip.value.text).toBe('Тег · Область')
  })

  it('steps a stray mode back into this layout cycle instead of sticking', () => {
    // A view saved on a timeline carries `assignee`; opened under a board layout it
    // is outside the cycle, and the chip has to still lead somewhere.
    const { api, groupMode } = facets({ groupMode: ref('assignee') })
    expect(api.nextGroupMode.value).toBe('status')
    expect(walk(api, groupMode, 2)).toEqual(['status', 'tag'])
  })

  it('names the grouping the next click lands on, not a fixed pair', () => {
    // The chip's tooltip used to read «Переключить статусы/теги» while showing
    // «Этап» — the label is built from the next mode now.
    const { api, groupMode } = facets()
    expect(api.nextGroupModeLabel.value).toBe('Тег')
    groupMode.value = 'tag'
    expect(api.nextGroupModeLabel.value).toBe('Этап')
    groupMode.value = 'milestone'
    expect(api.nextGroupModeLabel.value).toBe('Статус')
  })
})
