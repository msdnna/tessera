import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setI18nLocale } from '@/i18n'
import { isReviewColumn, columnStatusName } from '@/utils/columnStatus'
import ColumnHeader from '@/components/ColumnHeader.vue'
import MembersModal from '@/components/MembersModal.vue'
import ConfirmByName from '@/components/ConfirmByName.vue'

// Wave 8 of #2799 — the app shell (sidebar, layout, workspace tools, column
// header). The option tables here live inside dropdowns and selects, which is
// the worst place for a frozen label: the popup keeps the language of the first
// render and nothing looks broken until someone opens it after a switch.

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// The API module is only reached by the modal's load(), which never runs here
// (the modal is mounted closed).
vi.mock('@/api', () => ({
  workspaces: {
    members: vi.fn(),
    invitations: vi.fn(),
    addMember: vi.fn(),
    createInvitation: vi.fn(),
    removeMember: vi.fn(),
    updateMemberRole: vi.fn(),
    deleteInvitation: vi.fn(),
  },
  columns: { update: vi.fn(), remove: vi.fn() },
}))

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('column header', () => {
  const dcol = { key: 'c1', name: 'В процессе', color: '' }

  it('re-renders the context menu after a switch', async () => {
    const w = mount(ColumnHeader, { props: { dcol, editable: true } })
    expect(w.vm.ctxOptions.map((o) => o.label).filter(Boolean)).toEqual([
      'Переименовать',
      'Сделать завершающей',
      'Удалить колонку',
    ])

    await setI18nLocale('en')
    expect(w.vm.ctxOptions.map((o) => o.label).filter(Boolean)).toEqual([
      'Rename',
      'Make it completing',
      'Delete the column',
    ])
    // The keys drive onCtxSelect; they are wiring, not text.
    expect(w.vm.ctxOptions.map((o) => o.key)).toEqual(['rename', 'done', 'd1', 'delete'])
  })

  it('flips the done entry with the column state, in either language', async () => {
    const done = mount(ColumnHeader, { props: { dcol, editable: true, isDone: true } })
    expect(done.vm.ctxOptions[1].label).toBe('Снять завершение')

    await setI18nLocale('en')
    expect(done.vm.ctxOptions[1].label).toBe('Stop completing')
  })
})

describe('column status glyph', () => {
  // Column names are seeded by the server in Russian and are user data — the
  // heuristic has to keep matching them whatever the interface language is.
  it('recognises a review column regardless of the UI locale', async () => {
    expect(isReviewColumn('На рассмотрении')).toBe(true)
    expect(isReviewColumn('Review')).toBe(true)
    expect(isReviewColumn('Готово')).toBe(false)

    await setI18nLocale('en')
    expect(isReviewColumn('На рассмотрении')).toBe(true)
    expect(columnStatusName({ name: 'На рассмотрении' })).toBe('status-review')
    expect(columnStatusName({ isDone: true, name: 'На рассмотрении' })).toBe('status-done')
    expect(columnStatusName({ first: true, name: 'К работе' })).toBe('status-todo')
  })
})

describe('members modal', () => {
  it('re-renders the role options after a switch', async () => {
    const w = mount(MembersModal, { props: { show: false, wsId: 'w1' } })
    expect(w.vm.roleOptions.map((o) => o.label)).toEqual(['Участник', 'Админ'])

    await setI18nLocale('en')
    expect(w.vm.roleOptions.map((o) => o.label)).toEqual(['Member', 'Admin'])
    // Roles are the wire format the server checks; they never move.
    expect(w.vm.roleOptions.map((o) => o.value)).toEqual(['member', 'admin'])
  })
})

describe('confirm-by-name dialog', () => {
  // The title and the confirm button used to default to Russian literals in the
  // prop declaration. A literal default is never empty, so a fallback to the
  // catalogue inside the template could not fire: every caller that relied on
  // the default would keep a Russian dialog in an English interface.
  it('falls back to the catalogue when the caller passes no wording', async () => {
    const w = mount(ConfirmByName, { props: { show: true, name: 'Alpha' } })
    expect(w.vm.cardTitle).toBe('Подтвердите удаление')
    expect(w.vm.confirmLabel).toBe('Удалить')

    await setI18nLocale('en')
    expect(w.vm.cardTitle).toBe('Confirm deletion')
    expect(w.vm.confirmLabel).toBe('Delete')
  })

  it('still prefers what the caller passed', async () => {
    const w = mount(ConfirmByName, {
      props: { show: true, name: 'Alpha', title: 'Удалить пространство' },
    })
    expect(w.vm.cardTitle).toBe('Удалить пространство')
  })
})
