// Дерево воркспейсов и GitLab-привязка переживали выход из аккаунта (#2932).
//
// clearSession() гасил тему и ленту уведомлений, но два оставшихся персональных
// кэша — нет. В сторе воркспейсов лежит список с `my_role` (а на нём держится
// весь canManage), группы, проекты, доски и словарь быстрых команд; в
// gitlab-сторе — «подключён как @кто». Выход из аккаунта в SPA это смена
// маршрута, а не перезагрузка (перезагружается приложение только при смене
// сервера, stores/servers.js), так что следующий вход в той же вкладке
// начинался с чужого сайдбара: имена чужих проектов и досок, кликабельные, и
// 403 на каждый клик.
//
// И это не моргание. AppLayout в onMounted ждёт loadWorkspaces() раньше всего
// прочего, так что одного упавшего запроса списка хватает, чтобы тот сайдбар
// остался на всю сессию — вместе с правами, посчитанными по чужой роли.
// У gitlab-стора роль такой же защёлки играет `loaded`: GitLabModal читает
// стор, только если он ещё не загружен.
//
// Вторая половина починки — бамп обеих последовательностей: запрос списка или
// переключение воркспейса, поднятые уходящей сессией, могут быть ещё в полёте,
// и их ответ ничем не отличается от свежего.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'

const wsList = vi.fn(() => Promise.resolve({ data: [] }))
const wsGroups = vi.fn(() => Promise.resolve({ data: [] }))
const wsProjects = vi.fn(() => Promise.resolve({ data: [] }))
const wsCommands = vi.fn(() => Promise.resolve({ data: {} }))
const projBoards = vi.fn(() => Promise.resolve({ data: [] }))
const glConnection = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('@/api', () => ({
  auth: {
    logout: vi.fn(() => Promise.resolve()),
    me: vi.fn(() => Promise.resolve({ data: {} })),
  },
  notifications: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    markRead: vi.fn(() => Promise.resolve()),
    markAllRead: vi.fn(() => Promise.resolve()),
  },
  workspaces: {
    list: (...a) => wsList(...a),
    groups: (...a) => wsGroups(...a),
    projects: (...a) => wsProjects(...a),
    commands: (...a) => wsCommands(...a),
    remove: vi.fn(() => Promise.resolve()),
  },
  projects: { boards: (...a) => projBoards(...a) },
  gitlab: {
    getConnection: (...a) => glConnection(...a),
    connect: vi.fn(() => Promise.resolve({ data: {} })),
    disconnect: vi.fn(() => Promise.resolve()),
  },
  setAccessToken: vi.fn(),
  restoreSession: vi.fn(() => Promise.resolve(null)),
}))

import { useWorkspacesStore } from '@/stores/workspaces'
import { useGitlabStore } from '@/stores/gitlab'
import { useAuthStore } from '@/stores/auth'

function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

// Сайдбар прошлого читателя: воркспейс, где он владелец, с деревом и досками.
function seedTree(ws) {
  ws.list = [{ id: 'w1', name: 'Чужой воркспейс', my_role: 'owner' }]
  ws.currentId = 'w1'
  ws.groups = [{ id: 'g1', name: 'Секретная группа', parent_id: null, position: 0 }]
  ws.projects = [{ id: 'p1', name: 'Секретный проект', group_id: 'g1', position: 0 }]
  ws.boardsByProject = { p1: [{ id: 'b1', project_id: 'p1', name: 'Секретная доска' }] }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  vi.clearAllMocks()
  wsList.mockImplementation(() => Promise.resolve({ data: [] }))
  wsGroups.mockImplementation(() => Promise.resolve({ data: [] }))
  wsProjects.mockImplementation(() => Promise.resolve({ data: [] }))
  wsCommands.mockImplementation(() => Promise.resolve({ data: {} }))
  glConnection.mockImplementation(() => Promise.resolve({ data: {} }))
})

describe('выход из аккаунта уносит дерево воркспейсов', () => {
  it('чистит список, дерево, доски и словарь команд', () => {
    const ws = useWorkspacesStore()
    seedTree(ws)
    ws.customCommands = [{ name: 'deploy', body: 'секрет' }]
    ws.commandsCanManage = true
    expect(ws.canManage).toBe(true)

    useAuthStore().clearSession()

    expect(ws.list).toEqual([])
    expect(ws.groups).toEqual([])
    expect(ws.projects).toEqual([])
    expect(ws.boardsByProject).toEqual({})
    expect(ws.customCommands).toEqual([])
    expect(ws.commandsCanManage).toBe(false)
    // Роль уходит вместе со списком — иначе следующему читателю показывают
    // управляющие контролы по чужому my_role.
    expect(ws.current).toBeNull()
    expect(ws.canManage).toBe(false)
  })

  it('упавшая перезагрузка списка не возвращает чужой сайдбар на экран', async () => {
    const ws = useWorkspacesStore()
    seedTree(ws)
    useAuthStore().clearSession()

    wsList.mockImplementation(() => Promise.reject(new Error('offline')))
    await expect(ws.loadWorkspaces()).rejects.toThrow('offline')

    expect(ws.projects).toEqual([])
    expect(ws.groups).toEqual([])
    expect(ws.list).toEqual([])
  })

  it('ответ списка, поднятый уходящей сессией, не доливает список назад', async () => {
    const ws = useWorkspacesStore()
    const d = deferred()
    wsList.mockImplementation(() => d.promise)
    const inflight = ws.loadWorkspaces()

    useAuthStore().clearSession()
    d.resolve({ data: [{ id: 'w1', name: 'Чужой воркспейс', my_role: 'owner' }] })
    await inflight

    expect(ws.list).toEqual([])
    expect(ws.canManage).toBe(false)
  })

  it('ответ переключения воркспейса не доливает чужое дерево назад', async () => {
    const ws = useWorkspacesStore()
    const g = deferred()
    const p = deferred()
    wsGroups.mockImplementation(() => g.promise)
    wsProjects.mockImplementation(() => p.promise)
    const inflight = ws.selectWorkspace('w1')

    useAuthStore().clearSession()
    g.resolve({ data: [{ id: 'g1', name: 'Секретная группа', parent_id: null, position: 0 }] })
    p.resolve({ data: [{ id: 'p1', name: 'Секретный проект', group_id: 'g1', position: 0 }] })
    await inflight

    expect(ws.groups).toEqual([])
    expect(ws.projects).toEqual([])
  })

  it('запомненный воркспейс остаётся — это свойство сервера, а не читателя', () => {
    const ws = useWorkspacesStore()
    seedTree(ws)
    useAuthStore().clearSession()
    // Ключ `tessera_ws` в serverStorage привязан к серверу, перезагрузка читает
    // его обратно в любом случае, а loadWorkspaces сам отбросит его, если новый
    // читатель там не состоит.
    expect(ws.currentId).toBe('w1')
  })
})

describe('выход из аккаунта уносит GitLab-привязку', () => {
  it('снимает connected/username и взводит loaded обратно', async () => {
    const gl = useGitlabStore()
    glConnection.mockImplementation(() =>
      Promise.resolve({ data: { connected: true, base_url: 'https://gl', gl_username: 'ivan' } }),
    )
    await gl.load()
    expect(gl.connected).toBe(true)
    expect(gl.loaded).toBe(true)

    useAuthStore().clearSession()

    expect(gl.connected).toBe(false)
    expect(gl.username).toBe('')
    expect(gl.baseUrl).toBe('')
    // GitLabModal перечитывает привязку, только пока loaded === false.
    expect(gl.loaded).toBe(false)
  })
})
