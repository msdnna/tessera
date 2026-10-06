import { describe, it, expect } from 'vitest'
import { glAssigneesOf, glPickerMembers } from '../src/utils/assignees'

// #2932 — the GitLab half of the assignee picker was built from the project roster
// alone. The roster is pruned to the project's current membership on every refresh,
// while a hand-pinned assignee (source='user') survives every sync: someone who left
// the GitLab project stayed assigned to the task, was drawn on the card, and had no
// row in the picker to be un-assigned from.

const roster = [
  { gl_user_id: 1, gl_username: 'anna', gl_name: 'Анна Петрова', gl_avatar_url: '/a.png' },
  { gl_user_id: 2, gl_username: 'boris', gl_name: 'Борис Ким', gl_avatar_url: '' },
]

describe('glPickerMembers', () => {
  it('keeps the roster as-is when nobody is assigned', () => {
    const out = glPickerMembers(roster, [])
    expect(out.map((m) => m.gl_username)).toEqual(['anna', 'boris'])
    expect(out[0]).toEqual({
      gl_username: 'anna',
      gl_name: 'Анна Петрова',
      gl_avatar_url: '/a.png',
    })
  })

  it('appends a task assignee the roster no longer holds', () => {
    const task = { gitlab_assignee_logins: ['anna', 'quit'] }
    const out = glPickerMembers(roster, glAssigneesOf(task, roster))
    expect(out.map((m) => m.gl_username)).toEqual(['anna', 'boris', 'quit'])
    // The roster has nothing to call them, so the login stands in for the name —
    // exactly what the card's avatar is labelled with.
    expect(out[2]).toEqual({ gl_username: 'quit', gl_name: 'quit', gl_avatar_url: '' })
  })

  it('does not duplicate an assignee who is still in the roster', () => {
    const task = { gitlab_assignee_logins: ['anna', 'boris'] }
    const out = glPickerMembers(roster, glAssigneesOf(task, roster))
    expect(out.map((m) => m.gl_username)).toEqual(['anna', 'boris'])
  })

  it('accepts the task detail shape (gl_username/gl_name/gl_avatar_url)', () => {
    const detail = [{ gl_username: 'quit', gl_name: 'Ушедший', gl_avatar_url: '/q.png' }]
    const out = glPickerMembers(roster, detail)
    expect(out[2]).toEqual({
      gl_username: 'quit',
      gl_name: 'Ушедший',
      gl_avatar_url: '/q.png',
    })
  })

  it('skips a legacy name-only assignee (no login → nothing to toggle)', () => {
    const task = { gitlab_assignees: ['Кто-то Из Прошлого'] }
    const out = glPickerMembers(roster, glAssigneesOf(task, roster))
    expect(out.map((m) => m.gl_username)).toEqual(['anna', 'boris'])
  })

  it('filters by display name and by login', () => {
    const task = { gitlab_assignee_logins: ['quit'] }
    const assigned = glAssigneesOf(task, roster)
    expect(glPickerMembers(roster, assigned, 'борис').map((m) => m.gl_username)).toEqual(['boris'])
    expect(glPickerMembers(roster, assigned, 'ANN').map((m) => m.gl_username)).toEqual(['anna'])
    expect(glPickerMembers(roster, assigned, 'qui').map((m) => m.gl_username)).toEqual(['quit'])
    expect(glPickerMembers(roster, assigned, '  ').map((m) => m.gl_username)).toEqual([
      'anna',
      'boris',
      'quit',
    ])
  })

  it('survives empty / missing input', () => {
    expect(glPickerMembers()).toEqual([])
    expect(glPickerMembers(null, null, null)).toEqual([])
    expect(glPickerMembers([{ gl_name: 'без логина' }], [])).toEqual([])
  })
})
