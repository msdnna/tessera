// Правка комментария рисовала чип упоминания, но никого не уведомляла (#2932).
//
// Редактор правки получает тот же :mention-items, что и основной композер, то
// есть @-попап предлагает участников и вставляет резолвнутый чип. А PATCH уходил
// одним только body: mentionIdsIn звался лишь на отправке нового комментария и
// ответа. Дописанное правкой «@Аня, посмотри» выглядело ровно как упоминание,
// о котором человеку сказали, — а Аня не узнавала никогда. utils/mentions.js
// объявляет это инвариантом прямым текстом: чип, который видит читатель, ЕСТЬ
// обещание, что человека уведомили.
//
// Считается и шлётся ДЕЛЬТА: сервер держит прежнее тело простым текстом, без
// ростера, против которого хэндл становится чипом, так что сам он «кого эта
// правка добавила» не вычислит — а полный набор пинговал бы всех названных
// заново на каждой правке опечатки.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const updateComment = vi.fn()
const comments = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    comments: (...a) => comments(...a),
    updateComment: (...a) => updateComment(...a),
    removeComment: vi.fn(),
    addComment: vi.fn(),
    previewCommands: vi.fn(() => Promise.resolve({ data: { commands: [], custom: [] } })),
  },
}))

const ME = 'u-me'
const MATE = 'u-mate'
// Аня без GitLab-аккаунта: её вставляемый хэндл — собственное имя.
const MEMBERS = {
  [ME]: { user_id: ME, name: 'Я', email: 'me@example.com', role: 'owner' },
  [MATE]: { user_id: MATE, name: 'Аня', email: 'anya@example.com', role: 'member' },
}
const MINE = {
  id: 'c-mine',
  author_id: ME,
  author_name: 'Я',
  body: 'Черновик готов',
  created_at: '2026-09-19T10:00:00Z',
  parent_id: null,
}

const stubs = {
  MarkdownEditor: true,
  RichContent: true,
  UserAvatar: true,
  TesseraSpinner: true,
  EmptyState: true,
  teleport: true,
}

const open = async (list = [MINE]) => {
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser({ id: ME, email: 'me@example.com', name: 'Я' })
  const { useBoardViewStore } = await import('@/stores/boardView')
  const bv = useBoardViewStore()
  bv.refill(bv.membersMap, MEMBERS)
  const TaskCommentsTab = (await import('@/components/task/TaskCommentsTab.vue')).default
  return mount(TaskCommentsTab, {
    props: { taskId: 't-1', comments: list, hydrated: true },
    global: { stubs },
  })
}

// Открывает правку комментария («✎» в его строке) и возвращает подставной
// редактор, который в ней появился.
const startEdit = async (w, id) => {
  const row = `[data-comment-id="${id}"]`
  const pencil = w.find(`${row} [data-testid="comment-edit"]`)
  expect(pencil.exists()).toBe(true)
  await pencil.trigger('click')
  const editor = w
    .findAllComponents({ name: 'MarkdownEditor' })
    .find((e) => e.element?.closest?.(row))
  expect(editor).toBeTruthy()
  return editor
}

// Пишет в редактор правки и сохраняет (@submit — то же, что кнопка «Сохранить»).
const saveEdit = async (editor, body) => {
  editor.vm.$emit('update:modelValue', body)
  await editor.vm.$nextTick()
  editor.vm.$emit('submit')
  await new Promise((r) => setTimeout(r, 0))
}

describe('упоминания при правке комментария', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    updateComment.mockReset().mockResolvedValue({ data: {} })
    comments.mockReset().mockResolvedValue({ data: [MINE] })
  })

  it('уведомляет того, кого правка дописала', async () => {
    const w = await open()
    const editor = await startEdit(w, MINE.id)
    await saveEdit(editor, 'Черновик готов. @Аня, посмотри')

    expect(updateComment).toHaveBeenCalledWith(MINE.id, 'Черновик готов. @Аня, посмотри', [MATE])
    w.unmount()
  })

  it('правка опечатки не пингует названных заново', async () => {
    const w = await open([{ ...MINE, body: 'Черновик готов. @Аня, посмотри' }])
    const editor = await startEdit(w, MINE.id)
    await saveEdit(editor, 'Черновик готов. @Аня, посмотри его')

    expect(updateComment).toHaveBeenCalledWith(MINE.id, 'Черновик готов. @Аня, посмотри его', [])
    w.unmount()
  })

  it('хэндл в блоке кода никого не зовёт', async () => {
    // Та же граница, что у отправки нового комментария: чипа нет — значит и
    // обещания, что человеку сказали, нет (mentionIdsIn читает отрисованное).
    const w = await open()
    const editor = await startEdit(w, MINE.id)
    await saveEdit(editor, 'Черновик готов.\n\n```\n@Аня\n```')

    expect(updateComment.mock.calls[0][2]).toEqual([])
    w.unmount()
  })

  it('галочка в чеклисте остаётся правкой без упоминаний', async () => {
    // Тот же PATCH, но текст переписывает маркер — звать по нему некого.
    const list = [{ ...MINE, body: '- [ ] собрать смету' }]
    const w = await open(list)
    const rich = w
      .findAllComponents({ name: 'RichContent' })
      .find((r) => r.element?.closest?.(`[data-comment-id="${MINE.id}"]`))
    rich.vm.$emit('toggle', 0)
    await new Promise((r) => setTimeout(r, 0))

    expect(updateComment).toHaveBeenCalledWith(MINE.id, '- [x] собрать смету')
    w.unmount()
  })
})
