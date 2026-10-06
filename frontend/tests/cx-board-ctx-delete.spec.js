// Sibling of cx-column-ctx-delete: the board row's right-click menu. Deleting a
// board takes its columns and every card on them, and the same menu opens on a
// long press inside the mobile drawer — so the menu entry must only arm the
// confirm the board's «…» popover already shows, never call the API itself.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { NDropdown, NPopconfirm } from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ params: {}, query: {}, path: '/' }),
}))

const removeBoard = vi.fn()
vi.mock('@/api', () => ({
  projects: { remove: vi.fn(), update: vi.fn() },
  boards: { remove: (...a) => removeBoard(...a), update: vi.fn(), create: vi.fn() },
}))

const PROJECT = { id: 'p1', name: 'Проект', slug: 'proj', color: '#7c5cff' }
const BOARD = { id: 'b1', name: 'Доска', slug: 'board' }

// useTreeExpand reads its persisted state once, at import time: the project has
// to be marked open before ProjectRow is pulled in, or the board rows (and the
// context menu hanging off them) never render.
localStorage.setItem('tessera_tree_expanded', JSON.stringify({ [PROJECT.id]: true }))

const { default: ProjectRow } = await import('@/components/ProjectRow.vue')
const { useWorkspacesStore } = await import('@/stores/workspaces')

function mountRow() {
  setActivePinia(createPinia())
  useWorkspacesStore().boardsByProject = { [PROJECT.id]: [BOARD] }
  return mount(ProjectRow, {
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

// The board menu's confirm is the coordinate-anchored one; the popover's own
// delete confirm carries no :x/:y.
const ctxConfirm = (w) => w.findAllComponents(NPopconfirm).find((p) => p.props('x') !== undefined)
// The board dropdown is the last manual one in the row (project menu comes first).
const boardMenu = (w) => w.findAllComponents(NDropdown).at(-1)

async function openBoardMenu(w) {
  await w.get('.board-row').trigger('contextmenu')
  await w.vm.$nextTick()
  boardMenu(w).vm.$emit('select', 'delete')
  await w.vm.$nextTick()
}

describe('ProjectRow right-click board delete', () => {
  beforeEach(() => removeBoard.mockClear())

  it('asks before deleting instead of calling the API from the menu', async () => {
    const w = mountRow()
    await openBoardMenu(w)

    expect(removeBoard).not.toHaveBeenCalled()
    expect(ctxConfirm(w).props('show')).toBe(true)
    w.unmount()
  })

  it('deletes once the confirm is accepted', async () => {
    const w = mountRow()
    await openBoardMenu(w)

    ctxConfirm(w).vm.$emit('positive-click')
    await w.vm.$nextTick()

    expect(removeBoard).toHaveBeenCalledWith('b1')
    w.unmount()
  })

  it('leaves the board alone when the confirm is dismissed', async () => {
    const w = mountRow()
    await openBoardMenu(w)

    ctxConfirm(w).vm.$emit('clickoutside')
    await w.vm.$nextTick()

    expect(removeBoard).not.toHaveBeenCalled()
    expect(ctxConfirm(w).props('show')).toBe(false)
    w.unmount()
  })
})
