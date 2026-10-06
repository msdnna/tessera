import { describe, it, expect } from 'vitest'
import { matchesAssignee } from '@/utils/boardFilters'
import { filterBoardTasks, matchesTask } from '@/utils/taskFilter'
import { emptyFilters } from '@/utils/facetKeys'

// Filtering the board by «Исполнитель» when the person's GitLab login is what the
// task carries (#2932).
//
// The sync resolves a GitLab assignee to a Tessera account whenever it can, so the
// two normally do not coexist on one card. A login stays behind when the account was
// not resolvable at the moment the row was written — a member pinned by hand before
// they had one (those rows survive every sync rebuild), or an issue synced before the
// person linked their account and untouched in GitLab since. From the moment they ARE
// linked, the filter menu stops offering their `gl:` entry (the GitLab roster drops
// members that map to a workspace member, so nobody is listed twice) — so without the
// login lookup the card answers to no value of the facet at all.
//
// The author facet has always resolved the same way; these tests pin the pair.

const U1 = '11111111-1111-1111-1111-111111111111'
const U2 = '22222222-2222-2222-2222-222222222222'
const GL_BY_USER = { [U1]: 'john' }

const pinnedTask = (over = {}) => ({
  id: 't1',
  title: 'Синхронизированная задача',
  column_id: 'c1',
  assignee_ids: [],
  gitlab_assignee_logins: ['john'],
  ...over,
})

describe('matchesAssignee + linked GitLab login', () => {
  it('matches the person by their linked login', () => {
    expect(matchesAssignee(pinnedTask(), [U1], GL_BY_USER)).toBe(true)
  })

  it('does not match a different person through the map', () => {
    expect(matchesAssignee(pinnedTask(), [U2], GL_BY_USER)).toBe(false)
  })

  it('still matches the bare gl: value (the login may be offered)', () => {
    expect(matchesAssignee(pinnedTask(), ['gl:john'], GL_BY_USER)).toBe(true)
  })

  it('keeps the plain uuid path for a Tessera assignee', () => {
    const t = pinnedTask({ assignee_ids: [U2], gitlab_assignee_logins: [] })
    expect(matchesAssignee(t, [U2], GL_BY_USER)).toBe(true)
    expect(matchesAssignee(t, [U1], GL_BY_USER)).toBe(false)
  })

  it('without a map it is the old behaviour', () => {
    expect(matchesAssignee(pinnedTask(), [U1])).toBe(false)
  })
})

describe('board filtering by assignee', () => {
  const filters = () => ({ ...emptyFilters(), assignees: [U1] })

  it('keeps the card whose assignee is stored as a GitLab login', () => {
    const r = filterBoardTasks({
      tasks: [pinnedTask()],
      filters: filters(),
      glLoginByUserId: GL_BY_USER,
    })
    expect(r.tasks.map((t) => t.id)).toEqual(['t1'])
  })

  it('drops it when the login belongs to someone else', () => {
    const r = filterBoardTasks({
      tasks: [pinnedTask({ gitlab_assignee_logins: ['jane'] })],
      filters: filters(),
      glLoginByUserId: GL_BY_USER,
    })
    expect(r.tasks).toEqual([])
  })

  it('lifts a parent whose SUBTASK carries the login', () => {
    const parent = pinnedTask({ id: 'p1', gitlab_assignee_logins: [] })
    const sub = pinnedTask({ id: 's1', parent_id: 'p1' })
    const r = filterBoardTasks({
      tasks: [parent],
      subtasksByParent: { p1: [sub, pinnedTask({ id: 's2', gitlab_assignee_logins: ['jane'] })] },
      filters: filters(),
      glLoginByUserId: GL_BY_USER,
    })
    expect(r.tasks.map((t) => t.id)).toEqual(['p1'])
    expect(r.subtasksByParent.p1.map((s) => s.id)).toEqual(['s1'])
    expect(r.narrowedParents.has('p1')).toBe(true)
  })

  it('matchesTask resolves the login the same way', () => {
    expect(matchesTask(pinnedTask(), filters(), { glLoginByUserId: GL_BY_USER })).toBe(true)
    expect(matchesTask(pinnedTask(), filters(), {})).toBe(false)
  })
})
