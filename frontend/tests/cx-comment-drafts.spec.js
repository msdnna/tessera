// The comment composer keeps one draft per task (#3118, PackageJ «Черновик в
// другой задаче»): the task modal swaps `taskId` under a mounted comments tab, and
// the half-written comment of task A must neither follow the reader into task B
// nor be lost when they come back.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})
const addComment = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    addComment: (...a) => addComment(...a),
    comments: vi.fn(() => Promise.resolve({ data: [] })),
    previewCommands: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

const EditorStub = {
  props: ['modelValue'],
  emits: ['update:modelValue', 'submit'],
  methods: {
    getMentions: () => [],
    clear() {
      this.$emit('update:modelValue', '')
    },
    autoGrow() {},
    focus() {},
  },
  template: '<div class="editor-stub" />',
}
const stubs = {
  MarkdownEditor: EditorStub,
  RichContent: true,
  UserAvatar: true,
  TesseraSpinner: true,
  EmptyState: true,
  teleport: true,
}
const tick = async (w, n = 6) => {
  for (let i = 0; i < n; i++) await w.vm.$nextTick()
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  vi.useFakeTimers()
})
afterEach(() => vi.useRealTimers())

async function open(taskId) {
  const Tab = (await import('@/components/task/TaskCommentsTab.vue')).default
  const w = mount(Tab, { props: { taskId, comments: [], hydrated: true }, global: { stubs } })
  await tick(w)
  return w
}
const composer = (w) => w.findAllComponents(EditorStub).at(-1)

describe('черновик комментария — свой у каждой задачи', () => {
  it('не уезжает в другую задачу и возвращается вместе с задачей', async () => {
    const w = await open('A')
    composer(w).vm.$emit('update:modelValue', 'мысль про A')
    await tick(w)
    await w.setProps({ taskId: 'B' })
    await tick(w)
    expect(composer(w).props('modelValue')).toBe('')
    await w.setProps({ taskId: 'A' })
    await tick(w)
    expect(composer(w).props('modelValue')).toBe('мысль про A')
    w.unmount()
  })

  it('переживает перезагрузку и пропадает после отправки', async () => {
    let w = await open('A')
    composer(w).vm.$emit('update:modelValue', 'черновик')
    await tick(w)
    await vi.advanceTimersByTimeAsync(600)
    expect(w.find('[data-testid="comment-draft"]').exists()).toBe(true)
    w.unmount()

    w = await open('A')
    expect(composer(w).props('modelValue')).toBe('черновик')
    composer(w).vm.$emit('submit')
    await tick(w)
    await vi.advanceTimersByTimeAsync(600)
    expect(addComment).toHaveBeenCalled()
    expect(localStorage.getItem('tessera_comment_draft:A')).toBeNull()
    w.unmount()
  })
})
