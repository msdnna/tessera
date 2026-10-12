import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

// What a reconnect re-reads (#2932).
//
// The server closes a user's sockets on every membership change (Hub.DropUser):
// add, remove, role change, workspace delete. That close is the only way such a
// change reaches an open session, and what it buys is a resync — so the resync
// has to re-read the workspace *set*, not just the current workspace's tree. It
// did not, and `my_role` (all of `canManage`) plus sidebar membership stayed at
// whatever they were when the page was loaded.
const apiMock = vi.hoisted(() => ({
  workspaces: {
    list: vi.fn(),
    groups: vi.fn(() => Promise.resolve({ data: [] })),
    projects: vi.fn(() => Promise.resolve({ data: [] })),
    commands: vi.fn(() =>
      Promise.resolve({ data: { builtin: [], custom: [], can_manage: false } }),
    ),
    remove: vi.fn(() => Promise.resolve()),
  },
  projects: { boards: vi.fn(() => Promise.resolve({ data: [] })) },
}))
vi.mock('@/api', () => apiMock)
vi.mock('@/composables/useTreeExpand', () => ({
  useTreeExpand: () => ({ isExpanded: () => false }),
}))

import { useWorkspacesStore } from '@/stores/workspaces'

const ws = (id, role, name = id) => ({ id, name, my_role: role })

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
})
afterEach(() => localStorage.clear())

// Start every case from a loaded app: one workspace, plain member in it.
async function loaded(list = [ws('w1', 'member'), ws('w2', 'member')]) {
  apiMock.workspaces.list.mockResolvedValue({ data: list })
  const store = useWorkspacesStore()
  await store.loadWorkspaces()
  vi.clearAllMocks()
  apiMock.workspaces.groups.mockResolvedValue({ data: [] })
  apiMock.workspaces.projects.mockResolvedValue({ data: [] })
  apiMock.workspaces.commands.mockResolvedValue({
    data: { builtin: [], custom: [], can_manage: false },
  })
  apiMock.projects.boards.mockResolvedValue({ data: [] })
  return store
}

describe('workspaces store — resync', () => {
  it('picks up a promotion to admin without a page reload', async () => {
    const store = await loaded()
    expect(store.canManage).toBe(false)

    // Somebody promoted us while the tab was open; the socket was cut, we
    // reconnected, and this is the only place the new role can arrive.
    apiMock.workspaces.list.mockResolvedValue({
      data: [ws('w1', 'admin'), ws('w2', 'member')],
    })
    await store.resync()

    expect(apiMock.workspaces.list).toHaveBeenCalled()
    expect(store.canManage).toBe(true)
  })

  it('picks up a demotion too — a stale admin keeps controls that 403', async () => {
    const store = await loaded([ws('w1', 'admin')])
    expect(store.canManage).toBe(true)

    apiMock.workspaces.list.mockResolvedValue({ data: [ws('w1', 'member')] })
    await store.resync()

    expect(store.canManage).toBe(false)
  })

  it('shows a workspace we were added to while the socket was down', async () => {
    const store = await loaded([ws('w1', 'member')])
    expect(store.list.map((w) => w.id)).toEqual(['w1'])

    apiMock.workspaces.list.mockResolvedValue({
      data: [ws('w1', 'member'), ws('w3', 'member')],
    })
    await store.resync()

    expect(store.list.map((w) => w.id)).toEqual(['w1', 'w3'])
    // Still looking at the same board: a resync also fires after a plain
    // network outage, and must not move the reader.
    expect(store.currentId).toBe('w1')
  })

  it('keeps the selection, and refreshes its tree, when membership is unchanged', async () => {
    const store = await loaded([ws('w1', 'member'), ws('w2', 'member')])
    await store.selectWorkspace('w2')
    vi.clearAllMocks()
    apiMock.workspaces.list.mockResolvedValue({
      data: [ws('w1', 'member'), ws('w2', 'member')],
    })
    apiMock.workspaces.groups.mockResolvedValue({ data: [{ id: 'g1' }] })
    apiMock.workspaces.projects.mockResolvedValue({ data: [{ id: 'p1' }] })

    await store.resync()

    expect(store.currentId).toBe('w2')
    expect(store.groups).toEqual([{ id: 'g1' }])
    expect(store.projects).toEqual([{ id: 'p1' }])
    // refresh(), not selectWorkspace(): re-selecting would drop the loaded
    // boards and re-read the command registry for no reason.
    expect(apiMock.workspaces.commands).not.toHaveBeenCalled()
  })

  it('moves off a workspace we were removed from, onto one that is still ours', async () => {
    const store = await loaded([ws('w1', 'member'), ws('w2', 'member')])
    await store.selectWorkspace('w2')
    vi.clearAllMocks()
    apiMock.workspaces.list.mockResolvedValue({ data: [ws('w1', 'member')] })
    apiMock.workspaces.groups.mockResolvedValue({ data: [] })
    apiMock.workspaces.projects.mockResolvedValue({ data: [] })
    apiMock.workspaces.commands.mockResolvedValue({
      data: { builtin: [], custom: [], can_manage: false },
    })

    await store.resync()

    expect(store.currentId).toBe('w1')
    // A real switch this time, so the whole workspace is loaded, not just the tree.
    expect(apiMock.workspaces.commands).toHaveBeenCalledWith('w1')
  })

  it('empties the tree when the last workspace is gone', async () => {
    const store = await loaded([ws('w1', 'member')])
    apiMock.workspaces.groups.mockResolvedValue({ data: [{ id: 'g1' }] })
    apiMock.workspaces.projects.mockResolvedValue({ data: [{ id: 'p1' }] })
    await store.refresh()
    expect(store.projects).toEqual([{ id: 'p1' }])

    apiMock.workspaces.list.mockResolvedValue({ data: [] })
    await store.resync()

    expect(store.currentId).toBe('')
    expect(store.list).toEqual([])
    // Otherwise the sidebar keeps offering projects every click 403s on.
    expect(store.groups).toEqual([])
    expect(store.projects).toEqual([])
  })
})
