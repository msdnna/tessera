// An assignee without a Tessera account is not «не назначено» (#2932).
//
// The backend splits the two halves of an assignee: a GitLab member linked to a
// Tessera user is written as an ordinary `task_assignees` row (so the board row's
// `assignee_ids` covers them), an unlinked one goes to `task_gitlab_assignees` and
// reaches the board only as `gitlab_assignee_logins`. Every other surface of the
// board knows that: the composer offers those people under «Исполнитель» as
// `gl:<login>` and filters by it (utils/taskFilter matchesAssignees), the card draws
// their face, the timeline's hover card lists them.
//
// The chart swimlanes read `assignee_ids` alone, so on a GitLab-synced board every
// such card was swept into the single «Не назначено» lane — and `assignee` is the
// grouping the Timeline and the Gantt OPEN with (KanbanBoard defaultToolbar).
//
// Two halves are checked, because either alone can be green while the feature is
// broken: the lane rule itself, and the roster actually reaching the composable —
// the two views take it as a prop from KanbanBoard, and a prop that is never bound
// fails silently (the lane would just fall back to the bare login).
import { describe, it, expect, beforeEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed, defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { useChartLanes } from '@/composables/useChartLanes'

const root = process.cwd()
const read = (p) => readFileSync(resolve(root, p), 'utf8')

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

const day = (n) => new Date(2026, 0, n).toISOString()
const task = (id, extra = {}) => ({
  id,
  title: id,
  start_date: day(10),
  due_date: day(12),
  ...extra,
})

function lanesFor(source, opts = {}) {
  return host(() =>
    useChartLanes({
      source: computed(() => source),
      statusColumns: ref([]),
      membersMap: ref(opts.membersMap || {}),
      glMembers: ref(opts.glMembers || []),
      tagsMap: ref({}),
      groupMode: ref('assignee'),
      tagPrefix: ref(''),
      estCfg: ref({ unit: 'hours' }),
    }),
  )
}

beforeEach(() => setActivePinia(createPinia()))

describe('assignee lanes and GitLab-only people', () => {
  it('gives a GitLab-only assignee their own lane, named from the roster', () => {
    const { lanes } = lanesFor(
      [
        task('a', { assignee_ids: [], gitlab_assignee_logins: ['ipetrov'] }),
        task('b', { assignee_ids: [], gitlab_assignee_logins: ['ipetrov'] }),
        task('c', { assignee_ids: [], gitlab_assignee_logins: [] }),
      ],
      { glMembers: [{ gl_username: 'ipetrov', gl_name: 'Иван Петров' }] },
    )
    // The lane key follows the composer's own `gl:` convention for these people.
    expect(lanes.value.map((l) => l.key)).toEqual(['gl:ipetrov', '∅'])
    expect(lanes.value[0].label).toBe('Иван Петров')
    expect(lanes.value[0].tasks.map((t) => t.id)).toEqual(['a', 'b'])
    // The card with nobody on it is still the one in the unassigned lane, still last.
    expect(lanes.value[1].tasks.map((t) => t.id)).toEqual(['c'])
  })

  it('falls back to the login when the roster does not know it', () => {
    const { lanes } = lanesFor([task('a', { gitlab_assignee_logins: ['ghost'] })])
    expect(lanes.value.map((l) => l.key)).toEqual(['gl:ghost'])
    expect(lanes.value[0].label).toBe('ghost')
  })

  it('keeps a linked member in their Tessera lane — one person, one lane', () => {
    // A linked GitLab member is assigned on BOTH sides (the sync writes the Tessera
    // row, the pin may leave the GitLab one). The Tessera identity wins, so the
    // person does not show up as two lanes.
    const { lanes } = lanesFor(
      [task('a', { assignee_ids: ['u1'], gitlab_assignee_logins: ['ipetrov'] })],
      {
        membersMap: { u1: { name: 'Иван Петров' } },
        glMembers: [{ gl_username: 'ipetrov', gl_name: 'Иван Петров' }],
      },
    )
    expect(lanes.value.map((l) => l.key)).toEqual(['u1'])
  })

  it('leaves the other groupings alone', () => {
    // The fallback is assignee-specific: a card with a GitLab login and no tags
    // still belongs to the no-tag lane, not to a `gl:` one.
    const { lanes } = host(() =>
      useChartLanes({
        source: computed(() => [task('a', { gitlab_assignee_logins: ['ipetrov'] })]),
        statusColumns: ref([]),
        membersMap: ref({}),
        glMembers: ref([{ gl_username: 'ipetrov', gl_name: 'Иван Петров' }]),
        tagsMap: ref({}),
        groupMode: ref('tag'),
        tagPrefix: ref(''),
        estCfg: ref({ unit: 'hours' }),
      }),
    )
    expect(lanes.value.map((l) => l.key)).toEqual(['∅'])
  })
})

describe('the roster reaches both charts', () => {
  const timeline = read('src/components/BoardTimelineView.vue')
  const gantt = read('src/components/BoardGanttView.vue')
  const board = read('src/components/KanbanBoard.vue')

  it('both views declare the prop and hand it to useChartLanes', () => {
    for (const src of [timeline, gantt]) {
      expect(src).toMatch(/gitlabMembers:\s*\{\s*type:\s*Array/)
      expect(src).toContain("glMembers: toRef(props, 'gitlabMembers')")
    }
  })

  it('KanbanBoard binds the roster on both of them', () => {
    const bindings = board.match(/:gitlab-members="gitlabMembersList"/g) || []
    expect(bindings).toHaveLength(2)
  })
})
