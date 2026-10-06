// The «Список» layout drew a GitLab-synced card as unassigned (#2932).
//
// The backend splits a task's assignees in two: a GitLab member linked to a
// Tessera user becomes an ordinary `task_assignees` row (so `assignee_ids` covers
// them), an unlinked one lands in `task_gitlab_assignees` and reaches the client
// only as `gitlab_assignee_logins`. Every other surface of the board knows that —
// the card draws their face (TaskCardPills), the chart lanes stand them in a lane
// of their own, the composer filters by `gl:<login>` — but the list rows read
// `assignee_ids` alone, and that is the layout where the people column is the only
// thing on the row saying who holds the card.
//
// Three halves, because any one of them alone can be green while the feature is
// broken: the resolver rule, the row actually rendering what it returns, and the
// roster reaching the view — a prop the board never binds fails silently (every
// GitLab assignee would just render as a bare login with no avatar).
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { glAssigneesOf } from '@/utils/assignees'

vi.mock('@/api', () => ({
  tasks: {
    update: vi.fn(),
    move: vi.fn(),
    remove: vi.fn(),
    archive: vi.fn(),
    addRelation: vi.fn(),
    removeRelation: vi.fn(),
  },
  boards: { dependencies: vi.fn(() => Promise.resolve({ data: [] })) },
}))

const { default: BoardListView } = await import('@/components/BoardListView.vue')

const root = process.cwd()
const read = (p) => readFileSync(resolve(root, p), 'utf8')

const ROSTER = [
  { gl_username: 'ivan', gl_name: 'Иван Петров', gl_avatar_url: '/api/gitlab/avatar?u=ivan' },
  { gl_username: 'olga', gl_name: 'Ольга Сидорова', gl_avatar_url: '' },
]

describe('glAssigneesOf', () => {
  it('resolves a login to the roster name and avatar', () => {
    expect(glAssigneesOf({ gitlab_assignee_logins: ['ivan'] }, ROSTER)).toEqual([
      { login: 'ivan', name: 'Иван Петров', avatar_url: '/api/gitlab/avatar?u=ivan' },
    ])
  })

  // A login the roster does not carry (member left the GitLab project, roster not
  // yet refetched) still has to name somebody — the login is what the reader has.
  it('falls back to the bare login when the roster misses them', () => {
    expect(glAssigneesOf({ gitlab_assignee_logins: ['petr'] }, ROSTER)).toEqual([
      { login: 'petr', name: 'petr', avatar_url: null },
    ])
  })

  // Rows synced before logins were carried have only the display names.
  it('reads the name-only shape when there are no logins', () => {
    expect(glAssigneesOf({ gitlab_assignees: ['Внешний Исполнитель'] }, ROSTER)).toEqual([
      { login: '', name: 'Внешний Исполнитель', avatar_url: null },
    ])
  })

  it('is empty for a card with no GitLab assignees, and for no card at all', () => {
    expect(glAssigneesOf({ assignee_ids: ['u1'] }, ROSTER)).toEqual([])
    expect(glAssigneesOf(null)).toEqual([])
  })

  // The roster is optional: the board loads it asynchronously, and a row rendered
  // before it lands must still show the person rather than throw.
  it('works without a roster', () => {
    expect(glAssigneesOf({ gitlab_assignee_logins: ['ivan'] })).toEqual([
      { login: 'ivan', name: 'ivan', avatar_url: null },
    ])
  })
})

describe('BoardListView people column', () => {
  beforeEach(() => setActivePinia(createPinia()))

  const mountList = (task) =>
    mount(BoardListView, {
      props: {
        columns: [{ key: 'c1', name: 'К работе', color: '' }],
        statusColumns: [{ id: 'c1', name: 'К работе' }],
        lists: { c1: [{ id: 't1', number: 7, title: 'Синхронизированная задача', ...task }] },
        membersMap: { u1: { user_id: 'u1', name: 'Мария' } },
        gitlabMembers: ROSTER,
      },
      global: { stubs: { TagPill: true } },
    })

  it('draws the GitLab assignee of a synced card', () => {
    const w = mountList({ gitlab_assignee_logins: ['ivan'] })
    const avas = w.findAll('.lv-ava')

    expect(avas).toHaveLength(1)
    expect(avas[0].attributes('title')).toBe('Иван Петров (GitLab)')
    w.unmount()
  })

  // Both halves of the same card's assignees, in that order: the Tessera account
  // first (accent fill), the external one after it (neutral) — the card's own
  // avatar row makes the same distinction.
  it('draws both halves, external ones marked apart', () => {
    const w = mountList({ assignee_ids: ['u1'], gitlab_assignee_logins: ['olga'] })
    const avas = w.findAll('.lv-ava')

    expect(avas).toHaveLength(2)
    expect(avas[0].classes()).not.toContain('ext')
    expect(avas[0].attributes('title')).toBe('Мария')
    expect(avas[1].classes()).toContain('ext')
    expect(avas[1].attributes('title')).toBe('Ольга Сидорова (GitLab)')
    w.unmount()
  })

  it('leaves a card with nobody on it empty', () => {
    const w = mountList({})

    expect(w.findAll('.lv-ava')).toHaveLength(0)
    w.unmount()
  })
})

describe('the board hands the roster to the list view', () => {
  it('binds :gitlab-members on BoardListView', () => {
    const src = read('src/components/KanbanBoard.vue')
    const tag = src.slice(src.indexOf('<BoardListView'))
    const block = tag.slice(0, tag.indexOf('/>'))

    expect(block).toContain(':gitlab-members="gitlabMembersList"')
  })
})
