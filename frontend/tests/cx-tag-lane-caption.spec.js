// A tag lane is captioned the same everywhere it is drawn (#2932).
//
// Tags are stored as "scope::value" and that string is not a caption: the board's
// tag columns run it through `tagParts` + the project's friendly prefix names, and
// when the grouping is already narrowed to one namespace («Тег · Сложность») they
// drop the scope, because the group label above the board already says it.
//
// The Timeline and Gantt swimlanes stand for exactly the same tags and printed
// `tag.name` verbatim — in the one place least able to carry it. Their left column
// is a fixed 224px (LEFT_W) and `.lane-name` is `text-overflow: ellipsis`, so under
// a narrowed prefix every lane began with the same long namespace and the value that
// tells them apart was what got cut off. The caption had no `title` either, so there
// was nothing to hover to read it — the board's `.col-title` got one for this reason.
//
// Three halves are pinned, because any of them alone can be green while the lane is
// still unreadable: the shared rule itself, that the chart lanes go through it (with
// the board's own column caption asserted from the same rule, so the two cannot
// drift), and that the clipped caption carries a `title`.
import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

import { tagLaneLabel } from '@/utils/tagGroups'
import { useChartLanes } from '@/composables/useChartLanes'
import ChartLaneHeader from '@/components/chart/ChartLaneHeader.vue'

const root = process.cwd()

const HARD = { id: 't1', name: 'сложность::высокая', color: '#e0533d' }
const EASY = { id: 't2', name: 'сложность::низкая', color: '' }
const FLAT = { id: 't3', name: 'urgent', color: '' }
const NAMES = { 'сложность::': 'Сложность' }

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

beforeEach(() => setActivePinia(createPinia()))

describe('tagLaneLabel', () => {
  it('spells the friendly scope out ahead of the value when nothing narrows it', () => {
    expect(tagLaneLabel(HARD, { prefixNames: NAMES })).toBe('Сложность: высокая')
  })

  it('falls back to the raw scope when the project named no label for it', () => {
    expect(tagLaneLabel(HARD)).toBe('сложность: высокая')
  })

  it('leaves only the value once the grouping is narrowed to that namespace', () => {
    expect(tagLaneLabel(HARD, { tagPrefix: 'сложность::', prefixNames: NAMES })).toBe('высокая')
  })

  it('shows a prefix-less tag as it is', () => {
    expect(tagLaneLabel(FLAT, { prefixNames: NAMES })).toBe('urgent')
  })

  it('keeps a half-written tag whole rather than dropping a side of it', () => {
    // "effort::" / "::small" have no usable split — tagParts hands the raw name back.
    expect(tagLaneLabel({ name: 'сложность::' }, { prefixNames: NAMES })).toBe('сложность::')
    expect(tagLaneLabel({ name: '::высокая' })).toBe('::высокая')
  })

  it('survives a missing tag', () => {
    expect(tagLaneLabel(null)).toBe('')
  })
})

describe('chart swimlanes use the same caption as the board', () => {
  const tasks = [
    { id: 'a', tag_ids: ['t1'] },
    { id: 'b', tag_ids: ['t2'] },
  ]
  const tagsMap = { t1: HARD, t2: EASY }

  it('never labels a lane with the stored scope::value', () => {
    const lanes = chartLanes(tasks, { tagsMap, prefixNames: NAMES })
    expect(lanes.map((l) => l.label)).toEqual(['Сложность: высокая', 'Сложность: низкая'])
  })

  it('drops the namespace once the grouping is narrowed to it', () => {
    const lanes = chartLanes(tasks, {
      tagsMap,
      prefixNames: NAMES,
      tagPrefix: 'сложность::',
    })
    // The values are what distinguishes the lanes — and the part that used to be
    // ellipsised away behind a repeated prefix.
    expect(lanes.map((l) => l.label)).toEqual(['высокая', 'низкая'])
  })

  it('keeps the lane colour and the catch-all lane untouched', () => {
    const lanes = chartLanes([...tasks, { id: 'c', tag_ids: [] }], { tagsMap, prefixNames: NAMES })
    expect(lanes[0].color).toBe('#e0533d')
    expect(lanes[lanes.length - 1].key).toBe('∅')
    expect(lanes[lanes.length - 1].label).not.toBe('')
  })

  it('is the very rule the board captions its tag columns with', () => {
    // KanbanBoard.tagColumnName must go through tagLaneLabel rather than keep a
    // second copy of the split — that copy is how the two drifted apart.
    const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
    expect(board).toMatch(/function tagColumnName\(tag\)\s*\{\s*return tagLaneLabel\(/)
  })
})

describe('the clipped lane caption can be read', () => {
  it('carries the full label as a title', () => {
    const w = mount(ChartLaneHeader, {
      props: {
        lane: { key: 't1', label: 'Сложность: высокая', color: '#e0533d', tasks: [] },
        leftW: 224,
        axisW: 600,
        tier: 'day',
      },
    })
    const name = w.get('.lane-name')
    expect(name.text()).toBe('Сложность: высокая')
    expect(name.attributes('title')).toBe('Сложность: высокая')
  })
})
