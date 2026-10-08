// Модалка задачи, открытая из АРХИВА (`:readonly="archivedMode"` в KanbanBoard),
// уважала режим только наполовину. Описание, подзадачи и композер комментариев
// прятали свои кнопки, а четыре вкладки — нет:
//
//   • «Комментарии» — ✎/✕ у СВОЕГО комментария рисовались без оглядки на
//     readonly. ✕ действительно удалял (deleteComment гварда не имел), а ✎
//     открывал редактор, у которого «Сохранить» молча уходил в
//     `if (props.readonly) return` — набранный текст исчезал без единого слова.
//     Чекбоксы в своём комментарии тоже оставались кликабельными и сохранялись.
//   • «Связи», «Файлы», «Документы» — пропса `readonly` у них не было вовсе, и
//     модалка его туда не передавала: в архиве живьём снимались связи, удалялись
//     и заливались вложения, отцеплялись документы.
//
// Гарантия, которую пинует этот файл: в readonly ни одна вкладка не показывает
// разрушительной кнопки и ни одна не ходит в API на запись. Остальное (открыть
// связанную задачу, скачать файл, открыть документ) обязано работать — архив
// читают, а не запирают (#2932).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import ArmButton from '@/components/kit/ArmButton.vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const api = {
  removeComment: vi.fn(),
  updateComment: vi.fn(),
  addComment: vi.fn(),
  comments: vi.fn(() => Promise.resolve({ data: [] })),
  removeRelation: vi.fn(),
  addRelation: vi.fn(),
  relations: vi.fn(() => Promise.resolve({ data: [] })),
  removeAttachment: vi.fn(),
  uploadAttachment: vi.fn(),
  attachments: vi.fn(() => Promise.resolve({ data: [] })),
  previewCommands: vi.fn(() => Promise.resolve({ data: { commands: [], custom: [] } })),
}
const unlinkTask = vi.fn()
vi.mock('@/api', () => ({
  tasks: new Proxy(
    {},
    {
      get:
        (_, k) =>
        (...a) =>
          api[k]?.(...a),
    },
  ),
  workspaces: { tasks: vi.fn(() => Promise.resolve({ data: [] })) },
  documents: { unlinkTask: (...a) => unlinkTask(...a) },
}))

const ME = 'u-me'
const MINE = { id: 'c-mine', author_id: ME, author_name: 'Я', body: '- [ ] пункт', created_at: '2026-09-19T10:00:00Z', parent_id: null } // prettier-ignore
const REPLY = { id: 'c-reply', author_id: ME, author_name: 'Я', body: 'ответ', created_at: '2026-09-19T10:05:00Z', parent_id: 'c-mine' } // prettier-ignore
const THEIRS = { id: 'c-th', author_id: 'u-a', author_name: 'Аня', body: 'чужое', created_at: '2026-09-19T10:09:00Z', parent_id: null } // prettier-ignore

const RELATION = { id: 'rel-1', kind: 'blocks', task_id: 't-1', number: 42, title: 'другая' }
const FILE = { id: 'att-1', filename: 'план.pdf', size: 1024 }
const DOC_LINK = { id: 'dl-1', document_id: 'd-1', document_title: 'ТЗ' }

const stubs = {
  MarkdownEditor: true,
  RichContent: true,
  UserAvatar: true,
  TesseraSpinner: true,
  EmptyState: true,
  teleport: true,
}

async function openComments(readonly, list = [MINE, REPLY, THEIRS]) {
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser({ id: ME, email: 'me@example.com', name: 'Я' })
  const Tab = (await import('@/components/task/TaskCommentsTab.vue')).default
  return mount(Tab, {
    props: { taskId: 't-1', comments: list, hydrated: true, readonly },
    global: { stubs },
  })
}
const openRelations = async (readonly) =>
  mount((await import('@/components/task/TaskRelationsTab.vue')).default, {
    props: { taskId: 't-1', wsId: 'w-1', relations: [RELATION], readonly },
    global: { stubs },
  })
const openFiles = async (readonly) =>
  mount((await import('@/components/task/TaskFilesTab.vue')).default, {
    props: { taskId: 't-1', attachments: [FILE], readonly },
    global: { stubs },
  })
const openDocs = async (readonly) =>
  mount((await import('@/components/task/TaskDocumentsTab.vue')).default, {
    props: { taskId: 't-1', links: [DOC_LINK], readonly },
    global: { stubs },
  })

// Необратимое удаление — двухшаговая кнопка (v2, #3113): первое нажатие лишь
// взводит её, вызов идёт на `confirm`. Здесь — сразу второй шаг.
const fireArms = async (w) => {
  const list = w.findAllComponents(ArmButton)
  list.forEach((p) => p.vm.$emit('confirm'))
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
  return list.length
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((f) => f.mockClear())
  unlinkTask.mockClear().mockResolvedValue({ data: {} })
  api.removeComment.mockResolvedValue({ data: {} })
  api.removeRelation.mockResolvedValue({ data: {} })
  api.removeAttachment.mockResolvedValue({ data: {} })
  api.updateComment.mockResolvedValue({ data: {} })
})

describe('вкладка «Комментарии» в режиме только для чтения', () => {
  it('не показывает ✎/✕ ни у корня, ни у своего ответа', async () => {
    const w = await openComments(true)
    expect(w.findAll('.c-acts')).toHaveLength(0)
    w.unmount()
  })

  it('удаление не уходит в API, даже если кнопку всё-таки нажали', async () => {
    const w = await openComments(true)
    await fireArms(w)
    expect(api.removeComment).not.toHaveBeenCalled()
    expect(w.emitted('update:comments')).toBeUndefined()
    w.unmount()
  })

  it('чекбокс в своём комментарии не сохраняется', async () => {
    const w = await openComments(true)
    // Через сам компонент: стаб RichContent событие не родит, а проверить надо
    // именно гвард, а не то, что рисуется.
    await w.vm.onCommentCheck({ ...MINE }, 0)
    expect(api.updateComment).not.toHaveBeenCalled()
    w.unmount()
  })

  // Заглушка RichContent печатает пропс атрибутом, так что смотрим на значение:
  // `interactive` — это и есть разрешение кликать по чекбоксам в тексте.
  const interactiveFlags = (w) => w.findAll('.c-text').map((n) => n.attributes('interactive'))

  it('разметка комментариев не интерактивна', async () => {
    const w = await openComments(true)
    expect(interactiveFlags(w)).toEqual(['false', 'false', 'false'])
    w.unmount()
  })

  it('контроль: в обычном режиме свои комментарии интерактивны, чужой — нет', async () => {
    const w = await openComments(false)
    expect(interactiveFlags(w)).toEqual(['true', 'true', 'false'])
    w.unmount()
  })

  // Контроль: в обычном режиме всё это на месте — иначе «зелено» доказывало бы
  // лишь то, что вкладка ничего не умеет.
  it('контроль: в обычном режиме ✎/✕ есть и удаление доходит до API', async () => {
    api.comments.mockResolvedValue({ data: [THEIRS] })
    const w = await openComments(false)
    expect(w.findAll('.c-acts').length).toBe(2) // корень и свой ответ
    const pc = w
      .findAllComponents(ArmButton)
      .find((p) => p.element?.closest?.(`[data-comment-id="${MINE.id}"]`))
    pc.vm.$emit('confirm')
    await new Promise((r) => setTimeout(r, 0))
    expect(api.removeComment).toHaveBeenCalledWith(MINE.id)
    w.unmount()
  })

  it('контроль: в обычном режиме чекбокс своего комментария сохраняется', async () => {
    const w = await openComments(false)
    await w.vm.onCommentCheck({ ...MINE }, 0)
    expect(api.updateComment).toHaveBeenCalled()
    w.unmount()
  })

  it('контроль: чужой комментарий и без архива без кнопок', async () => {
    const w = await openComments(false, [THEIRS])
    expect(w.findAll('.c-acts')).toHaveLength(0)
    w.unmount()
  })
})

describe('вкладка «Связи» в режиме только для чтения', () => {
  it('не показывает ни «✕» у связи, ни формы добавления', async () => {
    const w = await openRelations(true)
    expect(w.findAll('[data-testid="rel-unlink"]')).toHaveLength(0)
    expect(w.find('[data-testid="rel-form-card"]').exists()).toBe(false)
    expect(w.find('.rel-open').exists()).toBe(false)
    w.unmount()
  })

  it('снятие связи не уходит в API', async () => {
    const w = await openRelations(true)
    await fireArms(w)
    await w.vm.removeRelation(RELATION)
    expect(api.removeRelation).not.toHaveBeenCalled()
    w.unmount()
  })

  it('открыть связанную задачу по-прежнему можно', async () => {
    const w = await openRelations(true)
    await w.find('.rel-link').trigger('click')
    expect(w.emitted('open-related')).toBeTruthy()
    w.unmount()
  })

  it('контроль: в обычном режиме «✕» и форма на месте', async () => {
    const w = await openRelations(false)
    expect(w.findAll('[data-testid="rel-unlink"]').length).toBeGreaterThan(0)
    // v2 (#2958): the form opens from «Связать»; in read-only that button is gone.
    await w.find('.rel-open').trigger('click')
    await w.vm.$nextTick()
    expect(w.find('[data-testid="rel-form-card"]').exists()).toBe(true)
    // Своя связь снимается сразу (и предлагает отмену) — запрос уходит.
    await w.find('[data-testid="rel-unlink"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    expect(api.removeRelation).toHaveBeenCalledWith(RELATION.id)
    w.unmount()
  })
})

describe('вкладка «Файлы» в режиме только для чтения', () => {
  it('не показывает ни кнопки загрузки, ни «удалить»', async () => {
    const w = await openFiles(true)
    expect(w.findAllComponents(ArmButton)).toHaveLength(0)
    expect(w.find('input[type="file"]').exists()).toBe(false)
    w.unmount()
  })

  it('удаление вложения не уходит в API', async () => {
    const w = await openFiles(true)
    await fireArms(w)
    expect(api.removeAttachment).not.toHaveBeenCalled()
    w.unmount()
  })

  it('контроль: в обычном режиме и выбор файла, и «удалить» на месте', async () => {
    const w = await openFiles(false)
    expect(w.findAllComponents(ArmButton).length).toBeGreaterThan(0)
    expect(w.find('input[type="file"]').exists()).toBe(true)
    w.unmount()
  })
})

describe('вкладка «Документы» в режиме только для чтения', () => {
  it('не показывает «отцепить»', async () => {
    const w = await openDocs(true)
    expect(w.findAllComponents(ArmButton)).toHaveLength(0)
    w.unmount()
  })

  it('отцепление не уходит в API', async () => {
    const w = await openDocs(true)
    await fireArms(w)
    expect(unlinkTask).not.toHaveBeenCalled()
    expect(w.emitted('update:links')).toBeUndefined()
    w.unmount()
  })

  it('открыть документ по-прежнему можно', async () => {
    const w = await openDocs(true)
    await w.find('[data-testid="task-doc-link"]').trigger('click')
    expect(w.emitted('open-document')).toBeTruthy()
    w.unmount()
  })

  it('контроль: в обычном режиме «отцепить» есть и доходит до API', async () => {
    const w = await openDocs(false)
    expect(await fireArms(w)).toBe(1)
    expect(unlinkTask).toHaveBeenCalledWith(DOC_LINK.id)
    w.unmount()
  })
})
