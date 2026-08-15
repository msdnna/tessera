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

  // #2684: with the access token held in memory, a reload starts signed out —
  // bootstrap() is what turns the refresh cookie back into a live session, and
  // main.js awaits it before the router (and its guard) exist.
  describe('bootstrap', () => {
    it('restores the session and reloads the profile', async () => {
      localStorage.setItem('tessera_user', '{"id":"u1"}')
      apiMock.restoreSession.mockResolvedValueOnce('fresh')
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
      apiMock.restoreSession.mockResolvedValueOnce(null)
      const s = useAuthStore()
      await s.bootstrap()
      expect(s.isAuthenticated).toBe(false)
      expect(localStorage.getItem('tessera_user')).toBeNull()
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
