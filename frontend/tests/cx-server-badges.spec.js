import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

const { post, get } = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }))
vi.mock('axios', () => ({ default: { post, get } }))

import { pollServer } from '@/composables/useServerBadges'
import { sSetFor, sGetFor } from '@/utils/serverStorage'

// A stand-in for the pinia store: pollServer only ever calls these two.
function fakeStore() {
  return {
    badges: {},
    signedOut: {},
    setBadge(id, n) {
      this.badges[id] = n
    },
    setSignedOut(id, v) {
      this.signedOut[id] = v
    },
  }
}

const SERVER = { id: 'srv-b', url: 'https://b.example.com' }

beforeEach(() => {
  localStorage.clear()
  post.mockReset()
  get.mockReset()
})
afterEach(() => {
  localStorage.clear()
})

describe('inactive-server badges (#2930)', () => {
  it('reports the unread count and stores the rotated refresh token', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-old')
    post.mockResolvedValue({ data: { access_token: 'at-1', refresh_token: 'rt-new' } })
    get.mockResolvedValue({ data: { count: 7 } })
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(post).toHaveBeenCalledWith(
      'https://b.example.com/api/auth/refresh',
      { refresh_token: 'rt-old' },
      expect.objectContaining({ timeout: expect.any(Number) }),
    )
    // Rotation: the backend revokes the chain if the old token is ever posted
    // twice, so the new one must be on disk immediately.
    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-new')
    expect(get).toHaveBeenCalledWith(
      'https://b.example.com/api/notifications/unread-count',
      expect.objectContaining({ headers: { Authorization: 'Bearer at-1' } }),
    )
    expect(store.badges[SERVER.id]).toBe(7)
    expect(store.signedOut[SERVER.id]).toBe(false)
  })

  it('keeps the rotated token even when the count request then fails', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-old')
    post.mockResolvedValue({ data: { access_token: 'at-1', refresh_token: 'rt-new' } })
    get.mockRejectedValue(Object.assign(new Error('boom'), { response: { status: 500 } }))
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-new')
    expect(store.signedOut[SERVER.id]).toBe(undefined)
  })

  it('marks the server signed out — and drops the dead token — on 401', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-dead')
    post.mockRejectedValue(Object.assign(new Error('nope'), { response: { status: 401 } }))
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe(null)
    expect(store.badges[SERVER.id]).toBe(0)
    expect(store.signedOut[SERVER.id]).toBe(true)
    expect(get).not.toHaveBeenCalled()
  })

  it('keeps the session when the server is merely unreachable', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-live')
    post.mockRejectedValue(new Error('Network Error')) // no response at all
    const store = fakeStore()

    await pollServer(store, SERVER)

    // An offline laptop must not sign anyone out of a perfectly live account.
    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-live')
    expect(store.signedOut[SERVER.id]).toBe(undefined)
    expect(store.badges[SERVER.id]).toBe(undefined)
  })

  it('does not call the network for a server it has no token for', async () => {
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(post).not.toHaveBeenCalled()
    expect(store.signedOut[SERVER.id]).toBe(true)
    expect(store.badges[SERVER.id]).toBe(0)
  })
})
