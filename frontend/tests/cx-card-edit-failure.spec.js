// A card edit the server refused must say so — and must not eat what was typed (#2932).
//
// Every write reachable from a board card (rename, complete, priority, tags,
// assignees, a child's checkbox) was `await api…()` followed by `emit('changed')`
// with nothing around it. A rejected promise therefore left: no toast, no resync,
// and — for the rename — no text, because `commitTitle` closes the inline editor
// BEFORE the PUT goes out. The reader sees the old title back and has no way to
// tell a refusal from a no-op.
//
// The rule is already written down twice in this codebase: the right-click menu
// over the very same card wraps each of these calls and resyncs on failure
// (`composables/useTaskMenu.js`), and KanbanBoard's `restoreFailedTitle` puts a
// typed title back into the input a failed create emptied — «retyping it is the
// reader's work the error has no claim on». These specs hold the card's own
// surface to the same rule.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

// One message bus shared by every component under test, so a spec can assert on
// the toast whichever component raised it. Injected under the key naive's own
// `useMessage()` reads (see tests/setup.js) rather than mocked into the module:
// that way these specs also pin that the components really are wired to the
// provider App.vue puts above them.
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }

const update = vi.fn(() => Promise.resolve({ data: {} }))
const addAssignee = vi.fn(() => Promise.resolve({ data: {} }))
const addTag = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    update: (...a) => update(...a),
    move: vi.fn(() => Promise.resolve({ data: {} })),
    setParent: vi.fn(() => Promise.resolve({ data: {} })),
    remove: vi.fn(),
    archive: vi.fn(),
    description: vi.fn(() => Promise.resolve({ data: { description: '' } })),
    addTag: (...a) => addTag(...a),
    removeTag: vi.fn(() => Promise.resolve({ data: {} })),
    addAssignee: (...a) => addAssignee(...a),
    removeAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    pinGitlabAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    removeGitlabAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    dueNotify: vi.fn(() => Promise.resolve({ data: {} })),
  },
  boards: { createTask: vi.fn(() => Promise.resolve({ data: {} })) },
  projects: { createTag: vi.fn(() => Promise.resolve({ data: {} })) },
  users: { updatePrefs: vi.fn() },
  columns: { update: vi.fn(), remove: vi.fn() },
}))

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'В процессе', color: '#2f80ed', position: 2 },
]
const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Родитель',
  number: 7,
  priority: 2,
  assignee_ids: [],
  tag_ids: [],
}
const SUBS = [{ id: 's2', title: 'Вторая', column_id: 'c1', parent_id: 't1', completed_at: null }]
const MEMBERS = [{ user_id: 'u1', name: 'Алиса' }]
const TAGS = [{ id: 'g1', name: 'баг', color: '#e0533d' }]

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  RichContent: true,
  DuePicker: true,
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

// The rejection every spec below drives the UI with. `api/index.js` normalises a
// server refusal into an Error, which is what the existing handlers print.
const refuse = () => Promise.reject(new Error('нет прав на задачу'))

async function settle() {
  for (let i = 0; i < 4; i++) await nextTick()
}

let wrapper = null
async function mountCard(props = {}, seed = {}) {
  seedBoardStore({ columns: COLS, cardSize: 'large', members: MEMBERS, tags: TAGS, ...seed })
  const TaskCard = (await import('@/components/TaskCard.vue')).default
  wrapper = mount(TaskCard, {
    props: { task: TASK, ...props },
    global: { stubs, provide: { 'n-message-api': toast } },
    attachTo: document.body,
  })
  await settle()
  return wrapper
}

// The pickers teleport their body to <body>, so rows are queried off the document.
const menuRows = () => Array.from(document.body.querySelectorAll('.menu .menu-item'))

beforeEach(() => {
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  update.mockReset()
  update.mockImplementation(() => Promise.resolve({ data: {} }))
  addAssignee.mockReset()
  addAssignee.mockImplementation(() => Promise.resolve({ data: {} }))
  addTag.mockReset()
  addTag.mockImplementation(() => Promise.resolve({ data: {} }))
  toast.error.mockClear()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('a refused card edit is reported and resynced', () => {
  it('says why completing the card did nothing, and asks the board to resync', async () => {
    const w = await mountCard()
    update.mockImplementationOnce(refuse)

    await w.find('.ca-complete').trigger('click')
    await settle()

    expect(toast.error).toHaveBeenCalledWith('нет прав на задачу')
    // The card still draws `task.completed_at` from its prop, so the board has to
    // be told to re-read it — otherwise a card the server DID change (a racing
    // second client) keeps the state this click failed to reach.
    expect(w.emitted('changed')).toBeTruthy()
  })

  it('keeps a refused rename in the editor instead of dropping the typed title', async () => {
    const w = await mountCard()
    await w.find('.title').trigger('dblclick')
    await settle()
    await w.find('.title-edit input').setValue('Новое имя')
    update.mockImplementationOnce(refuse)

    await w.find('.title-edit input').trigger('keydown', { key: 'Enter' })
    await settle()

    expect(toast.error).toHaveBeenCalledWith('нет прав на задачу')
    const input = w.find('.title-edit input')
    expect(input.exists()).toBe(true)
    expect(input.element.value).toBe('Новое имя')
  })

  it('does not yank the editor back if the reader has already moved on', async () => {
    const w = await mountCard()
    await w.find('.title').trigger('dblclick')
    await settle()
    await w.find('.title-edit input').setValue('Первое')

    // The PUT is in flight; hold it open so the reader can act meanwhile.
    let refuseIt = null
    update.mockImplementationOnce(
      () => new Promise((_, rej) => (refuseIt = () => rej(new Error('нет прав на задачу')))),
    )
    await w.find('.title-edit input').trigger('keydown', { key: 'Enter' })
    await settle()

    // …and they reopen the editor and type something else. Their text wins.
    await w.find('.title').trigger('dblclick')
    await settle()
    await w.find('.title-edit input').setValue('Второе')

    refuseIt()
    await settle()

    expect(w.find('.title-edit input').element.value).toBe('Второе')
  })

  it('leaves a rename that went through alone', async () => {
    const w = await mountCard()
    await w.find('.title').trigger('dblclick')
    await settle()
    await w.find('.title-edit input').setValue('Новое имя')

    await w.find('.title-edit input').trigger('keydown', { key: 'Enter' })
    await settle()

    expect(toast.error).not.toHaveBeenCalled()
    expect(w.find('.title-edit').exists()).toBe(false)
    expect(w.emitted('changed')).toBeTruthy()
  })

  it('reports a refused priority pick from the pill row', async () => {
    const w = await mountCard()
    await w.find('.pill').trigger('click')
    await settle()
    const rows = menuRows()
    expect(rows.length).toBeGreaterThan(0)
    update.mockImplementationOnce(refuse)

    rows[0].dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await settle()

    expect(toast.error).toHaveBeenCalledWith('нет прав на задачу')
  })

  it("reports a refused toggle of a child's checkbox", async () => {
    const w = await mountCard({ subtasks: SUBS, subtasksTotal: 1 })
    update.mockImplementationOnce(refuse)

    await w.find('.check.sm').trigger('click')
    await settle()

    expect(toast.error).toHaveBeenCalledWith('нет прав на задачу')
    expect(w.emitted('changed')).toBeTruthy()
  })
})
