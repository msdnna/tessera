import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// The SDK is mocked: initSentry's whole job is deciding *whether* and *with what*
// to call Sentry.init, and a real init would open a transport in jsdom.
const init = vi.fn()
vi.mock('@sentry/vue', () => ({
  init: (...args) => init(...args),
  browserTracingIntegration: (opts) => ({ name: 'BrowserTracing', opts }),
}))

import { initSentry } from '@/utils/sentry'

const app = { name: 'app' }
const router = { name: 'router' }

// respondWith installs a fetch stub answering /api/client-config with `body`.
function respondWith(body, ok = true) {
  const fetchMock = vi.fn().mockResolvedValue({ ok, json: async () => body })
  globalThis.fetch = fetchMock
  return fetchMock
}

describe('initSentry', () => {
  beforeEach(() => {
    init.mockClear()
    globalThis.__APP_VERSION__ = '9.9.9'
  })

  afterEach(() => {
    delete globalThis.fetch
    delete globalThis.__APP_VERSION__
    vi.useRealTimers()
  })

  it('stays off when the backend reports no Sentry', async () => {
    respondWith({ sentry: null })
    expect(await initSentry(app, router)).toBe(false)
    expect(init).not.toHaveBeenCalled()
  })

  it('stays off when the config has no dsn', async () => {
    respondWith({ sentry: { environment: 'production', tunnel: '/api/sentry-tunnel' } })
    expect(await initSentry(app, router)).toBe(false)
    expect(init).not.toHaveBeenCalled()
  })

  it('stays off when the config request fails', async () => {
    respondWith({ sentry: { dsn: 'http://k@sentry.lan/2' } }, false)
    expect(await initSentry(app, router)).toBe(false)
    expect(init).not.toHaveBeenCalled()
  })

  // A dead backend must not take the app down with it — this is the case that
  // matters, since main.js starts telemetry on every load.
  it('stays off (without throwing) when fetch rejects', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('network down'))
    expect(await initSentry(app, router)).toBe(false)
    expect(init).not.toHaveBeenCalled()
  })

  it('initialises with the tunnel, release and router integration', async () => {
    const fetchMock = respondWith({
      sentry: {
        dsn: 'http://key@sentry.lan/3',
        environment: 'production',
        tracesSampleRate: 0.25,
        tunnel: '/api/sentry-tunnel',
      },
    })

    expect(await initSentry(app, router)).toBe(true)
    // Web build: apiBaseURL() is same-origin '/api'.
    expect(fetchMock.mock.calls[0][0]).toBe('/api/client-config')

    const opts = init.mock.calls[0][0]
    expect(opts.app).toBe(app)
    expect(opts.dsn).toBe('http://key@sentry.lan/3')
    expect(opts.environment).toBe('production')
    expect(opts.tracesSampleRate).toBe(0.25)
    // Events go to the backend relay, never to the DSN host directly.
    expect(opts.tunnel).toBe('/api/sentry-tunnel')
    expect(opts.release).toBe('tessera-web@9.9.9')
    expect(opts.integrations[0].opts.router).toBe(router)
    expect(opts.ignoreErrors).toContain('ResizeObserver loop limit exceeded')
  })

  it('defaults the trace sample rate when the backend omits it', async () => {
    respondWith({ sentry: { dsn: 'http://key@sentry.lan/3', tunnel: '/api/sentry-tunnel' } })
    await initSentry(app, router)
    expect(init.mock.calls[0][0].tracesSampleRate).toBe(0.1)
  })

  // The 3s cap exists so a hanging backend can't stall telemetry startup (and,
  // with it, anything a caller chains onto initSentry).
  it('gives up on a hanging config request', async () => {
    vi.useFakeTimers()
    globalThis.fetch = vi.fn(
      (_url, { signal }) =>
        new Promise((_resolve, reject) => {
          signal.addEventListener('abort', () => reject(new Error('aborted')))
        }),
    )
    const pending = initSentry(app, router)
    await vi.advanceTimersByTimeAsync(3000)
    expect(await pending).toBe(false)
    expect(init).not.toHaveBeenCalled()
  })
})
