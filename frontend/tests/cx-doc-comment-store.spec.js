import { describe, it, expect, vi, beforeEach } from 'vitest'

// The composable talks to the API and nothing else, so the client is the only
// thing worth faking here.
const api = {
  comments: vi.fn(),
  addComment: vi.fn(),
  updateComment: vi.fn(),
  resolveComment: vi.fn(),
  removeComment: vi.fn(),
}
vi.mock('@/api', () => ({ documents: api }))

const { useDocComments } = await import('@/composables/useDocComments')

function row(id, over = {}) {
  return {
    id,
    block_id: '',
    parent_id: null,
    body: `тело ${id}`,
    resolved_at: null,
    created_at: '2026-08-15T10:00:00Z',
    ...over,
  }
}

const docJSON = {
  type: 'doc',
  content: [{ type: 'paragraph', attrs: { id: 'b1' }, content: [{ type: 'text', text: 'Пункт' }] }],
}

describe('useDocComments', () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset())
    api.comments.mockResolvedValue({ data: [] })
    api.addComment.mockResolvedValue({ data: {} })
    api.resolveComment.mockResolvedValue({ data: {} })
    api.removeComment.mockResolvedValue({})
    api.updateComment.mockResolvedValue({ data: {} })
  })

  it('loads the threads of the document it is pointed at', async () => {
    api.comments.mockResolvedValue({ data: [row('r1', { block_id: 'b1' })] })
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    expect(api.comments).toHaveBeenCalledWith('doc-1')
    expect(c.threads.value).toHaveLength(1)
    expect(c.groups.value.anchored).toHaveLength(1)
  })

  it('refetches after every write rather than splicing the local list', async () => {
    // The same document is open for several people at once, so a locally patched
    // list is stale the moment a colleague replies.
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    api.comments.mockClear()

    await c.add({ blockId: 'b1', body: 'нужен срок', quote: 'Пункт' })
    expect(api.addComment).toHaveBeenCalledWith('doc-1', {
      block_id: 'b1',
      body: 'нужен срок',
      quote: 'Пункт',
    })
    expect(api.comments).toHaveBeenCalledTimes(1)

    await c.reply('r1', 'ответ')
    expect(api.addComment).toHaveBeenLastCalledWith('doc-1', { parent_id: 'r1', body: 'ответ' })
    expect(api.comments).toHaveBeenCalledTimes(2)
  })

  it('refuses to send an empty comment', async () => {
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    expect(await c.add({ blockId: 'b1', body: '   ' })).toBe(false)
    expect(await c.reply('r1', '')).toBe(false)
    expect(api.addComment).not.toHaveBeenCalled()
  })

  it('marks a thread detached once its block leaves the document', async () => {
    api.comments.mockResolvedValue({ data: [row('r1', { block_id: 'b1' })] })
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    expect(c.groups.value.anchored).toHaveLength(1)

    // The user deleted the paragraph; the panel has to notice without a reload.
    c.setDoc({ type: 'doc', content: [] })
    expect(c.groups.value.anchored).toHaveLength(0)
    expect(c.groups.value.detached).toHaveLength(1)
  })

  it('paints only unresolved threads in the margin', async () => {
    api.comments.mockResolvedValue({
      data: [
        row('r1', { block_id: 'b1' }),
        row('r2', { block_id: 'b1', resolved_at: '2026-08-15T12:00:00Z' }),
      ],
    })
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    expect(c.openCounts.value).toEqual([{ block_id: 'b1', count: 1 }])
    expect(c.openCount.value).toBe(1)
  })

  it('reports a failed load instead of throwing at the view', async () => {
    api.comments.mockRejectedValue(new Error('сеть недоступна'))
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    expect(c.error.value).toBe('сеть недоступна')
    expect(c.loading.value).toBe(false)
  })

  it('drops everything on close, so the next document starts clean', async () => {
    api.comments.mockResolvedValue({ data: [row('r1', { block_id: 'b1' })] })
    const c = useDocComments()
    await c.open('doc-1', docJSON)
    c.close()
    expect(c.threads.value).toEqual([])
    api.comments.mockClear()
    await c.load()
    expect(api.comments).not.toHaveBeenCalled()
  })
})
