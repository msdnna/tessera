// Narrowing the tag grouping to one namespace is a CANONICAL question (#2932).
//
// Tags are free text, so the same namespace gets spelled both ways on a real board
// («Сложность::высокая» typed once, «сложность::низкая» the next day — and a GitLab
// sync brings in whatever casing the labels carry over there). Everything that
// answers "which namespace is this tag in" folds that away through `canonPrefix`
// (trim + lowercase): the backend's prefix→label registry (handlers/tag_prefixes.go),
// `buildTagGroups` behind the composer's tag menu, the friendly scope on every pill,
// the GitLab meta-prefix hiding.
//
// The lane narrowing — the board's killer feature, «Тег · Сложность» — did not: it
// asked `tag.name.startsWith(tagPrefix)`, raw. So one namespace became two: the «+»
// menu offered the grouping twice (both entries carrying the SAME friendly label,
// since that lookup is canonical), each drawing only the tags of its own casing,
// and every card whose tag was spelled the other way fell into «Без тегов» — the
// catch-all, usually already the fullest lane. The Timeline/Gantt swimlanes carried
// a copy of the same raw test.
//
// Pinned here: the shared membership rule, both surfaces asking it (the board's
// `groupTags` by source — KanbanBoard pulls the whole board stack and is never
// mounted in this suite, same as cx-tag-prefix-collapse-scope — and the chart lanes
// by behaviour), and the «+» menu offering the namespace once.
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, reactive, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

import { tagInPrefix } from '@/utils/tagGroups'
import { useChartLanes } from '@/composables/useChartLanes'
import { useBoardFacets } from '@/composables/useBoardFacets'

const root = process.cwd()

const HARD = { id: 't1', name: 'Сложность::высокая', color: '#e0533d' }
const EASY = { id: 't2', name: 'сложность::низкая', color: '' }
const OTHER = { id: 't3', name: 'область::бэкенд', color: '' }

beforeEach(() => setActivePinia(createPinia()))

describe('tagInPrefix — membership in a namespace', () => {
  it('holds whichever way either side is capitalised', () => {
    expect(tagInPrefix('Сложность::высокая', 'сложность::')).toBe(true)
    expect(tagInPrefix('сложность::низкая', 'Сложность::')).toBe(true)
    expect(tagInPrefix('сложность::низкая', 'сложность::')).toBe(true)
  })

  it('folds the separator spacing away, like canonPrefix does', () => {
    // The "S: value" form: the namespace carries a trailing space, the registry
    // (and the GitLab rule editor) stores it trimmed.
    expect(tagInPrefix('S: bug', 'S: ')).toBe(true)
    expect(tagInPrefix('S: bug', 's:')).toBe(true)
  })

  it('answers about the namespace, not about the first letters of the name', () => {
    // A raw `startsWith` said yes to both of these.
    expect(tagInPrefix('типовой отчёт', 'тип')).toBe(false)
    expect(tagInPrefix('типаж::модель', 'тип')).toBe(false)
  })

  it('leaves a prefix-less tag out of every namespace', () => {
    expect(tagInPrefix('urgent', 'сложность::')).toBe(false)
    expect(tagInPrefix('', 'сложность::')).toBe(false)
    expect(tagInPrefix(undefined, 'сложность::')).toBe(false)
  })

  it('takes an empty prefix as "no narrowing at all"', () => {
    expect(tagInPrefix('urgent', '')).toBe(true)
    expect(tagInPrefix('сложность::низкая', '')).toBe(true)
  })
})

function host(setup) {
  let api
  mount(
    defineComponent({
      setup() {
        api = setup()
        return () => h('div')
      },
    }),
  )
  return api
}

function chartLanes(tasks, opts = {}) {
  return host(() =>
    useChartLanes({
      source: computed(() => tasks),
      statusColumns: ref([]),
      membersMap: ref({}),
      tagsMap: ref(opts.tagsMap || {}),
      groupMode: ref('tag'),
      tagPrefix: ref(opts.tagPrefix || ''),
      prefixNames: ref(opts.prefixNames || {}),
      estCfg: ref({ unit: 'hours' }),
    }),
  ).lanes.value
}

describe('chart swimlanes under a narrowed namespace', () => {
  const tasks = [
    { id: 'a', tag_ids: ['t1'] },
    { id: 'b', tag_ids: ['t2'] },
  ]
  const tagsMap = { t1: HARD, t2: EASY, t3: OTHER }

  it('draws a lane for both spellings instead of dropping one into the catch-all', () => {
    const lanes = chartLanes(tasks, { tagsMap, tagPrefix: 'сложность::' })
    expect(lanes.map((l) => l.key)).toEqual(['t1', 't2'])
    // '∅' is the «без тега» bucket — the card whose tag was capitalised used to land
    // there, next to a lane standing for the very same namespace.
    expect(lanes.some((l) => l.key === '∅')).toBe(false)
  })

  it('still keeps another namespace out of the lanes', () => {
    const lanes = chartLanes([...tasks, { id: 'c', tag_ids: ['t3'] }], {
      tagsMap,
      tagPrefix: 'сложность::',
    })
    expect(lanes.map((l) => l.key)).toEqual(['t1', 't2', '∅'])
    expect(lanes.find((l) => l.key === '∅').tasks.map((t) => t.id)).toEqual(['c'])
  })
})

describe('the board asks the same question', () => {
  const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
  const chart = readFileSync(resolve(root, 'src/composables/useChartLanes.js'), 'utf8')

  it('builds its tag lanes through the shared rule', () => {
    expect(board).toMatch(/tagsList\.value\.filter\(\(tag\) => tagInPrefix\(tag\.name, tagPrefix/)
  })

  it('keeps no raw prefix test of its own on either surface', () => {
    // A second spelling of "is this tag in the namespace" is how the two drifted.
    for (const src of [board, chart]) {
      expect(src).not.toMatch(/startsWith\(\s*tagPrefix/)
    }
  })
})

function facets(over = {}) {
  const ctx = {
    groupMode: ref('tag'),
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
    clearMilestoneScope: () => {},
    ...over,
  }
  return useBoardFacets(ctx)
}

describe('the «+» menu offers a namespace once', () => {
  it('collapses the casings into a single grouping entry', () => {
    const api = facets({
      tagsList: ref([HARD, EASY, OTHER]),
      tagPrefixNames: reactive({ 'сложность::': 'Сложность', 'область::': 'Область' }),
    })
    const opts = api.tagPrefixOptions.value
    // '' («все теги») + one entry per namespace.
    expect(opts.map((o) => o.label)).toEqual(['Все теги', 'Область', 'Сложность'])
    // Two identical «Тег · Сложность» entries, each narrowing to half the tags, is
    // exactly what the raw Set of namespaces produced.
    expect(opts.filter((o) => o.label === 'Сложность')).toHaveLength(1)
  })
})
