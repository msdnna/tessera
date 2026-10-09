// Удаление корня ветки на вебе рассыпало саму ветку. Бэкенд на удалении корня
// НЕ удаляет ветку: promoteThreadSuccessor поднимает старший ответ в корень и
// перевешивает под него остальные — ровно для того, чтобы одно «✕» не уносило
// чужой текст. Вкладка комментариев же выкидывала строку локальным фильтром, и
// у ответов оставался parent_id на исчезнувшую строку. groupThreads на такой
// список отвечает фолбэком «родителя нет — рисуем как корень», так что ветка на
// экране распадалась на отдельные верхнеуровневые карточки (без рельса, отступа
// и счётчика ответов) и оставалась такой до переоткрытия модалки. Android на том
// же бэкенде перечитывает список, и соседний saveComment здесь же — тоже (#2932).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import ArmButton from '@/components/kit/ArmButton.vue'
import { groupThreads } from '@/utils/taskFeed'

const errors = []
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({
      error: (m) => errors.push(m),
      success: vi.fn(),
      info: vi.fn(),
      warning: vi.fn(),
    }),
  }
})
// The message API moved out of naive (composables/useMessage, #3117): route it
// to the naive mock above, where this spec keeps its spies.
vi.mock('@/composables/useMessage', async () => ({
  useMessage: (await import('naive-ui')).useMessage,
}))

const removeComment = vi.fn()
const comments = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    comments: (...a) => comments(...a),
    removeComment: (...a) => removeComment(...a),
    addComment: vi.fn(),
    updateComment: vi.fn(),
    previewCommands: vi.fn(() => Promise.resolve({ data: { commands: [], custom: [] } })),
  },
}))

const ME = 'u-me'
// Ветка: корень мой, два ответа — чужие. Удалять можно только свой комментарий,
// поэтому «✕» есть ровно у корня.
const ROOT = { id: 'c-root', author_id: ME, author_name: 'Я', body: 'вопрос', created_at: '2026-09-19T10:00:00Z', parent_id: null } // prettier-ignore
const R1 = { id: 'c-r1', author_id: 'u-a', author_name: 'Аня', body: 'ответ 1', created_at: '2026-09-19T10:05:00Z', parent_id: 'c-root' } // prettier-ignore
const R2 = { id: 'c-r2', author_id: 'u-b', author_name: 'Боря', body: 'ответ 2', created_at: '2026-09-19T10:09:00Z', parent_id: 'c-root' } // prettier-ignore

// То, что реально лежит в базе после DELETE корня: старший ответ стал корнем,
// младший перевешен под него.
const AFTER_PROMOTION = [
  { ...R1, parent_id: null },
  { ...R2, parent_id: R1.id },
]

const stubs = {
  MarkdownEditor: true,
  RichContent: true,
  UserAvatar: true,
  TesseraSpinner: true,
  EmptyState: true,
  teleport: true,
}

const open = async (list = [ROOT, R1, R2]) => {
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser({ id: ME, email: 'me@example.com', name: 'Я' })
  const TaskCommentsTab = (await import('@/components/task/TaskCommentsTab.vue')).default
  return mount(TaskCommentsTab, {
    props: { taskId: 't-1', comments: list, hydrated: true },
    global: { stubs },
  })
}

// Щёлкает «удалить» у корня ветки: кнопка двухшаговая (v2, #3113) — первое
// нажатие её лишь взводит, вызов идёт на `confirm`.
const confirmDelete = async (w, id) => {
  const pc = w
    .findAllComponents(ArmButton)
    .find((p) => p.element?.closest?.(`[data-comment-id="${id}"]`))
  expect(pc).toBeTruthy()
  pc.vm.$emit('confirm')
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
}

describe('удаление комментария во вкладке «Комментарии»', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    errors.length = 0
    removeComment.mockReset().mockResolvedValue({ data: {} })
    comments.mockReset().mockResolvedValue({ data: AFTER_PROMOTION })
  })

  it('перечитывает ветку с сервера, а не вычёркивает строку локально', async () => {
    const w = await open()
    await confirmDelete(w, ROOT.id)

    expect(removeComment).toHaveBeenCalledWith(ROOT.id)
    expect(comments).toHaveBeenCalledWith('t-1')
    const sent = w.emitted('update:comments').at(-1)[0]
    expect(sent).toEqual(AFTER_PROMOTION)
    w.unmount()
  })

  it('сохраняет ветку целой: один корень и один ответ под ним', async () => {
    const w = await open()
    await confirmDelete(w, ROOT.id)

    // Именно это ломал локальный фильтр: у ответов оставался parent_id на
    // удалённый корень, и groupThreads выдавал два самостоятельных корня.
    const threads = groupThreads(w.emitted('update:comments').at(-1)[0])
    expect(threads).toHaveLength(1)
    expect(threads[0].root.id).toBe(R1.id)
    expect(threads[0].replies.map((r) => r.id)).toEqual([R2.id])
    w.unmount()
  })

  it('перечитка не удалась — строка всё равно уходит из списка', async () => {
    comments.mockRejectedValue(new Error('сеть'))
    const w = await open()
    await confirmDelete(w, ROOT.id)

    // Удаление-то прошло; показывать исчезнувший комментарий нельзя, но и
    // ругаться на пользователя не за что.
    expect(
      w
        .emitted('update:comments')
        .at(-1)[0]
        .map((c) => c.id),
    ).toEqual([R1.id, R2.id])
    expect(errors).toEqual([])
    w.unmount()
  })

  it('удаление не прошло — список не трогаем и говорим об этом', async () => {
    removeComment.mockRejectedValue(new Error('нельзя'))
    const w = await open()
    await confirmDelete(w, ROOT.id)

    expect(comments).not.toHaveBeenCalled()
    expect(w.emitted('update:comments')).toBeUndefined()
    expect(errors).toEqual(['нельзя'])
    w.unmount()
  })
})
