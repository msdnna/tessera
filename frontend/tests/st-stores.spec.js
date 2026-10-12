import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

// One shared api mock for every store under test. vi.mock is hoisted, so the
// mock object must be created inside vi.hoisted to be referenceable.
const apiMock = vi.hoisted(() => ({
  auth: {
    login: vi.fn(),
    register: vi.fn(),
    me: vi.fn(),
    logout: vi.fn(() => Promise.resolve()),
  },
  // #2684: the access token lives in the api module, not localStorage, and the
  // session is restored from the refresh cookie on start-up.
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(() => ''),
  restoreSession: vi.fn(() => Promise.resolve(null)),
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  workspaces: {
    list: vi.fn(),
    remove: vi.fn(() => Promise.resolve()),
    groups: vi.fn(() => Promise.resolve({ data: [] })),
    projects: vi.fn(() => Promise.resolve({ data: [] })),
    commands: vi.fn(() =>
      Promise.resolve({ data: { builtin: [], custom: [], can_manage: false } }),
    ),
  },
  projects: { boards: vi.fn(() => Promise.resolve({ data: [] })) },
  gitlab: {
    getConnection: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(() => Promise.resolve()),
    conflicts: vi.fn(),
  },
}))
vi.mock('@/api', () => apiMock)

// useTreeExpand is pulled in by prefetchExpandedBoards; stub to "nothing expanded".
vi.mock('@/composables/useTreeExpand', () => ({
  useTreeExpand: () => ({ isExpanded: () => false }),
}))

import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { useWorkspacesStore } from '@/stores/workspaces'
import { formatEstimate } from '@/utils/estimation'
import { useBoardViewStore } from '@/stores/boardView'
import { useConflictsStore } from '@/stores/conflicts'
import { useGitlabStore } from '@/stores/gitlab'

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
})
afterEach(() => localStorage.clear())

describe('auth store', () => {
  it('starts unauthenticated with no cached token', () => {
    const s = useAuthStore()
    expect(s.isAuthenticated).toBe(false)
    expect(s.isAdmin).toBe(false)
    expect(s.user).toBeNull()
  })

  it('login persists token/user and hydrates theme prefs', async () => {
    apiMock.auth.login.mockResolvedValue({
      data: {
        access_token: 'tok',
        refresh_token: 'ref',
        user: { id: 'u1', is_admin: true },
        preferences: { accent: 'blue' },
      },
    })
    const s = useAuthStore()
    const theme = useThemeStore()
    await s.login('a@b.c', 'pw')
    expect(apiMock.auth.login).toHaveBeenCalledWith({ email: 'a@b.c', password: 'pw' })
    expect(s.token).toBe('tok')
    expect(s.isAuthenticated).toBe(true)
    expect(s.isAdmin).toBe(true)
    // The access token goes to the api layer's memory, never to localStorage.
    expect(apiMock.setAccessToken).toHaveBeenCalledWith('tok')
    expect(localStorage.getItem('tessera_token')).toBeNull()
    // A refresh token in the body means body mode (desktop) — store it. On web
    // the backend omits it and it ends up in the httpOnly cookie instead.
    expect(localStorage.getItem('tessera_refresh_token')).toBe('ref')
    expect(JSON.parse(localStorage.getItem('tessera_user')).id).toBe('u1')
    expect(theme.activeTheme.key).toBe('blue')
  })

  // #2894: the embedded editor is the one caller that gets a live token from
  // its host instead of logging in, so it calls setToken directly. The method
  // existed but was left out of the store's returned object, so the call threw
  // TypeError inside onMounted — before the document was even fetched — and the
  // Android WebView sat on the loading state forever with no error to show.
  it('exposes setToken so a host-supplied session can be installed', () => {
    const s = useAuthStore()
    expect(typeof s.setToken).toBe('function')
    s.setToken('host-tok')
    expect(s.token).toBe('host-tok')
    expect(s.isAuthenticated).toBe(true)
    // The axios layer must learn about it too, or every request goes out
    // unauthenticated while the store insists there is a session.
    expect(apiMock.setAccessToken).toHaveBeenCalledWith('host-tok')
  })

  it('setAuth without a refresh token clears the stored one', () => {
    localStorage.setItem('tessera_refresh_token', 'stale')
    const s = useAuthStore()
    s.setAuth({ access_token: 'tok', user: { id: 'x' } })
    expect(localStorage.getItem('tessera_refresh_token')).toBeNull()
  })

  it('register goes through setAuth', async () => {
    apiMock.auth.register.mockResolvedValue({
      data: { access_token: 't', user: { id: 'r1' } },
    })
    const s = useAuthStore()
    await s.register('a@b.c', 'Name', 'pw')
    expect(apiMock.auth.register).toHaveBeenCalledWith({
      email: 'a@b.c',
      name: 'Name',
      password: 'pw',
    })
    expect(s.user.id).toBe('r1')
  })

  it('logout revokes server-side and clears local state synchronously', () => {
    const s = useAuthStore()
    s.setAuth({ access_token: 't', refresh_token: 'r', user: { id: 'x' } })
    s.logout()
    // Without the server call the refresh token would stay valid for 30 days.
    expect(apiMock.auth.logout).toHaveBeenCalled()
    // Cleared before the first await: the sidebar navigates without awaiting,
    // and a still-authenticated store would bounce it back off /login.
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
    expect(s.isAuthenticated).toBe(false)
    expect(localStorage.getItem('tessera_refresh_token')).toBeNull()
    expect(localStorage.getItem('tessera_user')).toBeNull()
  })

  it('logout still clears the session when the server call fails', async () => {
    apiMock.auth.logout.mockRejectedValueOnce(new Error('offline'))
    const s = useAuthStore()
    s.setAuth({ access_token: 't', user: { id: 'x' } })
    await s.logout()
    expect(s.isAuthenticated).toBe(false)
  })

  it('setUser refreshes the cached user', () => {
    const s = useAuthStore()
    s.setUser({ id: 'u9', name: 'Z' })
    expect(s.user.name).toBe('Z')
    expect(JSON.parse(localStorage.getItem('tessera_user')).id).toBe('u9')
  })

  it('loginWithTokens persists the pair then loads the profile via /auth/me', async () => {
    apiMock.auth.me.mockResolvedValue({
      data: { user: { id: 'oa' }, preferences: { accent: 'red' } },
    })
    const s = useAuthStore()
    const theme = useThemeStore()
    await s.loginWithTokens('acc', 'rfr')
    expect(s.token).toBe('acc')
    expect(localStorage.getItem('tessera_token')).toBeNull()
    expect(localStorage.getItem('tessera_refresh_token')).toBe('rfr')
    expect(apiMock.auth.me).toHaveBeenCalled()
    expect(s.user.id).toBe('oa')
    expect(theme.activeTheme.key).toBe('red')
  })

  it('verify is a no-op without a token', async () => {
    const s = useAuthStore()
    await s.verify()
    expect(apiMock.auth.me).not.toHaveBeenCalled()
  })

  it('verify refreshes the cached user on success', async () => {
    apiMock.auth.me.mockResolvedValue({ data: { user: { id: 'v1' } } })
    const s = useAuthStore()
    s.setAuth({ access_token: 'tok' })
    await s.verify()
    expect(s.user.id).toBe('v1')
  })

  it('verify logs out on a rejected /auth/me', async () => {
    apiMock.auth.me.mockRejectedValue(new Error('401'))
    const s = useAuthStore()
    s.setAuth({ access_token: 'tok' })
    await s.verify()
    expect(s.token).toBe('')
    expect(s.user).toBeNull()
  })

  // #2750: /auth/me failing because the network is down says nothing about
  // whether the session is still valid — signing the user out on it is exactly
  // the "brief blip → login screen" the bug report describes.
  it('verify keeps the session when /auth/me fails offline', async () => {
    apiMock.auth.me.mockRejectedValue(Object.assign(new Error('offline'), { offline: true }))
    const s = useAuthStore()
    s.setAuth({ access_token: 'tok', user: { id: 'v1' } })
    await s.verify()
    expect(s.token).toBe('tok')
    expect(s.user).not.toBeNull()
  })

  // #2684: with the access token held in memory, a reload starts signed out —
  // bootstrap() is what turns the refresh cookie back into a live session, and
  // main.js awaits it before the router (and its guard) exist.
  describe('bootstrap', () => {
    it('restores the session and reloads the profile', async () => {
      localStorage.setItem('tessera_user', '{"id":"u1"}')
      apiMock.restoreSession.mockResolvedValueOnce({ token: 'fresh', offline: false })
      apiMock.auth.me.mockResolvedValue({ data: { user: { id: 'u1', is_admin: true } } })
      const s = useAuthStore()
      await s.bootstrap()
      expect(s.token).toBe('fresh')
      expect(s.isAuthenticated).toBe(true)
      expect(s.isAdmin).toBe(true)
    })

    it('does nothing for a visitor who was never signed in here', async () => {
      const s = useAuthStore()
      await s.bootstrap()
      // No cached user → no pointless refresh call (and no 401) on the login page.
      expect(apiMock.restoreSession).not.toHaveBeenCalled()
      expect(s.isAuthenticated).toBe(false)
    })

    it('clears the stale cache when the refresh cookie is gone', async () => {
      localStorage.setItem('tessera_user', '{"id":"u1"}')
      apiMock.restoreSession.mockResolvedValueOnce({ token: null, offline: false })
      const s = useAuthStore()
      await s.bootstrap()
      expect(s.isAuthenticated).toBe(false)
      expect(localStorage.getItem('tessera_user')).toBeNull()
    })

    // #2750: clearSession() drops `tessera_user`, and that key is the flag
    // bootstrap() checks to decide whether to restore at all — so clearing it on
    // a network error made ONE offline reload forget a live session forever.
    it('keeps the cached user when the server is unreachable, and retries', async () => {
      localStorage.setItem('tessera_user', '{"id":"u1"}')
      apiMock.restoreSession
        .mockResolvedValueOnce({ token: null, offline: true })
        .mockResolvedValueOnce({ token: 'fresh', offline: false })
      apiMock.auth.me.mockResolvedValue({ data: { user: { id: 'u1' } } })
      const s = useAuthStore()
      // Fake timers: the retry backoff is seconds of real time otherwise.
      vi.useFakeTimers()
      const done = s.bootstrap()
      await vi.runAllTimersAsync()
      await done
      vi.useRealTimers()
      expect(apiMock.restoreSession).toHaveBeenCalledTimes(2)
      expect(s.token).toBe('fresh')
      expect(localStorage.getItem('tessera_user')).not.toBeNull()
    })

    it('gives up after the retries but leaves the session recoverable', async () => {
      localStorage.setItem('tessera_user', '{"id":"u1"}')
      apiMock.restoreSession.mockResolvedValue({ token: null, offline: true })
      const s = useAuthStore()
      vi.useFakeTimers()
      const done = s.bootstrap()
      await vi.runAllTimersAsync()
      await done
      vi.useRealTimers()
      expect(s.isAuthenticated).toBe(false)
      // The cached user survives, so the next reload tries the cookie again.
      expect(localStorage.getItem('tessera_user')).toBe('{"id":"u1"}')
    })
  })
})

describe('workspaces store', () => {
  it('loadWorkspaces auto-selects the first workspace and persists tessera_ws', async () => {
    apiMock.workspaces.list.mockResolvedValue({ data: [{ id: 'w1' }, { id: 'w2' }] })
    const s = useWorkspacesStore()
    await s.loadWorkspaces()
    expect(s.currentId).toBe('w1')
    expect(s.current.id).toBe('w1')
    expect(localStorage.getItem('tessera_ws')).toBe('w1')
    expect(apiMock.workspaces.groups).toHaveBeenCalledWith('w1')
  })

  it('loadWorkspaces keeps a valid cached selection', async () => {
    localStorage.setItem('tessera_ws', 'w2')
    setActivePinia(createPinia())
    apiMock.workspaces.list.mockResolvedValue({ data: [{ id: 'w1' }, { id: 'w2' }] })
    const s = useWorkspacesStore()
    await s.loadWorkspaces()
    expect(s.currentId).toBe('w2')
  })

  it('selectWorkspace loads groups + projects and clears boards cache', async () => {
    apiMock.workspaces.groups.mockResolvedValue({ data: [{ id: 'g1', position: 0 }] })
    apiMock.workspaces.projects.mockResolvedValue({
      data: [{ id: 'p1', group_id: 'g1', position: 0 }],
    })
    const s = useWorkspacesStore()
    await s.selectWorkspace('w9')
    expect(s.currentId).toBe('w9')
    expect(s.groups).toHaveLength(1)
    expect(s.projects).toHaveLength(1)
    expect(localStorage.getItem('tessera_ws')).toBe('w9')
  })

  it('selectWorkspace loads the command registry and flattens it for the popup', async () => {
    apiMock.workspaces.commands.mockResolvedValue({
      data: {
        builtin: [{ key: 'close', arg: 'none', description: 'Закрыть задачу' }],
        custom: [{ key: 'approve', description: 'Одобрить план' }],
        can_manage: true,
      },
    })
    const s = useWorkspacesStore()
    await s.selectWorkspace('w9')
    expect(s.commands.map((c) => c.key)).toEqual(['close', 'approve'])
    expect(s.commands[1].builtin).toBe(false)
    expect(s.commandsCanManage).toBe(true)
  })

  it('a failing command registry leaves the popup empty, not the board broken', async () => {
    apiMock.workspaces.commands.mockRejectedValue(new Error('boom'))
    const s = useWorkspacesStore()
    await s.selectWorkspace('w9')
    expect(s.commands).toEqual([])
    expect(s.commandsCanManage).toBe(false)
    expect(s.currentId).toBe('w9')
  })

  it('setCustomCommands patches the dictionary and keeps the built-ins', async () => {
    apiMock.workspaces.commands.mockResolvedValue({
      data: { builtin: [{ key: 'close', arg: 'none', description: '' }], custom: [] },
    })
    const s = useWorkspacesStore()
    await s.selectWorkspace('w9')
    s.setCustomCommands([{ key: 'hold', description: 'Отложить' }])
    expect(s.commands.map((c) => c.key)).toEqual(['close', 'hold'])
  })

  // #2932: board.created/updated/deleted are workspace events, but the sidebar
  // tree used to hear none of them — refresh() deliberately leaves expanded
  // boards alone, and the open board ignores board CRUD. So only the client that
  // pressed the button ever re-read boardsByProject: a colleague's new board
  // stayed invisible, their rename stayed stale, and a board they deleted kept
  // sitting in everybody's sidebar, handing out «Не найдено» to whoever clicked.
  describe('onBoardEvent keeps the sidebar tree in sync with other clients', () => {
    const loaded = () => {
      const s = useWorkspacesStore()
      s.boardsByProject = {
        p1: [
          { id: 'b1', project_id: 'p1', name: 'Спринт', position: 1 },
          { id: 'b3', project_id: 'p1', name: 'Хотелки', position: 3 },
        ],
        p2: [{ id: 'b9', project_id: 'p2', name: 'Чужая', position: 1 }],
      }
      return s
    }

    it('вставляет созданную доску на её место по position', () => {
      const s = loaded()
      s.onBoardEvent({
        type: 'board.created',
        data: { id: 'b2', project_id: 'p1', name: 'Багфиксы', position: 2 },
      })
      expect(s.boardsByProject.p1.map((b) => b.id)).toEqual(['b1', 'b2', 'b3'])
    })

    it('не дублирует доску, которую создавший уже подгрузил сам', () => {
      const s = loaded()
      s.onBoardEvent({ type: 'board.created', data: { id: 'b1', project_id: 'p1', position: 1 } })
      expect(s.boardsByProject.p1).toHaveLength(2)
    })

    it('не трогает проект, чьи доски ещё не раскрыты — он подтянет правду сам', () => {
      const s = loaded()
      s.onBoardEvent({ type: 'board.created', data: { id: 'b7', project_id: 'p7', position: 1 } })
      expect(s.boardsByProject.p7).toBeUndefined()
    })

    it('подхватывает чужое переименование', () => {
      const s = loaded()
      s.onBoardEvent({ type: 'board.updated', data: { id: 'b1', project_id: 'p1', name: 'Релиз' } })
      expect(s.boardsByProject.p1[0].name).toBe('Релиз')
      // Переименование не переставляет доски.
      expect(s.boardsByProject.p1.map((b) => b.id)).toEqual(['b1', 'b3'])
    })

    // board.deleted несёт голый {id} — без project_id, так что доску приходится
    // искать по всем загруженным спискам.
    it('убирает удалённую доску, хотя событие не называет проект', () => {
      const s = loaded()
      s.onBoardEvent({ type: 'board.deleted', data: { id: 'b3' } })
      expect(s.boardsByProject.p1.map((b) => b.id)).toEqual(['b1'])
      expect(s.boardsByProject.p2).toHaveLength(1)
    })

    it('событие о незагруженной доске ничего не ломает', () => {
      const s = loaded()
      const before = s.boardsByProject
      s.onBoardEvent({ type: 'board.deleted', data: { id: 'b404' } })
      s.onBoardEvent({ type: 'board.deleted', data: {} })
      s.onBoardEvent({ type: 'task.deleted', data: { id: 'b1' } })
      expect(s.boardsByProject).toBe(before)
    })
  })

  // #2932: узлы спринтов рисует ProjectRow, а сокет открыт один на приложение —
  // поэтому событие доезжает до строки проекта ревизией, которую она watch'ит.
  describe('onMilestoneEvent помечает список спринтов протухшим', () => {
    const ws = 'w1'
    const ready = () => {
      const s = useWorkspacesStore()
      s.currentId = ws
      return s
    }

    it('двигает ревизию только того проекта, который назвало событие', () => {
      const s = ready()
      const p1 = s.milestoneRevFor('p1')
      const p2 = s.milestoneRevFor('p2')
      s.onMilestoneEvent({
        type: 'milestone.created',
        scope: ws,
        data: { id: 'm1', project_id: 'p1' },
      })
      expect(s.milestoneRevFor('p1')).toBe(p1 + 1)
      expect(s.milestoneRevFor('p2')).toBe(p2)
    })

    // milestone.deleted — голый {id}: проект неизвестен, значит протухли все.
    it('двигает ревизию всем, когда событие не называет проект', () => {
      const s = ready()
      const p1 = s.milestoneRevFor('p1')
      const p2 = s.milestoneRevFor('p2')
      s.onMilestoneEvent({ type: 'milestone.deleted', scope: ws, data: { id: 'm1' } })
      expect(s.milestoneRevFor('p1')).toBe(p1 + 1)
      expect(s.milestoneRevFor('p2')).toBe(p2 + 1)
    })

    it('молчит на чужом пространстве и на не-спринтовых событиях', () => {
      const s = ready()
      const p1 = s.milestoneRevFor('p1')
      s.onMilestoneEvent({ type: 'milestone.created', scope: 'w2', data: { project_id: 'p1' } })
      s.onMilestoneEvent({ type: 'task.updated', scope: ws, data: { id: 't1' } })
      s.onMilestoneEvent({ type: 'board.deleted', scope: ws, data: { id: 'b1' } })
      expect(s.milestoneRevFor('p1')).toBe(p1)
    })
  })

  // #2932: a ring in the group edges breaks the sidebar in two ways, and the
  // tree is built from parent_id in both. Drawing starts at the root, so a ring
  // is unreachable from it: the groups in it — and every project inside them —
  // simply stop appearing, with no way left to drag them out. And the
  // active-rail lookup walks *up* from the open project's group, which in a ring
  // never reaches a root: that loop sits in a computed, so it takes the tab with
  // it. The server refuses to make such a cycle now; data from before that guard
  // still has to render, so the ring is flattened to roots — nothing hidden.
  it('a cycle in group parents stays visible instead of dropping out of the tree', () => {
    const s = useWorkspacesStore()
    // a ↔ b closed on each other, c hangs under a, d is an untouched root
    s.groups = [
      { id: 'a', parent_id: 'b', position: 1 },
      { id: 'b', parent_id: 'a', position: 2 },
      { id: 'c', parent_id: 'a', position: 3 },
      { id: 'd', parent_id: null, position: 4 },
    ]

    // Walk the whole tree the way the components do, from the root down.
    const visited = []
    const walk = (parentId, depth) => {
      if (depth > 12) throw new Error('group tree does not terminate')
      for (const g of s.childGroups(parentId)) {
        visited.push(g.id)
        walk(g.id, depth + 1)
      }
    }
    walk(null, 0)

    // Every group is reachable — without the guard a, b and c are orphaned off
    // the root and only d is drawn.
    expect(visited.sort()).toEqual(['a', 'b', 'c', 'd'])
    expect(s.childGroups(null).map((g) => g.id)).toEqual(['a', 'b', 'd'])
    expect(s.childGroups('a').map((g) => g.id)).toEqual(['c']) // b broke its own edge
    expect(s.parentGroupOf('a')).toBeNull()
    expect(s.parentGroupOf('c').id).toBe('a')
  })

  it('walking up from a group in a cycle reaches a root instead of looping', () => {
    const s = useWorkspacesStore()
    s.groups = [
      { id: 'a', parent_id: 'b', position: 1 },
      { id: 'b', parent_id: 'a', position: 2 },
      { id: 'c', parent_id: 'a', position: 3 },
    ]
    // Sidebar's active-rail lookup, verbatim: up from the open project's group.
    const rootOf = (startId) => {
      let g = s.groups.find((x) => x.id === startId)
      let parent = g ? s.parentGroupOf(g.id) : null
      for (let steps = 0; parent; steps += 1) {
        if (steps > 24) throw new Error('ancestor walk does not terminate')
        g = parent
        parent = s.parentGroupOf(g.id)
      }
      return g?.id ?? null
    }
    expect(rootOf('c')).toBe('a') // a is a root now, so the walk stops there
    expect(rootOf('a')).toBe('a')
    expect(rootOf('b')).toBe('b')
  })

  it('an ordinary group tree is untouched by the cycle guard', () => {
    const s = useWorkspacesStore()
    s.groups = [
      { id: 'top', parent_id: null, position: 1 },
      { id: 'mid', parent_id: 'top', position: 2 },
      { id: 'deep', parent_id: 'mid', position: 3 },
    ]
    expect(s.childGroups(null).map((g) => g.id)).toEqual(['top'])
    expect(s.childGroups('top').map((g) => g.id)).toEqual(['mid'])
    expect(s.childGroups('mid').map((g) => g.id)).toEqual(['deep'])
    expect(s.parentGroupOf('deep').id).toBe('mid')
    expect(s.parentGroupOf('top')).toBeNull()
  })

  // #2932: the estimation config decides what a task's `estimate` *number*
  // means, and the backend broadcasts every change to it. Until the event is
  // applied, a colleague who left the board open keeps both reading and writing
  // estimates against the config the project no longer uses.
  it('a project estimation override from the socket reaches an open client', () => {
    const s = useWorkspacesStore()
    s.list = [{ id: 'w1', estimation: null }]
    s.currentId = 'w1'
    s.projects = [{ id: 'p1', estimation: { unit: 'time', hours_per_day: 8, days_per_week: 5 } }]
    // 480 minutes is one working day while the day is eight hours long.
    expect(formatEstimate(480, s.estimationFor('p1'))).toBe('1д')

    s.onEstimationEvent({
      type: 'project.estimation',
      scope: 'w1',
      data: { project_id: 'p1', estimation: { unit: 'time', hours_per_day: 6, days_per_week: 5 } },
    })

    // The team went to six-hour days: the same stored number is now a day and two hours.
    expect(s.estimationFor('p1').hours_per_day).toBe(6)
    expect(formatEstimate(480, s.estimationFor('p1'))).toBe('1д 2ч')
  })

  it('a workspace estimation default from the socket reaches an open client', () => {
    const s = useWorkspacesStore()
    s.list = [{ id: 'w1', estimation: { unit: 'time', hours_per_day: 8, days_per_week: 5 } }]
    s.currentId = 'w1'
    s.projects = [{ id: 'p1', estimation: null }] // inherits the workspace default

    s.onEstimationEvent({
      type: 'workspace.estimation',
      scope: 'w1',
      data: { workspace_id: 'w1', estimation: { unit: 'points', points_scale: 'fibonacci' } },
    })

    expect(s.estimationFor('p1').unit).toBe('points')
    expect(formatEstimate(5, s.estimationFor('p1'))).toBe('5 SP')
  })

  it('clearing a project override on the socket falls back to the workspace default', () => {
    const s = useWorkspacesStore()
    s.list = [{ id: 'w1', estimation: { unit: 'custom', custom_label: 'сеанс' } }]
    s.currentId = 'w1'
    s.projects = [{ id: 'p1', estimation: { unit: 'points', points_scale: 'linear' } }]

    s.onEstimationEvent({
      type: 'project.estimation',
      scope: 'w1',
      data: { project_id: 'p1', estimation: null },
    })

    expect(s.estimationFor('p1').unit).toBe('custom')
  })

  it('an unrelated event leaves the estimation config alone', () => {
    const s = useWorkspacesStore()
    s.list = [{ id: 'w1', estimation: null }]
    s.currentId = 'w1'
    s.projects = [{ id: 'p1', estimation: { unit: 'points', points_scale: 'linear' } }]
    s.onEstimationEvent({ type: 'project.updated', scope: 'w1', data: { id: 'p1', slug: 'new' } })
    expect(s.estimationFor('p1').unit).toBe('points')
  })

  // #2721: a deep-linked board has to drag the app into its own workspace, or
  // the realtime scope filter drops all of its events.
  it('ensureWorkspace switches to a foreign workspace and persists it', async () => {
    const s = useWorkspacesStore()
    s.currentId = 'w1'
    await s.ensureWorkspace('w2')
    expect(s.currentId).toBe('w2')
    expect(localStorage.getItem('tessera_ws')).toBe('w2')
  })

  it('ensureWorkspace on the current workspace does not refetch the tree', async () => {
    const s = useWorkspacesStore()
    s.currentId = 'w1'
    await s.ensureWorkspace('w1')
    expect(apiMock.workspaces.groups).not.toHaveBeenCalled()
  })

  it('ensureWorkspace ignores an empty id', async () => {
    const s = useWorkspacesStore()
    s.currentId = 'w1'
    await s.ensureWorkspace(undefined)
    expect(s.currentId).toBe('w1')
    expect(apiMock.workspaces.groups).not.toHaveBeenCalled()
  })

  // The startup race: AppLayout selects the remembered workspace while a deep
  // link selects the board's own. Responses can land in either order, so the
  // later switch has to win the tree regardless.
  it('a superseded selectWorkspace does not overwrite the later one’s tree', async () => {
    const s = useWorkspacesStore()
    let releaseStale
    const stale = new Promise((resolve) => {
      releaseStale = () => resolve({ data: [{ id: 'p-stale', position: 0 }] })
    })
    apiMock.workspaces.projects.mockReturnValueOnce(stale)
    apiMock.workspaces.projects.mockResolvedValueOnce({ data: [{ id: 'p-fresh', position: 0 }] })

    const first = s.selectWorkspace('w-stale')
    await s.selectWorkspace('w-fresh')
    releaseStale()
    await first

    expect(s.currentId).toBe('w-fresh')
    expect(s.projects.map((p) => p.id)).toEqual(['p-fresh'])
  })

  it('removeWorkspace deletes then reloads and re-picks', async () => {
    apiMock.workspaces.list.mockResolvedValue({ data: [{ id: 'w2' }] })
    const s = useWorkspacesStore()
    s.currentId = 'w1'
    await s.removeWorkspace('w1')
    expect(apiMock.workspaces.remove).toHaveBeenCalledWith('w1')
    expect(s.currentId).toBe('w2')
  })

  it('loadBoards caches boards per project immutably', async () => {
    apiMock.projects.boards.mockResolvedValue({ data: [{ id: 'b1', project_id: 'p1' }] })
    const s = useWorkspacesStore()
    await s.loadBoards('p1')
    expect(s.boardsByProject.p1).toHaveLength(1)
  })

  it('upsertBoard patches a loaded board in place', async () => {
    apiMock.projects.boards.mockResolvedValue({
      data: [{ id: 'b1', project_id: 'p1', name: 'Old' }],
    })
    const s = useWorkspacesStore()
    await s.loadBoards('p1')
    s.upsertBoard({ id: 'b1', project_id: 'p1', name: 'New' })
    expect(s.boardsByProject.p1[0].name).toBe('New')
    // Unknown project → no-op, no throw.
    s.upsertBoard({ id: 'x', project_id: 'nope' })
  })

  it('childGroups / projectsInGroup filter and sort by position', () => {
    const s = useWorkspacesStore()
    s.groups = [
      { id: 'g2', parent_id: null, position: 2 },
      { id: 'g1', parent_id: null, position: 1 },
      { id: 'g3', parent_id: 'g1', position: 0 },
    ]
    s.projects = [
      { id: 'p2', group_id: 'g1', position: 2 },
      { id: 'p1', group_id: 'g1', position: 1 },
      { id: 'p3', group_id: null, position: 0 },
    ]
    expect(s.childGroups(null).map((g) => g.id)).toEqual(['g1', 'g2'])
    expect(s.childGroups('g1').map((g) => g.id)).toEqual(['g3'])
    expect(s.projectsInGroup('g1').map((p) => p.id)).toEqual(['p1', 'p2'])
    expect(s.projectsInGroup(null).map((p) => p.id)).toEqual(['p3'])
  })

  it('setProjectEstimation / setWorkspaceEstimation patch in place', () => {
    const s = useWorkspacesStore()
    s.projects = [{ id: 'p1' }, { id: 'p2' }]
    s.list = [{ id: 'w1' }]
    s.setProjectEstimation('p1', { unit: 'h' })
    expect(s.projects.find((p) => p.id === 'p1').estimation).toEqual({ unit: 'h' })
    s.setWorkspaceEstimation('w1', { unit: 'd' })
    expect(s.list[0].estimation).toEqual({ unit: 'd' })
  })
})

describe('boardView store', () => {
  it('setContext marks a board active', () => {
    const s = useBoardViewStore()
    expect(s.active).toBe(false)
    s.setContext('b1', 'w1', 'p1')
    expect(s.active).toBe(true)
    expect(s.boardId).toBe('b1')
    expect(s.wsId).toBe('w1')
    expect(s.projectId).toBe('p1')
  })

  it('bumpReload increments the nonce', () => {
    const s = useBoardViewStore()
    const before = s.reloadNonce
    s.bumpReload()
    expect(s.reloadNonce).toBe(before + 1)
  })

  it('refill normalizes a null map to empty and derives the lists from it', () => {
    const s = useBoardViewStore()
    s.refill(s.tagsMap, null)
    s.refill(s.prefixNames, null)
    s.refill(s.milestonesMap, null)
    expect(s.tagsList).toEqual([])
    expect(s.prefixNames).toEqual({})
    expect(s.milestonesList).toEqual([])
    s.refill(s.tagsMap, { t: { id: 't' } })
    expect(s.tagsList).toHaveLength(1)
  })

  it('refill keeps the map identity so holders of the reference see the update', () => {
    const s = useBoardViewStore()
    const held = s.tagsMap // what a card binds to
    s.refill(s.tagsMap, { t: { id: 't' } })
    expect(held).toBe(s.tagsMap)
    expect(held.t).toEqual({ id: 't' })
    s.refill(s.tagsMap, { u: { id: 'u' } })
    expect(held.t).toBeUndefined()
    expect(held.u).toEqual({ id: 'u' })
  })

  it('gitlabMembersList hides GitLab members already mapped to a workspace member', () => {
    const s = useBoardViewStore()
    s.refill(s.membersMap, { u1: { user_id: 'u1', name: 'Аня' } })
    s.refill(s.gitlabMembersMap, {
      1: { gl_user_id: 1, gl_username: 'anya', tessera_user_id: 'u1' },
      2: { gl_user_id: 2, gl_username: 'extern', tessera_user_id: null },
    })
    expect(s.gitlabMembersList.map((g) => g.gl_username)).toEqual(['extern'])
  })

  it('reset clears context and card display, but leaves layout untouched', () => {
    const s = useBoardViewStore()
    s.setContext('b1', 'w1', 'p1')
    s.refill(s.tagsMap, { t: { id: 't' } })
    s.board = { id: 'b1' }
    s.columns = [{ id: 'c1' }]
    s.metaTagPrefixes = new Set(['status'])
    s.gitlabCanCreate = true
    s.cardSize = 'large'
    s.fieldVis.due = false
    s.layout = 'gantt'
    s.archiveOpen = true
    s.reset()
    expect(s.active).toBe(false)
    expect(s.boardId).toBeNull()
    expect(s.board).toBeNull()
    expect(s.columns).toEqual([])
    expect(s.tagsList).toEqual([])
    expect(s.metaTagPrefixes.size).toBe(0)
    expect(s.gitlabCanCreate).toBe(false)
    // Card display settings are per-board too — a stale 'large' would leak into
    // the next board opened before its saved view is restored.
    expect(s.cardSize).toBe('medium')
    expect(s.fieldVis.due).toBe(true)
    expect(s.archiveOpen).toBe(false)
    // layout is intentionally not reset.
    expect(s.layout).toBe('gantt')
  })
})

describe('conflicts store', () => {
  it('load populates the list and derives count / taskIds / has', async () => {
    apiMock.gitlab.conflicts.mockResolvedValue({
      data: [
        { id: 'c1', task_id: 't1' },
        { id: 'c2', task_id: 't2' },
      ],
    })
    const s = useConflictsStore()
    await s.load('w1')
    expect(s.count).toBe(2)
    expect(s.has('t1')).toBe(true)
    expect(s.has('nope')).toBe(false)
    expect([...s.taskIds]).toEqual(['t1', 't2'])
  })

  it('load with no workspace id clears the list', async () => {
    const s = useConflictsStore()
    await s.load()
    expect(s.count).toBe(0)
    expect(apiMock.gitlab.conflicts).not.toHaveBeenCalled()
  })

  it('load swallows API errors as no-conflicts', async () => {
    apiMock.gitlab.conflicts.mockRejectedValue(new Error('offline'))
    const s = useConflictsStore()
    await s.load('w1')
    expect(s.count).toBe(0)
  })

  it('openResolver opens the modal focused on a task', () => {
    const s = useConflictsStore()
    s.openResolver('t7')
    expect(s.resolverOpen).toBe(true)
    expect(s.focusTaskId).toBe('t7')
  })

  it('onEvent reloads on a matching gitlab.conflict event', async () => {
    apiMock.gitlab.conflicts.mockResolvedValue({ data: [{ id: 'c', task_id: 't' }] })
    const s = useConflictsStore()
    await s.load('w1')
    apiMock.gitlab.conflicts.mockClear()
    s.onEvent({ type: 'gitlab.conflict', scope: 'w1' })
    expect(apiMock.gitlab.conflicts).toHaveBeenCalled()
    // Wrong scope → ignored.
    apiMock.gitlab.conflicts.mockClear()
    s.onEvent({ type: 'gitlab.conflict', scope: 'other' })
    expect(apiMock.gitlab.conflicts).not.toHaveBeenCalled()
    // Wrong type → ignored.
    s.onEvent({ type: 'task.created' })
    expect(apiMock.gitlab.conflicts).not.toHaveBeenCalled()
  })

  it('clear resets list and workspace', async () => {
    apiMock.gitlab.conflicts.mockResolvedValue({ data: [{ id: 'c', task_id: 't' }] })
    const s = useConflictsStore()
    await s.load('w1')
    s.clear()
    expect(s.count).toBe(0)
    // wsId is store-internal (not exposed on the returned surface).
    expect(s.wsId).toBeUndefined()
  })
})

describe('gitlab store', () => {
  it('load reflects a connected account', async () => {
    apiMock.gitlab.getConnection.mockResolvedValue({
      data: { connected: true, base_url: 'https://gl', gl_username: 'me' },
    })
    const s = useGitlabStore()
    await s.load()
    expect(s.connected).toBe(true)
    expect(s.baseUrl).toBe('https://gl')
    expect(s.username).toBe('me')
    expect(s.loaded).toBe(true)
  })

  it('load marks not-connected and still loaded on error', async () => {
    apiMock.gitlab.getConnection.mockRejectedValue(new Error('x'))
    const s = useGitlabStore()
    await s.load()
    expect(s.connected).toBe(false)
    expect(s.loaded).toBe(true)
  })

  it('connect stores the returned connection', async () => {
    apiMock.gitlab.connect.mockResolvedValue({
      data: { connected: true, base_url: 'https://g2', gl_username: 'u' },
    })
    const s = useGitlabStore()
    await s.connect('https://g2', 'tok')
    expect(apiMock.gitlab.connect).toHaveBeenCalledWith({
      base_url: 'https://g2',
      token: 'tok',
    })
    expect(s.connected).toBe(true)
    expect(s.username).toBe('u')
  })

  it('disconnect clears connection state', async () => {
    apiMock.gitlab.connect.mockResolvedValue({
      data: { connected: true, gl_username: 'u' },
    })
    const s = useGitlabStore()
    await s.connect('b', 't')
    await s.disconnect()
    expect(apiMock.gitlab.disconnect).toHaveBeenCalled()
    expect(s.connected).toBe(false)
    expect(s.username).toBe('')
  })
})
