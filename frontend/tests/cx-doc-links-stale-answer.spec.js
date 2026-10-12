import { describe, it, expect, vi, beforeEach } from 'vitest'

// The panel's two lists arrive over the network; every test here drives the
// answers by hand so a *later* request can be made to answer *first*.
vi.mock('@/api', () => ({
  documents: {
    links: vi.fn(),
    approvals: vi.fn(),
    linkTask: vi.fn().mockResolvedValue({ data: {} }),
    unlinkTask: vi.fn().mockResolvedValue({ data: {} }),
    createApproval: vi.fn().mockResolvedValue({ data: { id: 'a-new' } }),
    decideApproval: vi.fn().mockResolvedValue({ data: {} }),
    cancelApproval: vi.fn().mockResolvedValue({ data: {} }),
  },
}))

const { documents: docsApi } = await import('@/api')
const { useDocLinks } = await import('@/composables/useDocLinks')

const linkRow = (id, title) => ({ id, task_id: `t-${id}`, task_title: title, block_id: '' })
const approvalRow = (id, over = {}) => ({
  id,
  status: 'pending',
  mode: 'sequential',
  title: 'Редакция 1',
  steps: [{ id: `s-${id}`, approver_id: 'u1', position: 0, status: 'pending' }],
  ...over,
})

// A promise whose settlement the test owns.
function deferred() {
  let resolve
  let reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

// Queues one answer pair per document id; `links`/`approvals` hand back the
// deferred belonging to the id they were called with, so the test can settle
// them in any order it likes.
function answers() {
  const byDoc = {}
  const slot = (id) => (byDoc[id] ||= { links: deferred(), approvals: deferred() })
  docsApi.links.mockImplementation((id) => slot(id).links.promise)
  docsApi.approvals.mockImplementation((id) => slot(id).approvals.promise)
  return {
    settle(id, { links = [], approvals = [] } = {}) {
      slot(id).links.resolve({ data: links })
      slot(id).approvals.resolve({ data: approvals })
    },
    fail(id, err) {
      slot(id).links.reject(err)
      slot(id).approvals.reject(err)
    },
  }
}

const tick = () => new Promise((r) => setTimeout(r, 0))

describe('useDocLinks — ответ покинутого документа', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('обычное открытие наполняет панель', async () => {
    const a = answers()
    const dl = useDocLinks()
    const p = dl.open('doc-a')
    a.settle('doc-a', { links: [linkRow('l1', 'Смета')], approvals: [approvalRow('ap1')] })
    await p
    expect(dl.links.value.map((l) => l.id)).toEqual(['l1'])
    expect(dl.approvals.value.map((x) => x.id)).toEqual(['ap1'])
    expect(dl.loading.value).toBe(false)
  })

  it('поздний ответ прежнего документа не подменяет ссылки открытого', async () => {
    const a = answers()
    const dl = useDocLinks()
    dl.open('doc-a')
    await tick()
    dl.open('doc-b')
    await tick()
    // B ответил первым, A — вторым: читатель смотрит на B.
    a.settle('doc-b', { links: [linkRow('lb', 'Задача B')] })
    await tick()
    a.settle('doc-a', { links: [linkRow('la', 'Задача A')] })
    await tick()
    expect(dl.links.value.map((l) => l.task_title)).toEqual(['Задача B'])
  })

  it('поздний ответ прежнего документа не приносит чужой протокол согласования', async () => {
    const a = answers()
    const dl = useDocLinks()
    dl.open('doc-a')
    await tick()
    dl.open('doc-b')
    await tick()
    a.settle('doc-b', { approvals: [] })
    await tick()
    // Протокол документа A: он открыт, значит `state` сказал бы «на согласовании»,
    // а кнопка подписи ушла бы в approvalId чужого документа.
    a.settle('doc-a', { approvals: [approvalRow('ap-a')] })
    await tick()
    expect(dl.approvals.value).toEqual([])
    expect(dl.state.value).toBe(null)
  })

  it('ошибка на покинутом документе не красит открытый', async () => {
    const a = answers()
    const dl = useDocLinks()
    dl.open('doc-a')
    await tick()
    dl.open('doc-b')
    await tick()
    a.settle('doc-b', { links: [linkRow('lb', 'Задача B')] })
    await tick()
    a.fail('doc-a', new Error('сеть моргнула'))
    await tick()
    expect(dl.error.value).toBe('')
    expect(dl.links.value.map((l) => l.id)).toEqual(['lb'])
  })

  it('ответ покинутого документа не гасит спиннер текущей загрузки', async () => {
    const a = answers()
    const dl = useDocLinks()
    dl.open('doc-a')
    await tick()
    dl.open('doc-b')
    await tick()
    a.settle('doc-a', { links: [linkRow('la', 'Задача A')] })
    await tick()
    expect(dl.loading.value).toBe(true)
    a.settle('doc-b', { links: [linkRow('lb', 'Задача B')] })
    await tick()
    expect(dl.loading.value).toBe(false)
  })

  it('уход в сетку: ответ в полёте не возвращает ссылки закрытого документа', async () => {
    const a = answers()
    const dl = useDocLinks()
    dl.open('doc-a')
    await tick()
    dl.close()
    a.settle('doc-a', { links: [linkRow('la', 'Задача A')], approvals: [approvalRow('ap-a')] })
    await tick()
    expect(dl.links.value).toEqual([])
    expect(dl.approvals.value).toEqual([])
    expect(dl.loading.value).toBe(false)
  })

  it('перечитывание после мутации всё ещё работает', async () => {
    const a = answers()
    const dl = useDocLinks()
    const p = dl.open('doc-a')
    a.settle('doc-a', { links: [] })
    await p
    // Вторая пара ответов того же документа.
    const second = deferred()
    docsApi.links.mockImplementation(() => second.promise)
    docsApi.approvals.mockResolvedValue({ data: [] })
    const linking = dl.link({ taskId: 't-9' })
    await tick()
    second.resolve({ data: [linkRow('l9', 'Привязанная')] })
    await linking
    expect(docsApi.linkTask).toHaveBeenCalledWith('doc-a', {
      task_id: 't-9',
      block_id: '',
      quote: '',
    })
    expect(dl.links.value.map((l) => l.id)).toEqual(['l9'])
  })
})
