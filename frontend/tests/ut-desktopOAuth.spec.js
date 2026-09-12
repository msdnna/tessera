import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  consumeOAuthState,
  exchangeOAuthCode,
  parseOAuthDeepLink,
  OAUTH_DEEP_LINK,
} from '@/composables/useDesktopOAuth'

// The exchange is the only network call this composable makes; stub the client so the
// test can assert on what was sent rather than on axios.
const { oauthHandoff } = vi.hoisted(() => ({ oauthHandoff: vi.fn() }))
vi.mock('@/api', () => ({ auth: { oauthHandoff } }))

// There is no OS to hand the URL to under vitest, so record what the app asked to open —
// that URL is the only place the pending nonce is observable from outside.
const { opened } = vi.hoisted(() => ({ opened: [] }))
vi.mock('@tauri-apps/plugin-opener', () => ({
  openUrl: async (url) => {
    opened.push(url)
  },
}))

// #2696: the desktop app receives its session as a tessera:// deep link. Everything that
// reaches the OS handler goes through this parser, so it has to be strict — a link that
// isn't the OAuth callback must return null rather than a half-filled session.
describe('parseOAuthDeepLink', () => {
  it('reads the one-time code out of the fragment', () => {
    const url = `${OAUTH_DEEP_LINK}#code=c.123&state=n1`
    expect(parseOAuthDeepLink(url)).toEqual({ code: 'c.123', state: 'n1', error: null })
  })

  it('accepts a code without a state', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#code=c.123`)).toEqual({
      code: 'c.123',
      state: null,
      error: null,
    })
  })

  it('url-decodes fragment values', () => {
    const url = `${OAUTH_DEEP_LINK}#code=${encodeURIComponent('a+b/c=')}`
    expect(parseOAuthDeepLink(url).code).toBe('a+b/c=')
  })

  // #2932: on Linux and Windows the OS hands this URL to the app as a command-line
  // argument, and argv is readable by any local process — so the callback stopped putting
  // the session in it. A fragment carrying tokens is not a login any more; ignoring it
  // here is what makes an old-format (or forged) link useless.
  it('does not accept tokens in the fragment as a session', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#access_token=acc.1&refresh_token=ref.2`)).toEqual(
      { code: null, state: null, error: 'no_token' },
    )
  })

  it('surfaces the backend error branch', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#oauth_error=not_configured`)).toEqual({
      code: null,
      state: null,
      error: 'not_configured',
    })
  })

  it('reports a callback with no code as no_token rather than a session', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#state=abc`)).toEqual({
      code: null,
      state: 'abc',
      error: 'no_token',
    })
    expect(parseOAuthDeepLink(OAUTH_DEEP_LINK)).toEqual({
      code: null,
      state: null,
      error: 'no_token',
    })
  })

  it('ignores links that are not the OAuth callback', () => {
    expect(parseOAuthDeepLink('tessera://task/42')).toBeNull()
    expect(
      parseOAuthDeepLink('https://tessera.msdnna.website/oauth/callback#access_token=x'),
    ).toBeNull()
    // A future tessera://oauth/callback-something must not be swallowed by prefix match.
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}-other#access_token=x`)).toBeNull()
  })

  it('ignores non-string input', () => {
    expect(parseOAuthDeepLink(undefined)).toBeNull()
    expect(parseOAuthDeepLink(null)).toBeNull()
    expect(parseOAuthDeepLink(42)).toBeNull()
  })
})

// #2932: tessera:// is an OS-wide scheme, so a web page can open the callback link at the
// installed app. Without the nonce the app would redeem that handoff and sign the user
// into the attacker's account — over a live session, with nothing on screen to show for it.
// The nonce is read back through startDesktopGitlabLogin rather than poked into storage
// directly, so the test fails if the two halves ever stop agreeing on where it lives.
describe('consumeOAuthState', () => {
  // The nonce that the pending login actually handed to the browser.
  async function startLoginAndGetState() {
    const { startDesktopGitlabLogin } = await import('@/composables/useDesktopOAuth')
    expect(
      await startDesktopGitlabLogin('https://t.example/api/auth/gitlab/authorize?platform=desktop'),
    ).toBe(true)
    const url = new URL(opened.at(-1))
    return url.searchParams.get('app_state')
  }

  beforeEach(() => {
    localStorage.clear()
    opened.length = 0
  })

  it('accepts the handoff of the login this app started', async () => {
    const state = await startLoginAndGetState()
    expect(state).toMatch(/^[a-f0-9]{32}$/)
    expect(consumeOAuthState(state)).toBe(true)
  })

  it('rejects an unsolicited handoff when no login is pending', () => {
    expect(consumeOAuthState('deadbeef')).toBe(false)
    expect(consumeOAuthState(null)).toBe(false)
  })

  it('rejects a handoff carrying someone else’s nonce, or none at all', async () => {
    await startLoginAndGetState()
    expect(consumeOAuthState('deadbeef')).toBe(false)
    expect(consumeOAuthState(null)).toBe(false)
  })

  it('leaves the pending login alone after a forged handoff', async () => {
    const state = await startLoginAndGetState()
    expect(consumeOAuthState('deadbeef')).toBe(false)
    // The real one still has to land — otherwise any page could cancel a sign-in.
    expect(consumeOAuthState(state)).toBe(true)
  })

  it('burns the nonce so the same link cannot be replayed', async () => {
    const state = await startLoginAndGetState()
    expect(consumeOAuthState(state)).toBe(true)
    expect(consumeOAuthState(state)).toBe(false)
  })

  it('mints a fresh nonce per login', async () => {
    const first = await startLoginAndGetState()
    const second = await startLoginAndGetState()
    expect(second).not.toBe(first)
    expect(consumeOAuthState(first)).toBe(false)
    expect(consumeOAuthState(second)).toBe(true)
  })
})

// #2932: the session now arrives over HTTPS in exchange for the code, not in the deep
// link. The caller signs the user in with whatever this returns, so it has to fail loudly
// on a rejected code instead of handing back an empty session.
describe('exchangeOAuthCode', () => {
  // Braces, not a concise body: mockReset() returns the mock, and a hook that returns a
  // *function* hands vitest a teardown callback — it would call the mock after each test.
  beforeEach(() => {
    oauthHandoff.mockReset()
  })

  it('trades the code for the session tokens', async () => {
    oauthHandoff.mockResolvedValue({ data: { access_token: 'acc.1', refresh_token: 'ref.2' } })
    await expect(exchangeOAuthCode('c.123')).resolves.toEqual({
      access: 'acc.1',
      refresh: 'ref.2',
    })
    expect(oauthHandoff).toHaveBeenCalledWith('c.123')
  })

  it('normalises a missing refresh token to null', async () => {
    oauthHandoff.mockResolvedValue({ data: { access_token: 'acc.1' } })
    await expect(exchangeOAuthCode('c.123')).resolves.toEqual({ access: 'acc.1', refresh: null })
  })

  it('propagates a rejected code', async () => {
    oauthHandoff.mockRejectedValue(new Error('401'))
    let threw = false
    try {
      await exchangeOAuthCode('spent')
    } catch {
      threw = true
    }
    expect(threw).toBe(true)
  })
})
