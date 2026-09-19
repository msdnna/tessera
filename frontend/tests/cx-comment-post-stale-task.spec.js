// Вкладка «Комментарии» и задача, которую успели сменить под ней (#2932).
//
// Модалка задачи одна на всю доску и НЕ перемонтируется — под вкладкой просто
// подменяется taskId (клик по подзадаче, связи, «Родителю» или тост активности
// прямо из открытой модалки). Комментарий об этом есть в самой вкладке
// (resetComposers: «switching to another task … keeps this subtree mounted»), а
// вот запросы этого не учитывали: и адрес запроса, и приёмник ответа читались из
// props.taskId ПОСЛЕ await.
//
// Самое громкое место — офлайн-ретрай отправки. Черновик специально сохраняется и
// ждёт до 17 секунд (2+5+10) — окна больше, чем нужно, чтобы читатель за это время
// открыл другую задачу. Дальше tasksApi.addComment(props.taskId, …) отправляет текст
// НЕ в ту задачу, где его писали: ни ошибки, ни предупреждения, комментарий просто
// оказывается в чужой ветке. Тот же класс — перечитка ветки после отправки/правки/
// удаления: список покинутой задачи уезжает в update:comments открытой, а
// reload-detail перезагружает не ту задачу.
//
// Пинится идиома из соседей по задаче (SearchBar, TaskModal, BoardView): id берётся
// ДО первого await, все запросы адресуются им, а запись в UI — только если задача на
// экране всё ещё та же. Успешный путь проверяется рядом: гвард, роняющий ЛЮБОЙ ответ,
// сам по себе прошёл бы все проверки на устаревание.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { NPopconfirm } from 'naive-ui'

const warnings = []
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({
      error: (m) => warnings.push(m),
      success: vi.fn(),
      info: vi.fn(),
      warning: vi.fn(),
    }),
  }
})

const addComment = vi.fn()
const comments = vi.fn()
const updateComment = vi.fn()
const removeComment = vi.fn()
const previewCommands = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    addComment: (...a) => addComment(...a),
    comments: (...a) => comments(...a),
    updateComment: (...a) => updateComment(...a),
    removeComment: (...a) => removeComment(...a),
    previewCommands: (...a) => previewCommands(...a),
  },
}))

const ME = 'u-me'
const A = 't-A' // задача, в которой пишут
const B = 't-B' // задача, которую открыли, пока запрос в полёте

const mine = (id, body) => ({
  id,
  author_id: ME,
  author_name: 'Я',
  body,
  created_at: '2026-09-19T10:00:00Z',
  parent_id: null,
})
const A_THREAD = [mine('c-a1', 'ветка задачи A')]
const B_THREAD = [mine('c-b1', 'ветка задачи B')]

// Редактор-заглушка: отдаёт v-model и @submit, плюс методы, которые вкладка зовёт
// по ref (getMentions/clear/autoGrow) — без них отправка падает в undefined.
const EditorStub = {
  props: ['modelValue'],
  emits: ['update:modelValue', 'submit', 'cancel'],
  methods: {
    getMentions() {
      return []
    },
    clear() {
      this.$emit('update:modelValue', '')
    },
    autoGrow() {},
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

async function open(taskId = A, list = A_THREAD) {
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser({ id: ME, email: 'me@example.com', name: 'Я' })
  const TaskCommentsTab = (await import('@/components/task/TaskCommentsTab.vue')).default
  const w = mount(TaskCommentsTab, {
    props: { taskId, comments: list, hydrated: true },
    global: { stubs },
  })
  await tick(w)
  return w
}

// Главный композер — последний редактор на странице (ветка рисует свои выше).
const composer = (w) => w.findAllComponents(EditorStub).at(-1)
async function type(w, text) {
  composer(w).vm.$emit('update:modelValue', text)
  await tick(w)
}
async function send(w) {
  composer(w).vm.$emit('submit')
  await tick(w)
}
// Читатель ушёл в другую задачу: модалка подменила taskId и список под вкладкой.
async function switchTo(w, id, list) {
  await w.setProps({ taskId: id, comments: list })
  await tick(w)
}

// Обещание, чей исход спек контролирует сам.
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
const offline = () => Object.assign(new Error('сеть недоступна'), { offline: true })

describe('вкладка «Комментарии»: задачу сменили, пока запрос в полёте (#2932)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    warnings.length = 0
    addComment.mockReset().mockResolvedValue({ data: {} })
    comments.mockReset().mockResolvedValue({ data: A_THREAD })
    updateComment.mockReset().mockResolvedValue({ data: {} })
    removeComment.mockReset().mockResolvedValue({ data: {} })
    previewCommands.mockReset().mockResolvedValue({ data: { commands: [], custom: [] } })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('офлайн-ретрай доносит комментарий до ТОЙ задачи, где его писали', async () => {
    vi.useFakeTimers()
    const w = await open()
    addComment.mockRejectedValueOnce(offline())
    await type(w, 'мысль про задачу A')
    await send(w)
    expect(addComment).toHaveBeenNthCalledWith(
      1,
      A,
      'мысль про задачу A',
      [],
      null,
      expect.any(String),
    )

    // Ретрай ещё ждёт своей очереди — читатель уходит в другую задачу.
    await switchTo(w, B, B_THREAD)
    await vi.advanceTimersByTimeAsync(2000)
    await tick(w)

    expect(addComment).toHaveBeenCalledTimes(2)
    // Вот это и уносило текст в чужую ветку: второй заход брал уже новый taskId.
    expect(addComment.mock.calls[1][0]).toBe(A)
    w.unmount()
  })

  it('ветка покинутой задачи не подменяет открытую', async () => {
    const landed = deferred()
    comments.mockReturnValue(landed.promise)
    const w = await open()
    await type(w, 'мысль про задачу A')
    await send(w)

    await switchTo(w, B, B_THREAD)
    landed.resolve({ data: A_THREAD })
    await tick(w)

    // Ответ на запрос покинутой задачи — не список открытой.
    const sent = (w.emitted('update:comments') || []).map((e) => e[0])
    expect(sent).not.toContainEqual(A_THREAD)
    w.unmount()
  })

  it('перечитка адресована покинутой задаче, а не открытой', async () => {
    const posted = deferred()
    addComment.mockReturnValue(posted.promise)
    const w = await open()
    await type(w, 'мысль про задачу A')
    await send(w)

    await switchTo(w, B, B_THREAD)
    posted.resolve({ data: {} })
    await tick(w)

    expect(comments).toHaveBeenCalledWith(A)
    w.unmount()
  })

  it('команды применились в покинутой задаче — открытую не перезагружаем', async () => {
    const posted = deferred()
    addComment.mockReturnValue(posted.promise)
    const w = await open()
    await type(w, '/due сегодня')
    await send(w)

    await switchTo(w, B, B_THREAD)
    posted.resolve({ data: { command_summary: { applied: [{ key: 'due', summary: 'срок' }] } } })
    await tick(w)

    // reload-detail перезагрузил бы уже ДРУГУЮ задачу, которой команда не касалась.
    expect(w.emitted('reload-detail')).toBeFalsy()
    w.unmount()
  })

  it('ответ в ветке уходит в свою задачу и не подменяет список открытой', async () => {
    const posted = deferred()
    addComment.mockReturnValue(posted.promise)
    const w = await open()
    // Открыть композер ответа под корнем ветки.
    await w.find('.c-link').trigger('click')
    await tick(w)
    const reply = w.findAllComponents(EditorStub).find((e) => e !== composer(w))
    reply.vm.$emit('update:modelValue', 'ответ в ветке A')
    await tick(w)
    reply.vm.$emit('submit')
    await tick(w)
    expect(addComment).toHaveBeenCalledWith(A, 'ответ в ветке A', [], 'c-a1', expect.any(String))

    await switchTo(w, B, B_THREAD)
    posted.resolve({ data: {} })
    await tick(w)

    expect(comments).toHaveBeenCalledWith(A)
    const sent = (w.emitted('update:comments') || []).map((e) => e[0])
    expect(sent).not.toContainEqual(A_THREAD)
    w.unmount()
  })

  it('правка комментария покинутой задачи не подменяет список открытой', async () => {
    const saved = deferred()
    updateComment.mockReturnValue(saved.promise)
    const w = await open()
    await w.find('.c-act').trigger('click') // ✎ у своего комментария
    await tick(w)
    const editor = w.findAllComponents(EditorStub)[0]
    editor.vm.$emit('update:modelValue', 'исправленный текст')
    await tick(w)
    editor.vm.$emit('submit')
    await tick(w)

    await switchTo(w, B, B_THREAD)
    saved.resolve({ data: {} })
    await tick(w)

    expect(comments).toHaveBeenCalledWith(A)
    const sent = (w.emitted('update:comments') || []).map((e) => e[0])
    expect(sent).not.toContainEqual(A_THREAD)
    w.unmount()
  })

  it('удаление в покинутой задаче не вычёркивает строку из открытой', async () => {
    const gone = deferred()
    removeComment.mockReturnValue(gone.promise)
    const w = await open()
    const pc = w
      .findAllComponents(NPopconfirm)
      .find((p) => p.element?.closest?.(`[data-comment-id="c-a1"]`))
    pc.vm.$emit('positive-click')
    await tick(w)

    await switchTo(w, B, B_THREAD)
    // Перечитка тоже падает — это путь локального фолбэка, который фильтровал
    // props.comments, то есть уже список ОТКРЫТОЙ задачи.
    comments.mockRejectedValue(new Error('сеть'))
    gone.resolve({ data: {} })
    await tick(w)

    expect(w.emitted('update:comments')).toBeFalsy()
    w.unmount()
  })

  it('подсказка по командам покинутой задачи не рисуется под открытой', async () => {
    vi.useFakeTimers()
    const hint = deferred()
    previewCommands.mockReturnValue(hint.promise)
    const w = await open()
    await type(w, '/due завтра')
    await vi.advanceTimersByTimeAsync(400)
    await tick(w)
    expect(previewCommands.mock.calls[0][0]).toBe(A)

    await switchTo(w, B, B_THREAD)
    hint.resolve({ data: { commands: [{ key: 'due', summary: 'срок: завтра' }], custom: [] } })
    await tick(w)

    expect(w.find('.cmd-preview').exists()).toBe(false)
    w.unmount()
  })

  it('обычная отправка по-прежнему доходит: ветка обновилась, черновик очищен', async () => {
    const w = await open()
    await type(w, 'просто комментарий')
    await send(w)

    expect(addComment).toHaveBeenCalledWith(A, 'просто комментарий', [], null, expect.any(String))
    expect(comments).toHaveBeenCalledWith(A)
    expect(w.emitted('update:comments').at(-1)[0]).toEqual(A_THREAD)
    expect(w.emitted('changed')).toBeTruthy()
    expect(composer(w).props('modelValue')).toBe('')
    w.unmount()
  })
})
