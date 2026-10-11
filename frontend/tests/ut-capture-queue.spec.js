import { describe, it, expect, beforeEach } from 'vitest'
import { enqueue, flush, newClientId, queued } from '@/utils/captureQueue'

const item = (title) => ({ kind: 'note', wsId: 'w1', data: { title, client_id: newClientId() } })

describe('capture queue (#3042)', () => {
  beforeEach(() => localStorage.clear())

  it('keeps captures in order and once per client id', () => {
    const a = item('a')
    enqueue(a)
    enqueue(item('b'))
    enqueue(a)
    expect(queued().map((x) => x.data.title)).toEqual(['b', 'a'])
  })

  it('sends in order, stops when the connection is still gone', async () => {
    enqueue(item('a'))
    enqueue(item('b'))
    enqueue(item('c'))
    const seen = []
    const res = await flush(async (x) => {
      seen.push(x.data.title)
      if (x.data.title === 'b') throw Object.assign(new Error('net'), { offline: true })
      return x.data.title
    })
    expect(seen).toEqual(['a', 'b'])
    expect(res.sent).toHaveLength(1)
    expect(queued().map((x) => x.data.title)).toEqual(['b', 'c'])
  })

  it('drops what the server refuses instead of blocking the queue', async () => {
    enqueue(item('gone'))
    enqueue(item('ok'))
    const res = await flush(async (x) => {
      if (x.data.title === 'gone') throw Object.assign(new Error('404'), { status: 404 })
      return 1
    })
    expect(res.dropped.map((d) => d.item.data.title)).toEqual(['gone'])
    expect(queued()).toEqual([])
  })
})
