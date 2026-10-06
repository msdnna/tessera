// Вкладки «Файлы», «Связи» и «Документы» и задача, которую успели сменить под
// ними (#2932).
//
// Та же конструкция, что и у вкладки комментариев: модалка задачи одна на всю
// доску и НЕ перемонтируется — под вкладкой просто подменяется taskId (клик по
// подзадаче, по связи, по «Родителю» прямо из открытой модалки). Список при этом
// принадлежит модалке (его счётчик стоит на ярлыке вкладки), и вкладка публикует
// его обратно через update:attachments / update:relations / update:links.
//
// Ни один из этих эмитов не спрашивал, та ли задача ещё на экране, и адрес
// перечитки тоже читался из props.taskId ПОСЛЕ await. Громче всего — загрузка
// файла: это самая долгая операция модалки (до 25 МБ по сети), и окна хватает с
// запасом. Файл уходит в ту задачу, где его выбрали, а перечитка списка — уже в
// другую: загруженного файла не видно нигде, и читатель грузит его второй раз.
// Если же смена попадает между двумя запросами, во вкладку ОТКРЫТОЙ задачи
// приезжает список ПОКИНУТОЙ — и ✕ на такой строке удаляет чужое вложение по id,
// без единого вопроса.
//
// Пинится идиома соседей (TaskCommentsTab, SearchBar, BoardView): id берётся ДО
// первого await, все запросы адресуются им, а запись в UI — только если задача на
// экране всё ещё та же. Рядом проверяется успешный путь: гвард, роняющий ЛЮБОЙ
// ответ, сам по себе прошёл бы все проверки на устаревание.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({
      error: vi.fn(),
      success: vi.fn(),
      info: vi.fn(),
      warning: vi.fn(),
    }),
  }
})

const uploadAttachment = vi.fn()
const attachments = vi.fn()
const removeAttachment = vi.fn()
const addRelation = vi.fn()
const relations = vi.fn()
const removeRelation = vi.fn()
const unlinkTask = vi.fn()
const wsTasks = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    uploadAttachment: (...a) => uploadAttachment(...a),
    attachments: (...a) => attachments(...a),
    removeAttachment: (...a) => removeAttachment(...a),
    addRelation: (...a) => addRelation(...a),
    relations: (...a) => relations(...a),
    removeRelation: (...a) => removeRelation(...a),
  },
  workspaces: { tasks: (...a) => wsTasks(...a) },
  documents: { unlinkTask: (...a) => unlinkTask(...a) },
}))

const A = 't-A' // задача, в которой начали операцию
const B = 't-B' // задача, которую открыли, пока запрос в полёте

const A_FILES = [{ id: 'f-a1', filename: 'договор-A.pdf', size: 1024 }]
const B_FILES = [{ id: 'f-b1', filename: 'смета-B.xlsx', size: 2048 }]
const A_RELS = [{ id: 'r-a1', kind: 'relates', related_number: 11, related_title: 'связь A' }]
const B_RELS = [{ id: 'r-b1', kind: 'relates', related_number: 22, related_title: 'связь B' }]
const A_LINKS = [{ id: 'l-a1', document_id: 'd-a', document_title: 'документ A' }]
const B_LINKS = [{ id: 'l-b1', document_id: 'd-b', document_title: 'документ B' }]

const stubs = { EmptyState: true, teleport: true }

const tick = async (w, n = 6) => {
  for (let i = 0; i < n; i++) await w.vm.$nextTick()
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

// Читатель ушёл в другую задачу: модалка подменила taskId и список под вкладкой.
const switchTo = async (w, props) => {
  await w.setProps(props)
  await tick(w)
}

const emitsOf = (w, event) => (w.emitted(event) || []).map((e) => e[0])

beforeEach(() => {
  uploadAttachment.mockReset().mockResolvedValue({ data: {} })
  attachments.mockReset().mockResolvedValue({ data: A_FILES })
  removeAttachment.mockReset().mockResolvedValue({ data: {} })
  addRelation.mockReset().mockResolvedValue({ data: {} })
  relations.mockReset().mockResolvedValue({ data: A_RELS })
  removeRelation.mockReset().mockResolvedValue({ data: {} })
  unlinkTask.mockReset().mockResolvedValue({ data: {} })
  wsTasks.mockReset().mockResolvedValue({ data: [] })
})

describe('вкладка «Файлы»: задачу сменили, пока файл грузится (#2932)', () => {
  async function openFiles(taskId = A, list = A_FILES) {
    const TaskFilesTab = (await import('@/components/task/TaskFilesTab.vue')).default
    const w = mount(TaskFilesTab, { props: { taskId, attachments: list }, global: { stubs } })
    await tick(w)
    return w
  }
  // Выбор файла в скрытом <input type="file">.
  async function choose(w, name = 'акт.pdf') {
    const file = new File(['x'], name, { type: 'application/pdf' })
    const input = w.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await tick(w)
  }

  it('перечитка после загрузки адресована ТОЙ задаче, куда ушёл файл', async () => {
    const sent = deferred()
    uploadAttachment.mockReturnValue(sent.promise)
    const w = await openFiles()
    await choose(w)
    expect(uploadAttachment).toHaveBeenCalledWith(A, expect.any(FormData))

    // Файл ещё идёт по сети — читатель открывает другую задачу.
    await switchTo(w, { taskId: B, attachments: B_FILES })
    sent.resolve({ data: {} })
    await tick(w)

    // Вот из-за этого загруженный файл пропадал: список перечитывался у B.
    expect(attachments).toHaveBeenCalledWith(A)
    w.unmount()
  })

  it('список вложений покинутой задачи не уезжает в открытую', async () => {
    const landed = deferred()
    attachments.mockReturnValue(landed.promise)
    const w = await openFiles()
    await choose(w)
    await switchTo(w, { taskId: B, attachments: B_FILES })
    landed.resolve({ data: A_FILES })
    await tick(w)

    // Иначе во вкладке задачи B видны файлы A, и ✕ там удаляет файл A по id.
    expect(emitsOf(w, 'update:attachments')).not.toContainEqual(A_FILES)
    w.unmount()
  })

  it('удаление вложения не переписывает список уже другой задачи', async () => {
    const gone = deferred()
    removeAttachment.mockReturnValue(gone.promise)
    const w = await openFiles()
    w.vm.deleteAttachment?.('f-a1')
    await tick(w)
    await switchTo(w, { taskId: B, attachments: B_FILES })
    gone.resolve({ data: {} })
    await tick(w)

    expect(w.emitted('update:attachments')).toBeFalsy()
    w.unmount()
  })

  it('обычная загрузка доходит до списка (контроль)', async () => {
    const w = await openFiles()
    await choose(w)
    expect(uploadAttachment).toHaveBeenCalledWith(A, expect.any(FormData))
    expect(attachments).toHaveBeenCalledWith(A)
    expect(emitsOf(w, 'update:attachments')).toContainEqual(A_FILES)
    w.unmount()
  })

  it('обычное удаление убирает строку из списка (контроль)', async () => {
    const w = await openFiles()
    w.vm.deleteAttachment?.('f-a1')
    await tick(w)
    expect(emitsOf(w, 'update:attachments')).toContainEqual([])
    w.unmount()
  })
})

describe('вкладка «Связи»: задачу сменили, пока связь создаётся (#2932)', () => {
  async function openRelations(taskId = A, list = A_RELS) {
    const TaskRelationsTab = (await import('@/components/task/TaskRelationsTab.vue')).default
    const w = mount(TaskRelationsTab, {
      props: { taskId, relations: list, wsId: 'ws-1' },
      global: { stubs },
    })
    await tick(w)
    return w
  }
  async function link(w, number = 42) {
    w.vm.relNumber = number
    await tick(w)
    w.vm.addRelation()
    await tick(w)
  }

  it('перечитка связей адресована ТОЙ задаче, где их связали', async () => {
    const sent = deferred()
    addRelation.mockReturnValue(sent.promise)
    const w = await openRelations()
    await link(w)
    expect(addRelation).toHaveBeenCalledWith(A, 42, 'relates')

    await switchTo(w, { taskId: B, relations: B_RELS })
    sent.resolve({ data: {} })
    await tick(w)

    expect(relations).toHaveBeenCalledWith(A)
    w.unmount()
  })

  it('список связей покинутой задачи не уезжает в открытую', async () => {
    const landed = deferred()
    relations.mockReturnValue(landed.promise)
    const w = await openRelations()
    await link(w)
    await switchTo(w, { taskId: B, relations: B_RELS })
    landed.resolve({ data: A_RELS })
    await tick(w)

    expect(emitsOf(w, 'update:relations')).not.toContainEqual(A_RELS)
    w.unmount()
  })

  it('снятие связи не переписывает список уже другой задачи', async () => {
    const gone = deferred()
    removeRelation.mockReturnValue(gone.promise)
    const w = await openRelations()
    w.vm.removeRelation?.('r-a1')
    await tick(w)
    await switchTo(w, { taskId: B, relations: B_RELS })
    gone.resolve({ data: {} })
    await tick(w)

    expect(w.emitted('update:relations')).toBeFalsy()
    w.unmount()
  })

  it('обычная связь доходит до списка (контроль)', async () => {
    const w = await openRelations()
    await link(w)
    expect(relations).toHaveBeenCalledWith(A)
    expect(emitsOf(w, 'update:relations')).toContainEqual(A_RELS)
    w.unmount()
  })
})

describe('вкладка «Документы»: задачу сменили, пока связь снимается (#2932)', () => {
  async function openDocs(taskId = A, list = A_LINKS) {
    const TaskDocumentsTab = (await import('@/components/task/TaskDocumentsTab.vue')).default
    const w = mount(TaskDocumentsTab, { props: { taskId, links: list }, global: { stubs } })
    await tick(w)
    return w
  }

  it('снятие связи с документом не переписывает список уже другой задачи', async () => {
    const gone = deferred()
    unlinkTask.mockReturnValue(gone.promise)
    const w = await openDocs()
    w.vm.unlink?.('l-a1')
    await tick(w)
    await switchTo(w, { taskId: B, links: B_LINKS })
    gone.resolve({ data: {} })
    await tick(w)

    expect(w.emitted('update:links')).toBeFalsy()
    w.unmount()
  })

  it('обычное снятие связи убирает строку (контроль)', async () => {
    const w = await openDocs()
    w.vm.unlink?.('l-a1')
    await tick(w)
    expect(emitsOf(w, 'update:links')).toContainEqual([])
    w.unmount()
  })
})
