import { describe, it, expect } from 'vitest'
import {
  LOCK_NONE,
  LOCK_VIEWER,
  LOCK_OWNER,
  LOCK_SELF,
  memberRowLock,
  memberRowBadge,
  canChangeMemberRole,
  canRemoveMember,
  addMemberRefusal,
} from '@/utils/members'

// The rules the members dialog runs on, mirroring handlers/workspaces.go (#2932).
// What is being pinned is not "an admin may not edit themselves" on its own —
// it is that the two refusals keep their order and that neither of them leaks
// onto anybody else's row.

const OWNER = { user_id: 'u-owner', email: 'Owner@Example.com', role: 'owner' }
const SELF = { user_id: 'u-self', email: 'admin@example.com', role: 'admin' }
const OTHER = { user_id: 'u-other', email: 'other@example.com', role: 'member' }
const LIST = [OWNER, SELF, OTHER]
const WHO = { ownerId: 'u-owner', selfId: 'u-self', selfEmail: 'admin@example.com' }

describe('memberRowLock', () => {
  it('locks the caller their own row', () => {
    expect(memberRowLock(SELF, WHO)).toBe(LOCK_SELF)
    expect(canChangeMemberRole(SELF, WHO)).toBe(false)
    expect(canRemoveMember(SELF, WHO)).toBe(false)
  })

  it('leaves every other member actionable', () => {
    expect(memberRowLock(OTHER, WHO)).toBe(LOCK_NONE)
    expect(canChangeMemberRole(OTHER, WHO)).toBe(true)
    expect(canRemoveMember(OTHER, WHO)).toBe(true)
  })

  it('keeps the owner locked as the owner, not as "yourself"', () => {
    // The handler checks the owner first, so an owner looking at their own row
    // must hear the owner rule — the one that is true whoever is asking.
    expect(memberRowLock(OWNER, { ownerId: 'u-owner', selfId: 'u-owner' })).toBe(LOCK_OWNER)
    expect(memberRowLock(OWNER, WHO)).toBe(LOCK_OWNER)
  })

  it('recognises the owner by the row role alone, with no workspace record', () => {
    expect(memberRowLock(OWNER, { selfId: 'u-self' })).toBe(LOCK_OWNER)
  })

  it('recognises the owner by id when the row role says otherwise', () => {
    // Defensive: a stale row that still says "admin" must not become removable
    // just because the list has not caught up with an ownership change.
    expect(memberRowLock({ user_id: 'u-owner', role: 'admin' }, WHO)).toBe(LOCK_OWNER)
  })

  it('locks nothing when the viewer is unknown', () => {
    // Before the auth store has a user there is no "self" — the dialog must not
    // start guessing, or it would lock a random row.
    expect(memberRowLock(OTHER, {})).toBe(LOCK_NONE)
    expect(memberRowLock(SELF, {})).toBe(LOCK_NONE)
    expect(memberRowLock(null, WHO)).toBe(LOCK_NONE)
  })
})

describe('memberRowLock: рядовой участник', () => {
  // requireManager — самый первый гейт на AddMember/UpdateMemberRole/
  // RemoveMember: рядовому участнику сервер отказывает, ещё не посмотрев, чья
  // это строка. Диалог при этом открыт всем (ListMembers — requireMember).
  const VIEWER = { ...WHO, selfId: 'u-other', selfEmail: 'other@example.com', canManage: false }

  it('запирает каждую строку, включая чужую', () => {
    expect(memberRowLock(OTHER, VIEWER)).toBe(LOCK_VIEWER)
    expect(memberRowLock(SELF, VIEWER)).toBe(LOCK_VIEWER)
    expect(memberRowLock(OWNER, VIEWER)).toBe(LOCK_VIEWER)
    expect(canChangeMemberRole(OTHER, VIEWER)).toBe(false)
    expect(canRemoveMember(OTHER, VIEWER)).toBe(false)
  })

  it('не отправляет на сервер ни одного адреса', () => {
    expect(addMemberRefusal('stranger@example.com', LIST, VIEWER)).toBe(LOCK_VIEWER)
    expect(addMemberRefusal('owner@example.com', LIST, VIEWER)).toBe(LOCK_VIEWER)
  })

  it('пустое поле остаётся пустым полем, а не отказом', () => {
    expect(addMemberRefusal('', LIST, VIEWER)).toBe(LOCK_NONE)
  })

  it('по умолчанию считает зовущего менеджером', () => {
    // Дрейф в одну безопасную сторону: предложить контрол, который сервер
    // отклонит, — но не спрятать тот, что он бы пропустил.
    expect(memberRowLock(OTHER, { selfId: 'u-self' })).toBe(LOCK_NONE)
    expect(addMemberRefusal('stranger@example.com', LIST, {})).toBe(LOCK_NONE)
  })
})

describe('memberRowBadge', () => {
  // Подпись — не причина отказа: у рядового участника блокировка на всех строках
  // одна, а называться они должны по-разному.
  const VIEWER = { selfId: 'u-other', canManage: false }

  it('называет владельца владельцем, себя — собой, остальных — ролью', () => {
    expect(memberRowBadge(OWNER, VIEWER)).toBe('owner')
    expect(memberRowBadge(OTHER, VIEWER)).toBe('you')
    expect(memberRowBadge(SELF, VIEWER)).toBe('admin')
    expect(memberRowBadge({ user_id: 'u-x', role: 'member' }, VIEWER)).toBe('member')
  })

  it('владелец побеждает «это вы», как и в правиле сервера', () => {
    expect(memberRowBadge(OWNER, { selfId: 'u-owner' })).toBe('owner')
  })
})

describe('addMemberRefusal', () => {
  it('refuses the caller their own address', () => {
    expect(addMemberRefusal('admin@example.com', LIST, WHO)).toBe(LOCK_SELF)
  })

  it('compares addresses the way GetUserByEmail does', () => {
    // strings.ToLower on the server, so case and stray spaces are the same
    // account — a guard that missed this would be no guard at all.
    expect(addMemberRefusal('  ADMIN@Example.com ', LIST, WHO)).toBe(LOCK_SELF)
    expect(addMemberRefusal('owner@example.com', LIST, WHO)).toBe(LOCK_OWNER)
  })

  it('refuses the owner even before the caller', () => {
    expect(addMemberRefusal('Owner@Example.com', LIST, WHO)).toBe(LOCK_OWNER)
  })

  it('recognises the caller with no matching row in the list', () => {
    // ListMembers is paged nowhere today, but the caller's own row is the one
    // thing the dialog can know without it.
    expect(addMemberRefusal('admin@example.com', [OTHER], WHO)).toBe(LOCK_SELF)
  })

  it('lets every other address through, including one already on the list', () => {
    // Re-adding someone else is the documented way a manager changes their role
    // (the CreateMembership upsert) — the guard must not take that away.
    expect(addMemberRefusal('other@example.com', LIST, WHO)).toBe(LOCK_NONE)
    expect(addMemberRefusal('nobody@example.com', LIST, WHO)).toBe(LOCK_NONE)
  })

  it('says nothing about an empty field', () => {
    expect(addMemberRefusal('', LIST, WHO)).toBe(LOCK_NONE)
    expect(addMemberRefusal('   ', LIST, WHO)).toBe(LOCK_NONE)
    expect(addMemberRefusal(null, LIST, WHO)).toBe(LOCK_NONE)
  })

  it('survives a missing list and a missing viewer', () => {
    expect(addMemberRefusal('someone@example.com', null, WHO)).toBe(LOCK_NONE)
    expect(addMemberRefusal('admin@example.com', LIST, {})).toBe(LOCK_NONE)
  })
})
