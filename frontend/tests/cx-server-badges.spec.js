import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'

const { post, get } = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }))
vi.mock('axios', () => ({ default: { post, get } }))

import { pollServer, resetBadgeCache } from '@/composables/useServerBadges'
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
  resetBadgeCache()
  post.mockReset()
  get.mockReset()
})
afterEach(() => {
  localStorage.clear()
  resetBadgeCache()
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
    // The rotated token must be on disk immediately — a lost response after this
    // point must not leave the old, single-use token behind.
    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-new')
    expect(get).toHaveBeenCalledWith(
      'https://b.example.com/api/notifications/unread-count',
      expect.objectContaining({ headers: { Authorization: 'Bearer at-1' } }),
    )
    expect(store.badges[SERVER.id]).toBe(7)
    expect(store.signedOut[SERVER.id]).toBe(false)
  })

  it('reuses the cached access token instead of rotating on every poll', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-old')
    post.mockResolvedValue({ data: { access_token: 'at-1', refresh_token: 'rt-new' } })
    get.mockResolvedValue({ data: { count: 3 } })
    const store = fakeStore()

    await pollServer(store, SERVER) // rotates once, caches at-1
    await pollServer(store, SERVER)
    await pollServer(store, SERVER)

    // Rotating the refresh token is the risky part — do it once per access-token
    // lifetime, not once a minute. The later polls are a bare count request.
    expect(post).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledTimes(3)
    expect(store.badges[SERVER.id]).toBe(3)
  })

  it('heals a lost rotation by retrying inside the grace window', async () => {
    vi.useFakeTimers()
    try {
      sSetFor('tessera_refresh_token', SERVER.id, 'rt-old')
      // First refresh never answers (dropped response), the retry replays it.
      post
        .mockRejectedValueOnce(new Error('Network Error')) // no response → transient
        .mockResolvedValueOnce({ data: { access_token: 'at-2', refresh_token: 'rt-replayed' } })
      get.mockResolvedValue({ data: { count: 1 } })
      const store = fakeStore()

      const done = pollServer(store, SERVER)
      await vi.runAllTimersAsync()
      await done

      expect(post).toHaveBeenCalledTimes(2)
      expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-replayed')
      expect(store.badges[SERVER.id]).toBe(1)
      expect(store.signedOut[SERVER.id]).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps the session and token when every refresh attempt is transient', async () => {
    vi.useFakeTimers()
    try {
      sSetFor('tessera_refresh_token', SERVER.id, 'rt-live')
      post.mockRejectedValue(Object.assign(new Error('boom'), { response: { status: 503 } }))
      const store = fakeStore()

      const done = pollServer(store, SERVER)
      await vi.runAllTimersAsync()
      await done

      // An unreachable server must not sign anyone out of a live account, and the
      // stored token stays put — the grace window may still accept it next tick.
      expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe('rt-live')
      expect(store.signedOut[SERVER.id]).toBe(undefined)
      expect(store.badges[SERVER.id]).toBe(undefined)
      expect(get).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('marks the server signed out — and drops the dead token — on 401', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-dead')
    post.mockRejectedValue(Object.assign(new Error('nope'), { response: { status: 401 } }))
    const store = fakeStore()

    await pollServer(store, SERVER)

    // A 401 is an answered "no" — stop retrying at once and drop the token.
    expect(post).toHaveBeenCalledTimes(1)
    expect(sGetFor('tessera_refresh_token', SERVER.id)).toBe(null)
    expect(store.badges[SERVER.id]).toBe(0)
    expect(store.signedOut[SERVER.id]).toBe(true)
    expect(get).not.toHaveBeenCalled()
  })

  it('re-rotates when the cached access token is rejected by the count request', async () => {
    sSetFor('tessera_refresh_token', SERVER.id, 'rt-old')
    post.mockResolvedValue({ data: { access_token: 'at-1', refresh_token: 'rt-new' } })
    // The count says the access token expired early; a fresh rotation fixes it.
    get
      .mockRejectedValueOnce(Object.assign(new Error('stale'), { response: { status: 401 } }))
      .mockResolvedValueOnce({ data: { count: 5 } })
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(post).toHaveBeenCalledTimes(2)
    expect(get).toHaveBeenCalledTimes(2)
    expect(store.badges[SERVER.id]).toBe(5)
    expect(store.signedOut[SERVER.id]).toBe(false)
  })

  it('does not call the network for a server it has no token for', async () => {
    const store = fakeStore()

    await pollServer(store, SERVER)

    expect(post).not.toHaveBeenCalled()
    expect(store.signedOut[SERVER.id]).toBe(true)
    expect(store.badges[SERVER.id]).toBe(0)
  })
})
