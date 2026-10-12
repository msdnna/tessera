// Members v2 (#3063): activity, groups and the roles table of the «Участники» window.
import { describe, it, expect } from 'vitest'
import {
  ONLINE_MS,
  lastActivity,
  activityLabel,
  groupsOf,
  groupMembers,
  groupToggle,
  ROLE_MATRIX,
} from '@/utils/memberActivity'

const NOW = Date.parse('2026-10-10T12:00:00Z')
const ago = (ms) => new Date(NOW - ms).toISOString()

describe('lastActivity', () => {
  it('takes the later of seen and active, online within the window', () => {
    const a = lastActivity({ last_seen_at: ago(3600e3), last_active_at: ago(60e3) }, NOW)
    expect(a.at).toBe(NOW - 60e3)
    expect(a.online).toBe(true)
    expect(lastActivity({ last_seen_at: ago(ONLINE_MS + 1) }, NOW).online).toBe(false)
    expect(lastActivity({}, NOW)).toEqual({ at: 0, online: false })
  })
})

describe('activityLabel', () => {
  it('buckets the distance', () => {
    expect(activityLabel(0, NOW).key).toBe('never')
    expect(activityLabel(NOW - 30e3, NOW).key).toBe('now')
    expect(activityLabel(NOW - 12 * 60e3, NOW)).toEqual({ key: 'min', n: 12 })
    expect(activityLabel(NOW - 2 * 3600e3, NOW)).toEqual({ key: 'h', n: 2 })
    expect(activityLabel(NOW - 30 * 3600e3, NOW).key).toBe('yesterday')
    expect(activityLabel(NOW - 3 * 86400e3, NOW)).toEqual({ key: 'days', n: 3 })
    expect(activityLabel(NOW - 30 * 86400e3, NOW).key).toBe('date')
  })
})

describe('groups', () => {
  const groups = [
    { id: 'g1', name: 'Фронтенд', member_ids: ['a', 'b'] },
    { id: 'g2', name: 'Бэкенд', member_ids: ['b', 'gone'] },
  ]
  const roster = [{ user_id: 'a' }, { user_id: 'b' }]
  it('names the groups of a member', () => {
    expect(groupsOf('b', groups)).toEqual(['Фронтенд', 'Бэкенд'])
    expect(groupsOf('', groups)).toEqual([])
  })
  it('keeps only people on the roster', () => {
    expect(groupMembers(groups[1], roster).map((m) => m.user_id)).toEqual(['b'])
  })
  it('a group pick adds the missing, or takes all off when all are on', () => {
    expect(groupToggle(groups[0], ['a'])).toEqual({ add: ['b'], remove: [], all: false })
    expect(groupToggle(groups[0], ['a', 'b'])).toEqual({ add: [], remove: ['a', 'b'], all: true })
    // An id no longer in the workspace is never assigned.
    expect(groupToggle(groups[1], [], roster).add).toEqual(['b'])
  })
})

describe('ROLE_MATRIX', () => {
  it('keeps the owner-only rights with the owner and lets the owner not leave', () => {
    const row = (k) => ROLE_MATRIX.find((r) => r.key === k)
    expect(row('transfer')).toMatchObject({ owner: true, admin: false, member: false })
    expect(row('delete')).toMatchObject({ owner: true, admin: false, member: false })
    expect(row('leave')).toMatchObject({ owner: false, admin: true, member: true })
    expect(row('invite').member).toBe(false)
  })
})
