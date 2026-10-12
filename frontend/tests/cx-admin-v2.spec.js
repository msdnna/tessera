import { describe, it, expect, beforeEach, vi } from 'vitest'
import { reactive, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { i18n } from '@/i18n'

// Admin panel v2 (#3064): «Пространства» and «Вход» columns, «Экспорт CSV» of
// what the list shows, and «Приглашения» of the whole instance with a revoke.

const route = reactive({ query: {} })
const replace = vi.fn((to) => (route.query = to.query))
vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace, push: vi.fn() }),
}))

const DAY = 864e5
const USERS = [
  {
    id: 'u1',
    email: 'anna@x.test',
    name: 'Анна',
    is_admin: true,
    active: true,
    email_verified: true,
    created_at: new Date(Date.now() - 30 * DAY).toISOString(),
    last_seen_at: new Date().toISOString(),
    workspaces: [
      { id: 'w1', name: 'Альфа', role: 'owner' },
      { id: 'w2', name: 'Бета', role: 'member' },
    ],
  },
  {
    id: 'u2',
    email: 'boris@x.test',
    name: 'Борис',
    is_admin: false,
    active: true,
    email_verified: false,
    created_at: new Date(Date.now() - 3 * DAY).toISOString(),
    last_seen_at: null,
    workspaces: [],
  },
]
const INVITES = [
  {
    id: 'i1',
    workspace_id: 'w1',
    workspace_name: 'Альфа',
    email: 'new@x.test',
    role: 'member',
    sent_at: new Date().toISOString(),
    expires_at: new Date(Date.now() + 5 * DAY).toISOString(),
    declined_at: null,
    invited_by_name: 'Анна',
  },
  {
    id: 'i2',
    workspace_id: 'w2',
    workspace_name: 'Бета',
    email: 'old@x.test',
    role: 'admin',
    sent_at: new Date(Date.now() - 9 * DAY).toISOString(),
    expires_at: new Date(Date.now() - DAY).toISOString(),
    declined_at: null,
    invited_by_name: '',
  },
]

const adminApi = vi.hoisted(() => ({
  listUsers: vi.fn(),
  listInvitations: vi.fn(),
  deleteInvitation: vi.fn(() => Promise.resolve({})),
  getOAuth: vi.fn(() => Promise.resolve({ data: {} })),
}))
vi.mock('@/api', async (orig) => ({ ...(await orig()), admin: adminApi }))
const handOff = vi.hoisted(() => vi.fn(() => Promise.resolve('downloaded')))
vi.mock('@/utils/blobHandoff', () => ({ handOffBlob: handOff }))

const { default: AdminView } = await import('@/views/AdminView.vue')

async function open(query = {}) {
  route.query = query
  const w = mount(AdminView, {
    global: { plugins: [i18n], stubs: { AdminHelpFeedback: true, BrandLogo: true } },
    attachTo: document.body,
  })
  await flushPromises()
  await nextTick()
  return w
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  adminApi.listUsers.mockResolvedValue({ data: USERS })
  adminApi.listInvitations.mockResolvedValue({ data: INVITES })
  document.body.innerHTML = ''
})

describe('AdminView v2', () => {
  it('shows the workspaces and the last visit of each user', async () => {
    const w = await open()
    const rows = w.findAll('[data-testid="admin-user"]')
    const anna = rows.find((r) => r.attributes('data-user') === 'anna@x.test')
    const boris = rows.find((r) => r.attributes('data-user') === 'boris@x.test')
    expect(anna.find('[data-testid="admin-user-spaces"]').text()).toBe('Альфа +1')
    expect(anna.find('[data-testid="admin-user-seen"]').text()).not.toBe('—')
    expect(boris.find('[data-testid="admin-user-spaces"]').text()).toBe('—')
    expect(boris.find('[data-testid="admin-user-seen"]').text()).toBe('—')
    w.unmount()
  })

  it('exports what the list shows as CSV', async () => {
    const w = await open()
    await w.find('[data-testid="admin-filter-admins"]').trigger('click')
    await w.find('[data-testid="admin-csv"]').trigger('click')
    expect(handOff).toHaveBeenCalledTimes(1)
    const [blob, name] = handOff.mock.calls[0]
    expect(name).toMatch(/^tessera-users-\d{4}-\d{2}-\d{2}\.csv$/)
    const text = await blob.text()
    expect(text).toContain('anna@x.test')
    expect(text).toContain('Альфа; Бета')
    expect(text).not.toContain('boris@x.test')
    w.unmount()
  })

  it('lists invitations of every workspace and revokes one in two presses', async () => {
    const w = await open({ s: 'invites' })
    let rows = w.findAll('[data-testid="admin-invite-row"]')
    expect(rows.map((r) => r.attributes('data-email'))).toEqual(['new@x.test'])
    await w.find('[data-testid="admin-inv-filter-expired"]').trigger('click')
    rows = w.findAll('[data-testid="admin-invite-row"]')
    expect(rows.map((r) => r.attributes('data-email'))).toEqual(['old@x.test'])
    await w.find('[data-testid="admin-inv-filter-pending"]').trigger('click')
    const revoke = w.find('[data-testid="admin-invite-row"] [data-testid="admin-invite-revoke"]')
    await revoke.trigger('click')
    expect(adminApi.deleteInvitation).not.toHaveBeenCalled()
    await w
      .find('[data-testid="admin-invite-row"] [data-testid^="admin-invite-revoke"]')
      .trigger('click')
    await flushPromises()
    expect(adminApi.deleteInvitation).toHaveBeenCalledWith('i1')
    expect(w.findAll('[data-testid="admin-invite-row"]')).toHaveLength(0)
    w.unmount()
  })
})
