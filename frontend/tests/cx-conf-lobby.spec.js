import { describe, it, expect } from 'vitest'
import { canModerateConference, confSeat } from '@/utils/confLobby'

// The lobby's permission rule, pinned apart from the view (#2932). It restates
// the server's canManageConference — the creator, a host in the call, or an
// owner/admin of the workspace — and the Android client answers the same
// question in util/ConferenceLobby.kt. When the server moves, all three move.

const conf = (over = {}) => ({ id: 'c1', workspace_id: 'ws1', created_by: 'u1', ...over })
const part = (over = {}) => ({ user_id: 'u1', role: 'member', ...over })

describe('confSeat', () => {
  it('finds my row and ignores a blank id', () => {
    const roster = [part({ user_id: 'u1' }), part({ user_id: 'u2', role: 'host' })]
    expect(confSeat(roster, 'u2').role).toBe('host')
    expect(confSeat(roster, 'u3')).toBeNull()
    // '' is what the session reports before /auth/me lands — it must not match
    // a row whose user_id is missing.
    expect(confSeat([part({ user_id: undefined })], '')).toBeNull()
  })
})

describe('canModerateConference', () => {
  it('lets the creator end their own call', () => {
    expect(canModerateConference(conf(), [], 'u1', false)).toBe(true)
  })

  it('lets an owner/admin of the workspace end someone else’s call', () => {
    expect(canModerateConference(conf(), [part()], 'u9', true)).toBe(true)
  })

  it('lets a host enrolled in the call moderate it', () => {
    expect(
      canModerateConference(conf(), [part({ user_id: 'u9', role: 'host' })], 'u9', false),
    ).toBe(true)
  })

  it('refuses a plain member — the button would answer 403', () => {
    expect(canModerateConference(conf(), [part({ user_id: 'u9' })], 'u9', false)).toBe(false)
  })

  it('refuses before the conference or the session has loaded', () => {
    expect(canModerateConference(null, [], 'u1', true)).toBe(false)
    // No id yet: a truthy workspace role must not hand the controls to nobody.
    expect(canModerateConference(conf(), [], '', true)).toBe(false)
  })
})
