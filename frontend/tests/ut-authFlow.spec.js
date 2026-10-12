// Pure helpers of the account screens and the shared invitation link (#3035).
import { describe, it, expect } from 'vitest'
import { safeNext, formatCountdown, minutesLeft, initialsOfSpace } from '@/utils/authFlow'
import { daysLeft, linkMeta, linkUses, shortLink } from '@/utils/inviteLink'

describe('safeNext', () => {
  it('keeps in-app paths', () => {
    expect(safeNext('/settings?s=profile')).toBe('/settings?s=profile')
    expect(safeNext('/')).toBe('/')
  })
  it('refuses anything that leaves the app', () => {
    for (const bad of [
      'https://evil.example',
      '//evil.example',
      '/\\evil.example',
      'settings',
      '',
    ]) {
      expect(safeNext(bad)).toBe('/')
    }
    expect(safeNext(undefined, '/home')).toBe('/home')
    expect(safeNext(['/a'])).toBe('/')
  })
})

describe('formatCountdown', () => {
  it('reads like the mockup', () => {
    expect(formatCountdown(42)).toBe('0:42')
    expect(formatCountdown(60)).toBe('1:00')
    expect(formatCountdown(605)).toBe('10:05')
    expect(formatCountdown(0.2)).toBe('0:01')
    expect(formatCountdown(-3)).toBe('0:00')
  })
})

describe('minutesLeft', () => {
  const now = Date.parse('2026-10-10T12:00:00Z')
  it('rounds up, so a live link never reads «0 минут»', () => {
    expect(minutesLeft('2026-10-10T12:54:00Z', now)).toBe(54)
    expect(minutesLeft('2026-10-10T12:00:40Z', now)).toBe(1)
  })
  it('is zero for a past or broken deadline', () => {
    expect(minutesLeft('2026-10-10T11:59:00Z', now)).toBe(0)
    expect(minutesLeft('nonsense', now)).toBe(0)
  })
})

describe('initialsOfSpace', () => {
  it('takes two letters, skipping quotes', () => {
    expect(initialsOfSpace('Студия «Север»')).toBe('СС')
    expect(initialsOfSpace('tessera')).toBe('TE')
    expect(initialsOfSpace('')).toBe('?')
  })
})

describe('shared invitation link', () => {
  const now = Date.parse('2026-10-10T12:00:00Z')
  const t = (key, named = {}, plural) => {
    if (key === 'shell.members.days') return `${named.n} дн`
    if (key === 'shell.members.usesOf') return `${named.n}/${named.max}`
    return key.split('.').pop() + (plural ?? '')
  }
  it('counts whole days left, rounding up', () => {
    expect(daysLeft('2026-10-16T11:00:00Z', now)).toBe(6)
    expect(daysLeft('2026-10-10T13:00:00Z', now)).toBe(1)
    expect(daysLeft('2026-10-09T13:00:00Z', now)).toBe(0)
  })
  it('words the row: role · days · uses', () => {
    const link = { role: 'member', expires_at: '2026-10-16T11:00:00Z', uses: 3, max_uses: 25 }
    expect(linkMeta(link, t, now)).toBe('member · 6 дн')
    expect(linkMeta({ ...link, role: 'admin' }, t, now)).toBe('admin · 6 дн')
    expect(linkUses(link, t)).toBe('3/25')
    expect(linkMeta(null, t, now)).toBe('')
  })
  it('shows the link without its scheme', () => {
    expect(shortLink('https://tessera.dev/join/abc')).toBe('tessera.dev/join/abc')
    expect(shortLink('')).toBe('')
  })
})
