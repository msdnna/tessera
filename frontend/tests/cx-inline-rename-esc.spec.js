// Esc abandons an inline rename — and the click that follows must not commit it (#2932).
//
// The board's inline *create* fields have always answered Esc: «＋ задача»
// (KanbanBoard `@keyup.esc="cancelAddTask"`), «＋ колонка», a card's «＋ подзадача»,
// the matrix quick-add. The inline *rename* fields did not — the column header, the
// sidebar's project rename, the sidebar's board rename and the «новая доска» field
// each bound only `@keyup.enter` and `@blur`.
//
// Esc there did nothing visible: the input stayed open with the edited text still in
// it. A reader who pressed Esc and then clicked elsewhere — the habit the rest of the
// app trains — went out through `@blur`, and the rename they had just cancelled was
// PUT. Column and board names are workspace-shared data, broadcast to every other
// open client; this is not local view state that a stray write merely annoys.
//
// Two halves are pinned below: Esc closes the field without writing, and a commit
// that arrives after the field is closed (the blur of a removed node, a second call
// from any other path) is a no-op rather than a duplicate write.
//
// The card's own title is the fourth field of that shape and was missed when the
// three above were fixed: a board card renames inline on dblclick, commits on Enter
// AND on blur, and bound no Esc — so the same cancelled edit reached the server from
// the card instead of the column header.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

const columnsUpdate = vi.fn(() => Promise.resolve({ data: {} }))
const boardsUpdate = vi.fn(() => Promise.resolve({ data: {} }))
const tasksUpdate = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

vi.mock('@/api', () => ({
  columns: { update: columnsUpdate, remove: vi.fn() },
  boards: {
    update: boardsUpdate,
    remove: vi.fn(),
    create: vi.fn(),
    createTask: vi.fn(() => Promise.resolve({ data: {} })),
  },
  projects: {
    milestones: vi.fn(() => Promise.resolve({ data: [] })),
    createBoard: vi.fn(),
    createTag: vi.fn(),
  },
  tasks: {
    update: tasksUpdate,
    move: vi.fn(),
    setParent: vi.fn(),
    remove: vi.fn(),
    archive: vi.fn(),
    description: vi.fn(() => Promise.resolve({ data: { description: '' } })),
    addTag: vi.fn(),
    removeTag: vi.fn(),
    addAssignee: vi.fn(),
    removeAssignee: vi.fn(),
    pinGitlabAssignee: vi.fn(),
    removeGitlabAssignee: vi.fn(),
    dueNotify: vi.fn(),
  },
  users: { updatePrefs: vi.fn() },
}))

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const DCOL = { key: 'c1', name: 'В процессе', rawName: 'В процессе' }

// Мount editable (a real status column outside the archive) — that is the only
// state in which the rename opens at all.
const mountHead = () =>
  mount(ColumnHeader, {
    props: { dcol: DCOL, count: 3, total: 3, editable: true },
    global: { stubs: { TesseraIcon: true } },
  })

// The rename input replaces the caption (the kit's InlineEdit since #2969).
const renameInput = (w) => w.find('.ie-input')

async function openRename(w) {
  await w.find('.col-title').trigger('dblclick')
  return renameInput(w)
}

beforeEach(() => {
  columnsUpdate.mockClear()
  boardsUpdate.mockClear()
  tasksUpdate.mockClear()
})

describe('column header rename — Esc (#2932)', () => {
  it('opens the rename on dblclick with the current name', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    expect(inp.exists()).toBe(true)
    expect(inp.element.value).toBe(DCOL.name)
    w.unmount()
  })

  // The gesture that renamed a column nobody asked to rename.
  it('closes the field on Esc without writing', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('keydown', { key: 'Escape' })
    expect(columnsUpdate).not.toHaveBeenCalled()
    expect(w.find('.ie-input').exists()).toBe(false)
    expect(w.find('.col-title').text()).toBe(DCOL.name)
    w.unmount()
  })

  // …and the blur that a browser may still deliver on the way out stays silent.
  it('stays silent when a blur follows the Esc', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('keydown', { key: 'Escape' })
    await inp.trigger('blur')
    expect(columnsUpdate).not.toHaveBeenCalled()
    w.unmount()
  })

  // Esc must not become a second way to lose a rename that WAS wanted.
  it('still renames on Enter', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('keydown', { key: 'Enter' })
    expect(columnsUpdate).toHaveBeenCalledTimes(1)
    expect(columnsUpdate.mock.calls[0][1]).toMatchObject({ name: 'Готово' })
    w.unmount()
  })

  it('still renames on blur', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('blur')
    expect(columnsUpdate).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  // Enter commits and closes; the blur that follows the click away is not a second PUT.
  it('does not write twice when Enter is followed by a blur', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('keydown', { key: 'Enter' })
    await inp.trigger('blur')
    expect(columnsUpdate).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  // Reopening after an Esc starts from the column's name, not from the abandoned text.
  it('reopens seeded from the column, not from the cancelled edit', async () => {
    const w = mountHead()
    const inp = await openRename(w)
    await inp.setValue('Готово')
    await inp.trigger('keydown', { key: 'Escape' })
    const again = await openRename(w)
    expect(again.element.value).toBe(DCOL.name)
    w.unmount()
  })
})

// The sidebar's three inline fields are read off the source: ProjectRow mounts a
// whole subtree (stores, long-press composables, the milestone fetch) that this
// spec has no business standing up, and what regressed is a binding, not logic.
// Each field is matched by the handler it already had, then required to carry an
// Esc handler too — so the assertion fails both when a handler is dropped and when
// a fourth field of the same shape is added without one.
describe('sidebar inline fields — Esc (#2932)', async () => {
  const { readFileSync } = await import('node:fs')
  const { resolve } = await import('node:path')
  const process = (await import('node:process')).default
  const src = readFileSync(resolve(process.cwd(), 'src/components/ProjectRow.vue'), 'utf8')

  // Since #2969 the three commit-on-blur fields (project rename, board rename,
  // «новая доска») are the kit's InlineEdit, which owns the contract this file
  // pins: Esc cancels, the blur after it is silent, ↵ + blur writes once
  // (tests/cx-inline-edit.spec.js). What is left to pin here is that each of
  // them takes the cancel, and that no bare commit-on-blur `<n-input>` is back.
  // The tag ends at `/>` or at a `>` alone on its line — an attribute may hold `=>`.
  const fields = src.match(/<InlineEdit\b[\s\S]*?(?:\/>|\n\s*>)/g) || []

  it('finds the three inline fields', () => {
    expect(fields.length).toBe(3)
  })

  it('gives every inline field an Esc (cancel) handler', () => {
    for (const f of fields) expect(f).toMatch(/@cancel=/)
  })

  it('has no commit-on-blur n-input left', () => {
    const bare = (src.match(/<n-input\b[\s\S]*?\/>/g) || []).filter((i) => /@blur=/.test(i))
    expect(bare).toHaveLength(0)
  })

  // A commit arriving after the field closed must not write.
  it('guards the board rename against a commit on a closed field', () => {
    expect(src).toMatch(
      /async function commitBoardRename\(b, value, \{ via \}\)\s*\{\s*if \(editingBoardId\.value !== b\.id\) return/,
    )
  })
})

// The board card renames the same way the column header does — dblclick on the
// title, Enter or blur commits — and is mounted for real here (its own store
// context, the pill row and the child list are cheap enough) so the contract is
// pinned on behaviour rather than on the template text.
describe('card title rename — Esc (#2932)', () => {
  const TASK = { id: 't1', board_id: 'b1', column_id: 'c1', title: 'Родитель', number: 7 }
  const stubs = {
    ...naive,
    ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
    RichContent: true,
    DueEditor: true,
    draggable: {
      template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
      props: ['list'],
    },
  }

  beforeEach(() => {
    setActivePinia(createPinia())
    // The child list asks useResponsive whether it is on a phone, and jsdom has no
    // matchMedia — without this the card fails to mount at all.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
  })

  const mountCard = async () => {
    seedBoardStore({ columns: [{ id: 'c1', name: 'К работе', position: 1 }] })
    const TaskCard = (await import('@/components/TaskCard.vue')).default
    const w = mount(TaskCard, { props: { task: TASK }, global: { stubs } })
    await w.vm.$nextTick()
    return w
  }
  const openRename = async (w) => {
    await w.find('.title').trigger('dblclick')
    return w.find('.title-edit input')
  }

  it('opens the rename on dblclick with the current title', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    expect(inp.exists()).toBe(true)
    expect(inp.element.value).toBe(TASK.title)
    w.unmount()
  })

  // The gesture that renamed a card nobody asked to rename.
  it('closes the field on Esc without writing', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    await inp.setValue('Другое имя')
    await inp.trigger('keydown', { key: 'Escape' })
    expect(tasksUpdate).not.toHaveBeenCalled()
    expect(w.find('.title-edit input').exists()).toBe(false)
    expect(w.find('.title').text()).toBe(TASK.title)
    w.unmount()
  })

  // …and the blur that a browser may still deliver on the way out stays silent.
  it('stays silent when a blur follows the Esc', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    await inp.setValue('Другое имя')
    await inp.trigger('keydown', { key: 'Escape' })
    await inp.trigger('blur')
    expect(tasksUpdate).not.toHaveBeenCalled()
    w.unmount()
  })

  it('still renames on Enter, and the following blur is not a second PUT', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    await inp.setValue('Другое имя')
    await inp.trigger('keydown', { key: 'Enter' })
    await inp.trigger('blur')
    expect(tasksUpdate).toHaveBeenCalledTimes(1)
    expect(tasksUpdate.mock.calls[0][1]).toMatchObject({ title: 'Другое имя' })
    // Inline edits never carry the description — the card payload has none (#2665).
    expect(tasksUpdate.mock.calls[0][1]).not.toHaveProperty('description')
    w.unmount()
  })

  it('still renames on blur', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    await inp.setValue('Другое имя')
    await inp.trigger('blur')
    expect(tasksUpdate).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  // Reopening after an Esc starts from the task, not from the abandoned text.
  it('reopens seeded from the task, not from the cancelled edit', async () => {
    const w = await mountCard()
    const inp = await openRename(w)
    await inp.setValue('Другое имя')
    await inp.trigger('keydown', { key: 'Escape' })
    const again = await openRename(w)
    expect(again.element.value).toBe(TASK.title)
    w.unmount()
  })
})
