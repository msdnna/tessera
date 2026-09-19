// TaskModal and the answer that arrives too late (#2932).
//
// There is ONE TaskModal for the whole board and it is never remounted: the board
// keeps it mounted and swaps `taskId` underneath it (a subtask row, a relation, the
// «Родитель» link and an activity toast all re-`@open` it, and so does ?task= in the
// URL). So two detail loads can be in flight at once, and nothing said which answer
// belonged to the task currently on screen.
//
// That is not a cosmetic race. The load writes `title`/`description`/`priority`/
// `due_date`/`completed` — the very fields Save sends — while Save addresses
// `props.taskId`. A slow answer for the task the reader has LEFT lands in the form of
// the task they are now looking at, and the next Save writes the first task's content
// over the second one's. No error, no flicker: the reader sees a title they did not
// type only if they happen to look up.
//
// The extras (comments/relations/attachments/history) race the same way, one screen
// lower: the previous task's thread under this task's header reads as this task's.
//
// The idiom this pins is the one SearchBar already uses for the same reason — a
// sequence number taken before the first early return, re-checked after every await.
// The happy path is asserted alongside, because a guard that drops EVERY answer would
// satisfy the staleness assertions on its own.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { seedBoardStore } from './helpers/boardStore'

const COLS = [
  { id: 'c1', name: 'К работе', color: '#9aa0aa', position: 1 },
  { id: 'c2', name: 'Готово', color: '#18a058', position: 2 },
]

function task(id, title) {
  return {
    id,
    number: id === 't1' ? 1 : 2,
    board_id: 'b1',
    column_id: 'c1',
    title,
    description: 'Описание ' + title,
    priority: id === 't1' ? 3 : 0,
    subtasks: [],
    tags: [],
    assignees: [],
  }
}
const A = task('t1', 'Задача A')
const B = task('t2', 'Задача B')

// A promise whose resolution this spec controls, so the two loads can be made to
// finish in the wrong order on purpose.
function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

// Detail + comments per task id, each answered only when the spec says so.
let pending = {}
function arm(id) {
  pending[id] = { detail: deferred(), comments: deferred() }
}
function landDetail(id) {
  pending[id].detail.resolve({ data: id === 't1' ? A : B })
}
function landComments(id) {
  pending[id].comments.resolve({ data: [{ id: 'c-' + id, body: 'Комментарий ' + id }] })
}
function land(id) {
  landDetail(id)
  landComments(id)
}

const update = vi.fn((id, payload) => Promise.resolve({ data: { ...B, ...payload, id } }))
const empty = () => Promise.resolve({ data: [] })

vi.mock('@/api', () => ({
  tasks: {
    get: vi.fn((id) => pending[id].detail.promise),
    comments: vi.fn((id) => pending[id].comments.promise),
    update: (...a) => update(...a),
    move: vi.fn(() => Promise.resolve({ data: {} })),
    relations: vi.fn(empty),
    attachments: vi.fn(empty),
    events: vi.fn(empty),
    documents: vi.fn(empty),
  },
  boards: {
    get: vi.fn(() =>
      Promise.resolve({ data: { name: 'Доска', project_id: 'p1', done_column_id: 'c2' } }),
    ),
    columns: vi.fn(() => Promise.resolve({ data: COLS })),
    tasks: vi.fn(empty),
    createTask: vi.fn(),
  },
  workspaces: { members: vi.fn(empty) },
  projects: { createMilestone: vi.fn() },
  gitlab: { issueTemplates: vi.fn(empty) },
  columns: { update: vi.fn(), remove: vi.fn() },
  users: { updatePrefs: vi.fn() },
}))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ path: '/', params: {}, query: {} }),
}))

// Naive stubs, keyed by both names — TaskModal is <script setup>, so its tags
// resolve to the imported bindings and VTU matches the component's own `name`
// ("Popover"), not the "NPopover" alias (see cx-task-status.spec.js).
const naive = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub" v-bind="$attrs"><slot /></button>' },
  // The title input has to render its value: this spec reads the form back off the
  // DOM, which is the only place the reader sees it either.
  // inheritAttrs:false — Naive takes size="small"/"tiny", which a bare <input>
  // rejects — but the class has to be forwarded by hand then, and this spec finds
  // the title by it.
  Input: {
    inheritAttrs: false,
    props: ['value'],
    template: '<input class="n-input-stub" :class="$attrs.class" :value="value" />',
  },
  Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
  // The footer slot has to be rendered: Save lives there, and this spec presses it.
  Card: { template: '<div class="n-card-stub"><slot /><slot name="footer" /></div>' },
  Spin: { template: '<div class="n-spin-stub"><slot /></div>' },
  Tabs: { template: '<div class="n-tabs-stub"><slot /></div>' },
  TabPane: { template: '<div class="n-tab-pane-stub"><slot /></div>' },
  Select: { template: '<div class="n-select-stub" />' },
  Badge: { template: '<span class="n-badge-stub" />' },
  Space: { template: '<div class="n-space-stub"><slot /></div>' },
  Popover: { template: '<div class="n-popover-stub"><slot name="trigger" /><slot /></div>' },
  Popconfirm: { template: '<div class="n-popconfirm-stub"><slot name="trigger" /><slot /></div>' },
  Dropdown: { template: '<div class="n-dropdown-stub" />' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /><slot /></div>' },
}

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  MarkdownEditor: true,
  RichContent: true,
  DueEditor: true,
  // Captures what the thread was handed, which is what the reader would read.
  TaskCommentsTab: {
    props: ['comments'],
    // The modal calls into the tab by ref (scroll-to-newest on open, composer
    // reset on close); a stub without them turns every load into an unhandled
    // rejection that hides the assertion under it.
    methods: { scrollToBottom() {}, resetComposers() {}, autoGrow() {} },
    template: '<div class="comments-stub">{{ (comments || []).map((c) => c.body).join() }}</div>',
  },
  draggable: {
    template: '<div><slot name="item" v-for="e in list" :element="e" :index="0" /></div>',
    props: ['list'],
  },
}

const flush = async (w) => {
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
  await new Promise((r) => setTimeout(r, 0))
  for (let i = 0; i < 8; i++) await w.vm.$nextTick()
}

const titleOf = (w) => w.find('.title-input').element.value
const threadOf = (w) => w.find('.comments-stub').text()

let TaskModal
beforeEach(async () => {
  vi.clearAllMocks()
  pending = {}
  setActivePinia(createPinia())
  // Board context absent on purpose: that is the deep-link / cross-board open, the
  // path where the modal fetches the board itself and the detail load is slowest.
  seedBoardStore({ board: null, columns: [] })
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
  ;({ default: TaskModal } = await import('@/components/TaskModal.vue'))
})

// Mount closed and open: the loader hangs off a non-immediate watcher on `show`.
async function openOn(id) {
  arm(id)
  const w = mount(TaskModal, { props: { show: false, taskId: id }, global: { stubs } })
  await w.setProps({ show: true })
  return w
}

// Open A (answer withheld), switch to B the way a subtask/relation/toast click does,
// let B answer, and only then let A's late answer land.
async function raceAThenB(w) {
  arm('t2')
  await w.setProps({ taskId: 't2' })
  land('t2')
  await flush(w)
  land('t1')
  await flush(w)
}

describe('TaskModal: a detail load that answers after the task changed', () => {
  it('leaves the open task on screen instead of the one the reader left', async () => {
    const w = await openOn('t1')
    await raceAThenB(w)
    expect(titleOf(w)).toBe('Задача B')
  })

  it('does not write the previous task over this one when the reader saves', async () => {
    const w = await openOn('t1')
    await raceAThenB(w)
    w.findAll('.n-button-stub')
      .find((b) => b.attributes('data-tour') === 'tm-save')
      .trigger('click')
    await flush(w)
    expect(update).toHaveBeenCalledTimes(1)
    const [id, payload] = update.mock.calls[0]
    expect(id).toBe('t2')
    expect(payload.title).toBe('Задача B')
    expect(payload.description).toBe('Описание Задача B')
    expect(payload.priority).toBe(0)
  })

  // The extras race on their own clock: the detail can answer while the reader is
  // still on the first task (so the thread is fetched under ITS id) and the slow
  // half — comments/relations/attachments/history — land after the switch.
  it('does not hang the previous task’s thread under this task', async () => {
    const w = await openOn('t1')
    landDetail('t1')
    await flush(w)
    arm('t2')
    await w.setProps({ taskId: 't2' })
    land('t2')
    await flush(w)
    landComments('t1')
    await flush(w)
    expect(threadOf(w)).toBe('Комментарий t2')
  })

  // The guard must invalidate, not merely reorder: a modal that was closed (taskId
  // dropped) must not repopulate itself when the request it left behind answers.
  it('does not repopulate a closed modal when its own answer arrives late', async () => {
    const w = await openOn('t1')
    await w.setProps({ show: false, taskId: null })
    land('t1')
    await flush(w)
    expect(titleOf(w)).not.toBe('Задача A')
  })

  it('still loads a task normally when nothing raced it', async () => {
    const w = await openOn('t1')
    land('t1')
    await flush(w)
    expect(titleOf(w)).toBe('Задача A')
    expect(threadOf(w)).toBe('Комментарий t1')
  })

  it('still loads the second task when the first one answered first', async () => {
    const w = await openOn('t1')
    land('t1')
    await flush(w)
    arm('t2')
    await w.setProps({ taskId: 't2' })
    land('t2')
    await flush(w)
    expect(titleOf(w)).toBe('Задача B')
    expect(threadOf(w)).toBe('Комментарий t2')
  })
})
