import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { setI18nLocale } from '@/i18n'
import { isReviewColumn, columnStatusName } from '@/utils/columnStatus'
import ColumnHeader from '@/components/ColumnHeader.vue'
import MembersModal from '@/components/MembersModal.vue'

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

  // The v2 column menu (#2969): ActionMenu rows built in a computed, so their
  // labels follow a language switch.
  const labels = (w) => w.vm.menuItems.filter((o) => !o.gap).map((o) => o.label)
  it('re-renders the column menu after a switch', async () => {
    const w = mount(ColumnHeader, { props: { dcol, editable: true } })
    expect(labels(w)).toEqual([
      'Переименовать',
      'Цвет',
      'Колонка «Готово»',
      'Свернуть',
      'Сдвинуть влево',
      'Сдвинуть вправо',
      'Новая колонка справа',
      'Удалить колонку',
    ])

    await setI18nLocale('en')
    expect(labels(w)[0]).toBe('Rename')
    expect(labels(w).at(-1)).toBe('Delete the column')
    // The keys drive onMenu; they are wiring, not text.
    expect(w.vm.menuItems.filter((o) => !o.gap).map((o) => o.key)).toEqual([
      'rename',
      'color',
      'done',
      'collapse',
      'moveLeft',
      'moveRight',
      'addRight',
      'delete',
    ])
  })

  it('the done switch follows the column state, in either language', async () => {
    const done = mount(ColumnHeader, { props: { dcol, editable: true, isDone: true } })
    const row = () => done.vm.menuItems.find((o) => o.key === 'done')
    expect(row().toggle).toBe(true)
    expect(row().label).toBe('Колонка «Готово»')

    await setI18nLocale('en')
    expect(row().label).toBe('“Done” column')
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
  // The invitation rows render their expiry through useFormat, so the modal now
  // reads the profile's date preferences off the theme store.
  beforeEach(() => setActivePinia(createPinia()))

  it('re-renders the role options after a switch', async () => {
    const w = mount(MembersModal, { props: { show: false, wsId: 'w1' } })
    expect(w.vm.roleOptions.map((o) => o.label)).toEqual(['Участник', 'Админ'])

    await setI18nLocale('en')
    expect(w.vm.roleOptions.map((o) => o.label)).toEqual(['Member', 'Admin'])
    // Roles are the wire format the server checks; they never move.
    expect(w.vm.roleOptions.map((o) => o.value)).toEqual(['member', 'admin'])
  })
})
