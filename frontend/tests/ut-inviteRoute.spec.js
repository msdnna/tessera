import { describe, it, expect } from 'vitest'
import {
  inviteTokenFromNext,
  loginRouteForInvite,
  registerRouteForInvite,
  invitePath,
} from '@/utils/inviteRoute'

// An invitation link is the invitee's one copy of the token: it arrives by mail,
// is spent once, and nothing on the board offers a second one. So every screen
// the link can bounce through has to carry it along — the moment a route drops
// it, the invitee is back to hunting through their inbox (#2932).
describe('invite token survives the account fork', () => {
  it('register → login keeps the token in ?next', () => {
    expect(loginRouteForInvite('abc123')).toEqual({
      path: '/login',
      query: { next: '/invite?token=abc123' },
    })
  })

  it('login → register keeps the token in ?invite', () => {
    expect(registerRouteForInvite('abc123')).toEqual({
      path: '/register',
      query: { invite: 'abc123' },
    })
  })

  it('reads the token back out of a ?next that points at the invite page', () => {
    expect(inviteTokenFromNext('/invite?token=abc123')).toBe('abc123')
  })

  // The token is URL-encoded on the way in; a raw one that happens to contain a
  // reserved character must come back out unharmed rather than truncated at it.
  it('round-trips a token with reserved characters', () => {
    const tok = 'a+b/c=d&e'
    const next = loginRouteForInvite(tok).query.next
    expect(inviteTokenFromNext(next)).toBe(tok)
  })

  it('plain routes when there is no token', () => {
    expect(loginRouteForInvite('')).toEqual({ path: '/login' })
    expect(loginRouteForInvite(null)).toEqual({ path: '/login' })
    expect(registerRouteForInvite('')).toEqual({ path: '/register' })
    expect(registerRouteForInvite(undefined)).toEqual({ path: '/register' })
  })

  // `next` is attacker-supplied in the sense that anyone can hand out a link to
  // /login?next=… — only an invite path may become an invitation token.
  it('ignores a ?next that is not the invite page', () => {
    expect(inviteTokenFromNext('/')).toBe('')
    expect(inviteTokenFromNext('/settings?token=abc')).toBe('')
    expect(inviteTokenFromNext('https://evil.example/invite?token=abc')).toBe('')
    expect(inviteTokenFromNext('/invite')).toBe('')
    expect(inviteTokenFromNext('')).toBe('')
    expect(inviteTokenFromNext(undefined)).toBe('')
    expect(inviteTokenFromNext(['/invite?token=abc'])).toBe('')
  })

  it('invitePath builds the page link the mail points at', () => {
    expect(invitePath('abc123')).toBe('/invite?token=abc123')
    expect(invitePath('')).toBe('/invite')
  })
})
