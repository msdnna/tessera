import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import axios from 'axios'
import api, { projects, workspaces, boards, setAccessToken } from '@/api'
import { connection } from '@/composables/useConnection'

// #2932 — the in-flight GET coalescing in api/index.js shares one request between
// callers that ask for the same read at the same time. The read that must never
// be shared is the list refetch that follows a write: the board keeps slow
// tags/members/milestones GETs in flight while the user creates a tag in the
// modal on top of it, and joining one of those hands back a list without the new
// row. These cases pin both halves — still coalesce ordinary concurrent reads,
// never coalesce across a write.

let instanceAdapter

function ok(config, data) {
  return { data, status: 200, statusText: 'OK', headers: {}, config, request: {} }
}

// A pending GET whose answer we release by hand, so a write can land while it is
// still in flight — the actual shape of the bug.
function deferred() {
  let settle
  const promise = new Promise((resolve) => {
    settle = resolve
  })
  return { promise, settle }
}

// axios runs its request interceptor through a promise chain, so the adapter is
// not reached in the same tick the call is made.
function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0))
}

beforeEach(() => {
  setAccessToken('')
  connection.pending = 0
  connection.slow = false
  connection.offline = false
  instanceAdapter = vi.fn()
  api.defaults.adapter = instanceAdapter
  axios.defaults.adapter = vi.fn()
})
afterEach(() => {
  setAccessToken('')
})

describe('sharedGet coalescing', () => {
  it('shares one request between concurrent identical reads', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [{ id: 't1' }])))
    const [a, b] = await Promise.all([projects.tags('p1'), projects.tags('p1')])
    expect(instanceAdapter).toHaveBeenCalledTimes(1)
    expect(a.data).toEqual([{ id: 't1' }])
    expect(b.data).toEqual([{ id: 't1' }])
  })

  it('gives each caller its own copy, so one cannot mutate the other', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [{ id: 't1' }])))
    const [a, b] = await Promise.all([projects.tags('p1'), projects.tags('p1')])
    a.data.push({ id: 'local' })
    expect(b.data).toEqual([{ id: 't1' }])
  })

  it('does not coalesce reads that are not concurrent', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [])))
    await projects.tags('p1')
    await projects.tags('p1')
    expect(instanceAdapter).toHaveBeenCalledTimes(2)
  })

  it('keeps different keys apart', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [])))
    await Promise.all([projects.tags('p1'), projects.tags('p2')])
    expect(instanceAdapter).toHaveBeenCalledTimes(2)
  })

  it('refetch after a write does not join the GET that was in flight before it', async () => {
    const slow = deferred()
    const calls = []
    instanceAdapter.mockImplementation((config) => {
      calls.push(`${config.method} ${config.url}`)
      if (config.method === 'post') return Promise.resolve(ok(config, { id: 'new' }))
      if (calls.filter((c) => c.startsWith('get')).length === 1) return slow.promise
      return Promise.resolve(ok(config, [{ id: 'old' }, { id: 'new' }]))
    })

    // The board's own tags read, still in flight.
    const boardRead = projects.tags('p1')
    // The user creates a tag in the modal on top of it, then refetches.
    await projects.createTag('p1', { name: 'new' })
    const afterWrite = await projects.tags('p1')

    expect(afterWrite.data).toEqual([{ id: 'old' }, { id: 'new' }])
    expect(calls.filter((c) => c.startsWith('get'))).toHaveLength(2)

    // The board's read still gets its own (older) answer — it was never cancelled.
    slow.settle(ok({ method: 'get', url: '/projects/p1/tags' }, [{ id: 'old' }]))
    expect((await boardRead).data).toEqual([{ id: 'old' }])
  })

  it('treats every write verb as a write, not just POST', async () => {
    for (const write of [
      () => projects.updateTag('t1', { name: 'x' }),
      () => projects.deleteTag('t1'),
      () => projects.setTagPrefixes('p1', ['a']),
    ]) {
      instanceAdapter.mockReset()
      instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [])))
      const inflight = projects.tags('p1')
      await write()
      await projects.tags('p1')
      await inflight
      expect(instanceAdapter.mock.calls.filter((c) => c[0].method === 'get')).toHaveLength(2)
    }
  })

  it('a write to one resource also unblocks the refetch of another', async () => {
    // The generation is global on purpose: a write can change more than the list
    // it was addressed to (adding a member changes the member list; transferring
    // a project changes both workspaces' trees), and guessing which reads it
    // invalidates is how a cache goes subtly wrong.
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [])))
    const inflight = workspaces.members('w1')
    await projects.createTag('p1', { name: 'new' })
    await workspaces.members('w1')
    await inflight
    expect(instanceAdapter.mock.calls.filter((c) => c[0].method === 'get')).toHaveLength(2)
  })

  it('still coalesces reads issued together after the write settles', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, [])))
    await projects.createTag('p1', { name: 'new' })
    await Promise.all([boards.columns('b1'), boards.columns('b1')])
    expect(instanceAdapter.mock.calls.filter((c) => c[0].method === 'get')).toHaveLength(1)
  })

  it('the superseded request settling does not evict the fresh entry', async () => {
    // Two entries share a key while the pre-write request drains. If cleanup
    // deleted by key alone, the older one finishing would drop the newer entry
    // and a third caller would open a third request.
    const slow = deferred()
    let gets = 0
    instanceAdapter.mockImplementation((config) => {
      if (config.method !== 'get') return Promise.resolve(ok(config, {}))
      gets++
      if (gets === 1) return slow.promise
      return new Promise(() => {}) // the fresh read stays in flight
    })

    const stale = projects.tags('p1')
    await projects.createTag('p1', { name: 'new' })
    projects.tags('p1') // opens the fresh entry
    await flush()
    expect(gets).toBe(2)

    slow.settle(ok({ method: 'get', url: '/projects/p1/tags' }, []))
    await stale

    projects.tags('p1') // must join the fresh entry, not open a third request
    await flush()
    expect(gets).toBe(2)
  })
})
