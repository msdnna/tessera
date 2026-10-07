// Deleting a board (v2 interaction model, #2969): it happens at once — from the
// row's right-click menu (also a long press in the mobile drawer) or its «…» —
// and the undo toast brings it back. The server keeps a deleted board
// restorable (migration 0078), so «Отменить» is a real restore, not a re-create.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const push = vi.fn()
const route = { params: {}, query: {}, path: '/' }
vi.mock('vue-router', () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  useRoute: () => route,
}))

const removeBoard = vi.fn(() => Promise.resolve({}))
const restoreBoard = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  projects: {
    remove: vi.fn(),
    update: vi.fn(),
    boards: vi.fn(() => Promise.resolve({ data: [] })),
  },
  boards: {
    remove: (...a) => removeBoard(...a),
    restore: (...a) => restoreBoard(...a),
    update: vi.fn(),
    create: vi.fn(),
  },
}))

const PROJECT = { id: 'p1', name: 'Проект', slug: 'proj', color: '#7c5cff' }
const BOARD = { id: 'b1', name: 'Ретро', slug: 'retro' }

// useTreeExpand reads its persisted state once, at import time: the project has
// to be marked open before ProjectRow is pulled in, or the board rows never render.
localStorage.setItem('tessera_tree_expanded', JSON.stringify({ [PROJECT.id]: true }))

const { default: ProjectRow } = await import('@/components/ProjectRow.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')
const { useUndoStore } = await import('@/stores/undo')

function mountRow() {
  setActivePinia(createPinia())
  const store = useWorkspacesStore()
  store.boardsByProject = { [PROJECT.id]: [BOARD] }
  store.loadBoards = vi.fn(() => Promise.resolve())
  return mount(ProjectRow, {
    attachTo: document.body,
    props: { project: PROJECT },
    global: {
      stubs: {
        ProjectIcon: true,
        TesseraIcon: true,
        IconColorPicker: true,
        ConfirmByName: true,
        EstimationModal: true,
        MilestoneManager: true,
      },
    },
  })
}
async function deleteFromMenu(w) {
  await w.get('.board-row').trigger('contextmenu', { clientX: 10, clientY: 10 })
  await nextTick()
  document.body.querySelector('[data-testid="board-actions-delete"]').click()
  await flushPromises()
}

describe('ProjectRow board delete', () => {
  beforeEach(() => {
    removeBoard.mockClear()
    restoreBoard.mockClear()
    push.mockClear()
    route.params = {}
  })

  it('deletes at once and offers the undo', async () => {
    const w = mountRow()
    await deleteFromMenu(w)
    expect(removeBoard).toHaveBeenCalledWith('b1')
    const toasts = useUndoStore().toasts
    expect(toasts).toHaveLength(1)
    expect(toasts[0].text()).toBe('Доска «Ретро» удалена')
    w.unmount()
  })

  it('«Отменить» restores the board', async () => {
    const w = mountRow()
    await deleteFromMenu(w)
    const undo = useUndoStore()
    await undo.undo(undo.toasts[0].id)
    expect(restoreBoard).toHaveBeenCalledWith('b1')
    expect(undo.toasts[0].status).toBe('done')
    w.unmount()
  })

  it('an open board sends the reader home, and the undo brings them back', async () => {
    route.params = { projectSlug: 'proj', boardSlug: 'retro' }
    const w = mountRow()
    await deleteFromMenu(w)
    expect(push).toHaveBeenCalledWith('/')
    const undo = useUndoStore()
    await undo.undo(undo.toasts[0].id)
    expect(push).toHaveBeenLastCalledWith('/project/proj/board/retro')
    w.unmount()
  })
})
