import { describe, it, expect } from 'vitest'
import { parseOAuthDeepLink, OAUTH_DEEP_LINK } from '@/composables/useDesktopOAuth'

// #2696: the desktop app receives its session as a tessera:// deep link. Everything that
// reaches the OS handler goes through this parser, so it has to be strict — a link that
// isn't the OAuth callback must return null rather than a half-filled session.
describe('parseOAuthDeepLink', () => {
  it('reads access + refresh out of the fragment', () => {
    const url = `${OAUTH_DEEP_LINK}#access_token=acc.123&refresh_token=ref.456`
    expect(parseOAuthDeepLink(url)).toEqual({ access: 'acc.123', refresh: 'ref.456', error: null })
  })

  it('accepts an access token without a refresh token', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#access_token=acc.123`)).toEqual({
      access: 'acc.123',
      refresh: null,
      error: null,
    })
  })

  it('url-decodes fragment values', () => {
    const url = `${OAUTH_DEEP_LINK}#access_token=${encodeURIComponent('a+b/c=')}`
    expect(parseOAuthDeepLink(url).access).toBe('a+b/c=')
  })

  it('surfaces the backend error branch', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#oauth_error=not_configured`)).toEqual({
      access: null,
      refresh: null,
      error: 'not_configured',
    })
  })

  it('reports a callback with no token as no_token rather than a session', () => {
    expect(parseOAuthDeepLink(`${OAUTH_DEEP_LINK}#state=abc`)).toEqual({
      access: null,
      refresh: null,
      error: 'no_token',
    })
    expect(parseOAuthDeepLink(OAUTH_DEEP_LINK)).toEqual({
      access: null,
      refresh: null,
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
