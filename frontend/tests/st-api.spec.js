import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import axios from 'axios'
import api, { auth, gitlab, boards, setAccessToken, getAccessToken, restoreSession } from '@/api'
import { connection } from '@/composables/useConnection'

// The api module builds an axios instance with real interceptors (token header,
// refresh-on-401 coalescing, error humanizing). We drive it by swapping the
// low-level HTTP adapter so no network is touched. Both the instance adapter and
// the global axios default adapter are stubbed: the instance handles normal
// requests, and refreshAccessToken() uses a bare axios.post for /auth/refresh.

let instanceAdapter
let globalAdapter

// Build an axios-style success/error response for a mock adapter.
function ok(config, data, status = 200) {
  return { data, status, statusText: 'OK', headers: {}, config, request: {} }
}
function fail(config, status, data) {
  const res = { data, status, statusText: 'ERR', headers: {}, config, request: {} }
  const err = new Error(`Request failed with status code ${status}`)
  err.config = config
  err.response = res
  err.isAxiosError = true
  return Promise.reject(err)
}

beforeEach(() => {
  localStorage.clear()
  // The access token is module state now (#2684), not localStorage — reset it
  // between cases or one test's session leaks into the next.
  setAccessToken('')
  connection.pending = 0
  connection.slow = false
  connection.offline = false
  instanceAdapter = vi.fn()
  globalAdapter = vi.fn()
  api.defaults.adapter = instanceAdapter
  axios.defaults.adapter = globalAdapter
})
afterEach(() => {
  localStorage.clear()
  setAccessToken('')
  delete window.__TAURI_INTERNALS__
})

describe('api request interceptor', () => {
  it('attaches the in-memory Bearer token', async () => {
    setAccessToken('mytoken')
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, { ok: 1 })))
    const res = await auth.me()
    expect(res.data).toEqual({ ok: 1 })
    const sent = instanceAdapter.mock.calls[0][0]
    expect(sent.headers.Authorization).toBe('Bearer mytoken')
    // baseURL resolves the request onto /api/auth/me.
    expect(sent.url).toContain('/auth/me')
    expect(sent.baseURL).toBe('/api')
  })

  it('never persists the access token to localStorage (#2684)', async () => {
    setAccessToken('mytoken')
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, {})))
    await auth.me()
    // XSS reads localStorage; it cannot read a module-scoped variable.
    expect(localStorage.getItem('tessera_token')).toBeNull()
  })

  it('omits Authorization when there is no token', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, {})))
    await auth.providers()
    const sent = instanceAdapter.mock.calls[0][0]
    expect(sent.headers.Authorization).toBeUndefined()
  })

  it('counts a normal request in the connection loader', async () => {
    instanceAdapter.mockImplementation((config) => {
      // pending is incremented before the adapter runs.
      expect(connection.pending).toBe(1)
      return Promise.resolve(ok(config, {}))
    })
    await auth.me()
    expect(connection.pending).toBe(0)
  })

  it('skipLoader requests do not touch the connection loader', async () => {
    instanceAdapter.mockImplementation((config) => {
      expect(connection.pending).toBe(0)
      return Promise.resolve(ok(config, {}))
    })
    // gitlab.sync is flagged skipLoader.
    await gitlab.sync('w1', 'i1')
    expect(connection.pending).toBe(0)
  })
})

describe('boards.setDoneColumn', () => {
  // Clearing the done column (#2588) must send an explicit column_id: null —
  // an omitted key would leave the board's current done column untouched.
  it('sends column_id: null when clearing', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, {})))
    await boards.setDoneColumn('b1', null)
    const sent = instanceAdapter.mock.calls[0][0]
    expect(sent.method).toBe('patch')
    expect(sent.url).toContain('/boards/b1/done-column')
    const body = JSON.parse(sent.data)
    expect('column_id' in body).toBe(true)
    expect(body.column_id).toBeNull()
  })

  it('sends the column id when setting', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, {})))
    await boards.setDoneColumn('b1', 'c9')
    expect(JSON.parse(instanceAdapter.mock.calls[0][0].data)).toEqual({ column_id: 'c9' })
  })
})

describe('api error handling', () => {
  it('humanizes a backend error string and rejects with an Error', async () => {
    instanceAdapter.mockImplementation((config) =>
      fail(config, 400, { error: 'invalid credentials' }),
    )
    await expect(auth.login({})).rejects.toThrow('Неверный email или пароль')
  })

  it('marks offline when no response is reached (network error)', async () => {
    instanceAdapter.mockImplementation(() => {
      const err = new Error('Network Error')
      err.isAxiosError = true
      return Promise.reject(err)
    })
    await expect(auth.me()).rejects.toThrow()
    expect(connection.offline).toBe(true)
  })

  it('tags a network error with offline=true so callers can retry', async () => {
    instanceAdapter.mockImplementation(() => {
      const err = new Error('Network Error')
      err.isAxiosError = true
      return Promise.reject(err)
    })
    await expect(auth.me()).rejects.toMatchObject({ offline: true })
  })

  it('a reached error response leaves offline false', async () => {
    connection.offline = true
    instanceAdapter.mockImplementation((config) => fail(config, 500, { error: 'boom' }))
    await expect(auth.me()).rejects.toThrow()
    expect(connection.offline).toBe(false)
  })

  it('an HTTP error carries offline=false (not a connectivity failure)', async () => {
    instanceAdapter.mockImplementation((config) => fail(config, 500, { error: 'boom' }))
    await expect(auth.me()).rejects.toMatchObject({ offline: false })
  })
})

describe('refresh-on-401', () => {
  it('refreshes on 401 with the cookie (no token in the body) and retries', async () => {
    setAccessToken('old')
    let calls = 0
    instanceAdapter.mockImplementation((config) => {
      calls++
      if (calls === 1) return fail(config, 401, { error: 'unauthorized' })
      return Promise.resolve(ok(config, { retried: true }))
    })
    globalAdapter.mockImplementation((config) => Promise.resolve(ok(config, { access_token: 'new' })))

    const res = await api.get('/protected')
    expect(res.data).toEqual({ retried: true })
    expect(getAccessToken()).toBe('new')
    expect(localStorage.getItem('tessera_token')).toBeNull()
    // Retry carried the fresh token.
    const retry = instanceAdapter.mock.calls[1][0]
    expect(retry.headers.Authorization).toBe('Bearer new')
    // One refresh call to /auth/refresh, in cookie mode with an empty body: the
    // refresh token rides in the httpOnly cookie the browser attaches itself.
    expect(globalAdapter).toHaveBeenCalledTimes(1)
    const sent = globalAdapter.mock.calls[0][0]
    expect(sent.url).toContain('/auth/refresh')
    expect(sent.headers['X-Auth-Mode']).toBe('cookie')
    expect(JSON.parse(sent.data)).toEqual({})
  })

  it('coalesces concurrent 401s into a single refresh', async () => {
    setAccessToken('old')
    const seen = {}
    instanceAdapter.mockImplementation((config) => {
      const key = config.url
      seen[key] = (seen[key] || 0) + 1
      if (seen[key] === 1) return fail(config, 401, { error: 'unauthorized' })
      return Promise.resolve(ok(config, { url: key }))
    })
    let refreshCalls = 0
    globalAdapter.mockImplementation((config) => {
      refreshCalls++
      // Small async gap so both 401s arrive before it resolves.
      return new Promise((resolve) =>
        setTimeout(() => resolve(ok(config, { access_token: 'new', refresh_token: 'ref2' })), 5),
      )
    })

    const [a, b] = await Promise.all([api.get('/a'), api.get('/b')])
    expect(a.data.url).toBe('/a')
    expect(b.data.url).toBe('/b')
    // Two 401s but only one shared refresh.
    expect(refreshCalls).toBe(1)
  })

  it('logs out (clears storage + emits auth:expired) when refresh fails', async () => {
    setAccessToken('old')
    localStorage.setItem('tessera_refresh_token', 'ref1')
    localStorage.setItem('tessera_user', '{"id":"u"}')
    instanceAdapter.mockImplementation((config) => fail(config, 401, { error: 'unauthorized' }))
    globalAdapter.mockImplementation((config) => fail(config, 401, { error: 'bad refresh' }))
    const spy = vi.fn()
    window.addEventListener('auth:expired', spy)

    await expect(api.get('/protected')).rejects.toThrow()
    expect(getAccessToken()).toBe('')
    expect(localStorage.getItem('tessera_refresh_token')).toBeNull()
    expect(localStorage.getItem('tessera_user')).toBeNull()
    expect(spy).toHaveBeenCalledTimes(1)
    window.removeEventListener('auth:expired', spy)
  })

  it('does not attempt refresh for a 401 on the refresh endpoint itself', async () => {
    localStorage.setItem('tessera_refresh_token', 'ref1')
    // A direct call to the refresh URL through the instance returning 401 must not
    // recurse into refreshAccessToken.
    instanceAdapter.mockImplementation((config) => fail(config, 401, { error: 'bad' }))
    await expect(api.post('/auth/refresh', {})).rejects.toThrow()
    expect(globalAdapter).not.toHaveBeenCalled()
    // Refresh-endpoint 401 is still treated as unauthorized-but-not-retryable, so
    // it does NOT clear storage (isRefreshCall guard on the logout branch).
    expect(localStorage.getItem('tessera_refresh_token')).toBe('ref1')
  })

  it('retries a 401 only once (no infinite loop) if the retry also 401s', async () => {
    setAccessToken('old')
    let calls = 0
    instanceAdapter.mockImplementation((config) => {
      calls++
      return fail(config, 401, { error: 'unauthorized' })
    })
    globalAdapter.mockImplementation((config) =>
      Promise.resolve(ok(config, { access_token: 'new', refresh_token: 'ref2' })),
    )
    await expect(api.get('/protected')).rejects.toThrow()
    // Original + one retry = 2 instance calls, then it gives up.
    expect(calls).toBe(2)
  })
})

// #2684. Two delivery modes for the refresh token: the web build is same-origin
// with the API and gets an httpOnly cookie; the desktop build talks to it
// cross-origin, where that cookie would never be sent, so it keeps posting the
// token in the body. Everything below pins which one applies where.
describe('refresh-token delivery mode', () => {
  it('asks for cookie delivery on register, login and logout (web)', async () => {
    instanceAdapter.mockImplementation((config) => Promise.resolve(ok(config, {})))
    await auth.register({ email: 'a@b.c', name: 'A', password: 'password-123' })
    await auth.login({ email: 'a@b.c', password: 'password-123' })
    await auth.logout()
    for (const [config] of instanceAdapter.mock.calls) {
      expect(config.headers['X-Auth-Mode']).toBe('cookie')
    }
    // Nothing to send in the body: the cookie is the credential.
    expect(JSON.parse(instanceAdapter.mock.calls[2][0].data)).toEqual({})
  })

  it('restoreSession trades the cookie for a fresh access token on start-up', async () => {
    globalAdapter.mockImplementation((config) => Promise.resolve(ok(config, { access_token: 'boot' })))
    await expect(restoreSession()).resolves.toBe('boot')
    expect(getAccessToken()).toBe('boot')
  })

  it('restoreSession resolves null when there is no session to restore', async () => {
    globalAdapter.mockImplementation((config) => fail(config, 401, { error: 'invalid refresh token' }))
    await expect(restoreSession()).resolves.toBeNull()
    expect(getAccessToken()).toBe('')
  })

  describe('desktop (Tauri, cross-origin)', () => {
    beforeEach(() => {
      // What isTauri() looks for; the webview injects it in the real app.
      window.__TAURI_INTERNALS__ = {}
    })

    it('keeps posting the stored refresh token and sends no cookie-mode header', async () => {
      localStorage.setItem('tessera_refresh_token', 'ref1')
      setAccessToken('old')
      let calls = 0
      instanceAdapter.mockImplementation((config) => {
        calls++
        if (calls === 1) return fail(config, 401, { error: 'unauthorized' })
        return Promise.resolve(ok(config, { retried: true }))
      })
      globalAdapter.mockImplementation((config) =>
        Promise.resolve(ok(config, { access_token: 'new', refresh_token: 'ref2' })),
      )

      await api.get('/protected')
      const sent = globalAdapter.mock.calls[0][0]
      expect(JSON.parse(sent.data)).toEqual({ refresh_token: 'ref1' })
      expect(sent.headers['X-Auth-Mode']).toBeUndefined()
      // The rotated token is stored again — desktop has nowhere else to keep it.
      expect(localStorage.getItem('tessera_refresh_token')).toBe('ref2')
      expect(getAccessToken()).toBe('new')
    })

    it('logs out immediately on 401 when no refresh token is stored', async () => {
      setAccessToken('old')
      localStorage.setItem('tessera_user', '{"id":"u"}')
      instanceAdapter.mockImplementation((config) => fail(config, 401, { error: 'unauthorized' }))
      const spy = vi.fn()
      window.addEventListener('auth:expired', spy)

      await expect(api.get('/protected')).rejects.toThrow()
      // Nothing to refresh with → no refresh attempt at all.
      expect(globalAdapter).not.toHaveBeenCalled()
      expect(getAccessToken()).toBe('')
      expect(spy).toHaveBeenCalledTimes(1)
      window.removeEventListener('auth:expired', spy)
    })
  })
})
