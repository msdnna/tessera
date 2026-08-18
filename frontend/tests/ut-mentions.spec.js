import { describe, it, expect } from 'vitest'
import { buildMentionItems, resolveMention, roleLabel } from '@/utils/mentions'

const MEMBERS = [
  { user_id: 'u1', name: 'Ann Lee', email: 'ann@t.io', role: 'owner' },
  { user_id: 'u2', name: 'Боб', email: 'bob@t.io', role: 'member' },
]
const GL = [{ gl_username: 'v.sokolov', gl_name: 'Виктор Соколов', gl_avatar_url: '/a.png' }]

describe('buildMentionItems', () => {
  it('gives both rosters the one shape mentions are matched by', () => {
    const items = buildMentionItems(MEMBERS, GL)
    expect(items).toHaveLength(3)
    expect(items[0]).toMatchObject({
      id: 'u1',
      label: 'Ann Lee', // the field highlightMentions matches on — the bug this fixes
      display: 'Ann Lee',
      email: 'ann@t.io',
      role: 'owner',
      avatarUserId: 'u1',
    })
  })

  it('inserts a GitLab-only user by @username so GitLab resolves it on writeback', () => {
    const gl = buildMentionItems([], GL)[0]
    expect(gl).toMatchObject({
      id: null,
      label: 'v.sokolov',
      display: 'Виктор Соколов',
      username: 'v.sokolov',
      avatarSrc: '/a.png',
      gitlab: true,
    })
  })

  it('falls back to the handle when the GitLab user has no display name', () => {
    expect(buildMentionItems([], [{ gl_username: 'kim' }])[0].display).toBe('kim')
  })

  // A member who signed in via GitLab OAuth is in both rosters: the Tessera row
  // carries their name, the GitLab row their @username. They must be folded into a
  // single item that inserts and resolves by that login — else "@v.sokolov"
  // resolves to nobody, and a mention pushed to GitLab wouldn't tag them.
  it('folds an OAuth-linked GitLab user into their Tessera member, inserting the login', () => {
    const linkedGl = [{ ...GL[0], tessera_user_id: 'u2' }]
    const items = buildMentionItems(MEMBERS, linkedGl)
    expect(items).toHaveLength(2) // not listed twice
    const bob = items.find((m) => m.id === 'u2')
    expect(bob).toMatchObject({
      id: 'u2',
      label: 'v.sokolov',
      display: 'Боб',
      username: 'v.sokolov',
    })
    // The GitLab handle resolves to that member (the reworked bug).
    expect(resolveMention(items, { label: 'v.sokolov' }).id).toBe('u2')
  })

  it('tolerates missing rosters', () => {
    expect(buildMentionItems(null, undefined)).toEqual([])
  })
})

describe('resolveMention', () => {
  const items = buildMentionItems(MEMBERS, GL)

  it('prefers the chip id — it pins the exact person past namesakes', () => {
    expect(resolveMention(items, { id: 'u2', label: 'Ann Lee' }).display).toBe('Боб')
  })

  it('falls back to the label when the chip carries no id', () => {
    expect(resolveMention(items, { id: '', label: 'Ann Lee' }).id).toBe('u1')
  })

  it('resolves a GitLab handle case-insensitively', () => {
    expect(resolveMention(items, { label: 'V.Sokolov' }).gitlab).toBe(true)
  })

  it('resolves by label when the id names nobody (stale chip)', () => {
    expect(resolveMention(items, { id: 'gone', label: 'Боб' }).id).toBe('u2')
  })

  it('returns null for a handle nobody owns — the caller shows no card', () => {
    expect(resolveMention(items, { label: 'someone' })).toBeNull()
    expect(resolveMention(items, { label: '' })).toBeNull()
    expect(resolveMention(null, { label: 'Боб' })).toBeNull()
  })
})

describe('roleLabel', () => {
  it('translates known roles', () => {
    expect(roleLabel('owner')).toBe('Владелец')
    expect(roleLabel('member')).toBe('Участник')
  })

  it('shows an unknown role verbatim rather than inventing one', () => {
    expect(roleLabel('guest')).toBe('guest')
    expect(roleLabel(null)).toBe('')
  })
})
