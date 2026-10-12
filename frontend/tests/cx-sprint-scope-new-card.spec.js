import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { newTaskMilestoneId } from '@/utils/milestones'

// Regression test for #2932 — a card typed on a sprint-scoped board vanished.
//
// `?milestone=` narrows the board SERVER-side (backend handlers/tasks.go →
// ListBoardTasksWithMeta), and both create paths in KanbanBoard sent nothing but
// `column_id` + `title`. So on a board scoped to «Спринт 7» the reader typed a card
// into a column, `load()` re-read the list with that same `?milestone=`, and the card
// was not in the answer: it disappeared from the column with no error and no clue it
// had been created at all (it was sitting in the backlog). The milestone GROUPING has
// always stamped a new card with its lane's milestone — the scope was the one promise
// not kept.
//
// The first half of this file is the round trip the reader makes, played against a
// backend stub that filters exactly the way the query does. The second pins the
// board's wiring: KanbanBoard is never mounted in this suite (it pulls the whole
// board stack), so the guard reads the source the way cx-tag-prefix-collapse-scope
// does.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// The body of `function <name>(…) { … }`. The opening brace is looked for AFTER the
// parameter list closes: `createInQuadrant({ title, quadrant })` destructures, so the
// first `{` in the declaration belongs to the params, not to the body.
function fnBody(source, name) {
  const start = source.indexOf(`function ${name}(`)
  if (start === -1) return null
  let params = 0
  let i = source.indexOf('(', start)
  for (; i < source.length; i++) {
    if (source[i] === '(') params++
    else if (source[i] === ')' && --params === 0) break
  }
  const open = source.indexOf('{', i)
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

const SPRINT = 'ms-sprint-7'
const OTHER = 'ms-sprint-8'

// The server's side of the trip: create a card, then list the board the way the open
// view is scoped. `scope` mirrors patchScope()'s vocabulary — absent = unscoped,
// null = ?milestone=backlog, an id = that sprint.
function backend() {
  const rows = []
  return {
    create(milestoneId) {
      const row = { id: `t${rows.length + 1}`, milestone_id: milestoneId || null }
      rows.push(row)
      return row
    },
    // db/queries/tasks.sql: `t.milestone_id = @milestone_id`, or IS NULL for backlog.
    list(scope) {
      if (scope === undefined) return rows
      return rows.filter((r) => r.milestone_id === scope)
    },
  }
}

// What the board does on «+ задача»: decide the milestone, create, stamp, reload.
function typeCard(api, { groupMode, lane, scopeId } = {}) {
  const milestoneId = newTaskMilestoneId({ groupMode, lane, scopeId })
  const row = api.create(milestoneId)
  const visible = api.list(scopeId === '' ? undefined : scopeId)
  return { row, stillDrawn: visible.some((r) => r.id === row.id) }
}

describe('a card typed on a sprint-scoped board (#2932)', () => {
  it('stays drawn after the reload that follows', () => {
    const api = backend()
    const { row, stillDrawn } = typeCard(api, { groupMode: 'status', scopeId: SPRINT })
    expect(row.milestone_id).toBe(SPRINT)
    expect(stillDrawn).toBe(true)
  })

  it('is born in the sprint the reader is looking at, not another one', () => {
    const api = backend()
    api.create(OTHER)
    const { row } = typeCard(api, { groupMode: 'status', scopeId: SPRINT })
    expect(row.milestone_id).toBe(SPRINT)
    expect(api.list(OTHER)).toHaveLength(1)
  })

  it('carries no milestone on the backlog scope', () => {
    // ?milestone=backlog lists cards WITHOUT a milestone, so stamping one would be
    // the same bug mirrored — the card would vanish out of the backlog instead.
    const api = backend()
    const { row, stillDrawn } = typeCard(api, { groupMode: 'status', scopeId: null })
    expect(row.milestone_id).toBe(null)
    expect(stillDrawn).toBe(true)
  })

  it('carries no milestone on an unscoped board', () => {
    const api = backend()
    const { row, stillDrawn } = typeCard(api, { groupMode: 'status', scopeId: undefined })
    expect(row.milestone_id).toBe(null)
    expect(stillDrawn).toBe(true)
  })

  it('leaves an unresolvable scope alone', () => {
    // A stale or broken `?milestone=<slug>` resolves to '' (patchScope). There is no
    // id to stamp, and inventing one would file the card under a sprint nobody named.
    expect(newTaskMilestoneId({ groupMode: 'status', scopeId: '' })).toBe(null)
  })

  it('lets the milestone lane decide when the board groups by milestone', () => {
    // Typing into «Спринт 8»'s lane while scoped to «Спринт 7» means the lane: that
    // is where the reader put the card, and it is the older of the two rules.
    expect(
      newTaskMilestoneId({
        groupMode: 'milestone',
        lane: { milestone: { id: OTHER } },
        scopeId: SPRINT,
      }),
    ).toBe(OTHER)
  })

  it('honours the «Без этапа» lane over the scope', () => {
    // Saying "no milestone" out loud beats the address bar.
    expect(
      newTaskMilestoneId({ groupMode: 'milestone', lane: { milestone: null }, scopeId: SPRINT }),
    ).toBe(null)
  })

  it('still stamps a tag lane card with the scope', () => {
    // Tag grouping has no milestone of its own — the lane adds a tag, the scope the
    // sprint, and the two do not compete.
    expect(
      newTaskMilestoneId({ groupMode: 'tag', lane: { tag: { id: 'tag-1' } }, scopeId: SPRINT }),
    ).toBe(SPRINT)
  })

  it('scopes the matrix quick-add by the address alone', () => {
    // The matrix draws no lanes, so a grouping carried in from the board layout must
    // not be consulted there — only the scope.
    expect(newTaskMilestoneId({ scopeId: SPRINT })).toBe(SPRINT)
    expect(newTaskMilestoneId({})).toBe(null)
    expect(newTaskMilestoneId()).toBe(null)
  })
})

describe("the board's wiring of it (#2932)", () => {
  it('derives the stamped id from the very scope the list is filtered by', () => {
    // Two spellings of "which sprint is this board showing" would drift, and the drift
    // is invisible: the card is created, just not listed.
    const body = fnBody(board, 'scopeMilestoneId')
    expect(body).toBeTruthy()
    expect(body).toContain('patchScope()')
  })

  it('stamps the card typed into a column', () => {
    const body = fnBody(board, 'submitAddTask')
    expect(body).toMatch(/newTaskMilestoneId\(\{[\s\S]*?scopeId:\s*scopeMilestoneId\(\)/)
    expect(body).toContain('groupMode: groupMode.value')
    expect(body).toContain('lane: dcol')
    expect(body).toContain('if (milestoneId) await tasksApi.setMilestone(res.data.id, milestoneId)')
    // The old lane-only rule must be gone, not standing beside the new one: two
    // setMilestone calls on one create would race the same field.
    expect(body).not.toContain("groupMode.value === 'milestone' && dcol.milestone")
  })

  it('stamps the card typed into a matrix quadrant', () => {
    const body = fnBody(board, 'createInQuadrant')
    expect(body).toContain('newTaskMilestoneId({ scopeId: scopeMilestoneId() })')
    expect(body).toContain('if (milestoneId) await tasksApi.setMilestone(res.data.id, milestoneId)')
  })

  it('decides before the create, not after the input is cleared', () => {
    // submitAddTask empties `newTaskTitle` before awaiting; the milestone must be
    // resolved while the board state is still the one the reader was looking at.
    const body = fnBody(board, 'submitAddTask')
    expect(body.indexOf('newTaskMilestoneId')).toBeLessThan(body.indexOf('suppress()'))
  })
})
