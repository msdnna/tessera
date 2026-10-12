// «Участники»: «Ещё раз» / «Продлить» (#3061) и ссылка-приглашение (#3062).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'

const msg = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}))
vi.mock('@/composables/useMessage', () => ({ useMessage: () => msg }))

const ws = vi.hoisted(() => ({
  members: vi.fn(),
  invitations: vi.fn(),
  inviteLink: vi.fn(),
  createInviteLink: vi.fn(),
  revokeInviteLink: vi.fn(),
  resendInvitation: vi.fn(),
  addMember: vi.fn(),
  createInvitation: vi.fn(),
  removeMember: vi.fn(),
  updateMemberRole: vi.fn(),
  deleteInvitation: vi.fn(),
  // Members v2 (#3063) reads these on open too: an empty queue, no groups.
  accessRequests: vi.fn(() => Promise.resolve({ data: [] })),
  memberGroups: vi.fn(() => Promise.resolve({ data: [] })),
}))
vi.mock('@/api', () => ({ workspaces: ws }))

const copied = vi.hoisted(() => [])
vi.mock('@/utils/clipboard', () => ({
  copyText: vi.fn((s) => {
    copied.push(s)
    return Promise.resolve(true)
  }),
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const tick = async (w) => {
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
}

const SENT = new Date(Date.now() - 2 * 864e5).toISOString()
const LIVE = {
  id: 'i-live',
  email: 'dmitry@tessera.dev',
  role: 'member',
  sent_at: SENT,
  created_at: SENT,
  expires_at: new Date(Date.now() + 5 * 864e5).toISOString(),
}
const DECLINED = { ...LIVE, id: 'i-no', email: 'olga@partner.ru', declined_at: SENT }
const LINK = {
  id: 'l1',
  role: 'member',
  uses: 3,
  max_uses: 25,
  created_at: new Date(Date.now() - 864e5).toISOString(),
  expires_at: new Date(Date.now() + 6 * 864e5 - 60000).toISOString(),
  state: 'pending',
}

async function open({ invites = [LIVE], link = null } = {}) {
  ws.members.mockResolvedValue({ data: [] })
  ws.invitations.mockResolvedValue({ data: invites })
  ws.inviteLink.mockResolvedValue({ data: { link } })
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().list = [{ id: 'ws-1', my_role: 'admin' }]
  const MembersModal = (await import('@/components/MembersModal.vue')).default
  const w = mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
  await w.setProps({ show: true })
  await tick(w)
  return w
}

const rowOf = (w, email) => w.findAll('.invite-row').find((r) => r.text().includes(email))

describe('MembersModal: повтор приглашения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    copied.length = 0
  })

  it('ждущее — «Ещё раз» и когда отправлено; отклонённое — «Продлить»', async () => {
    const w = await open({ invites: [LIVE, DECLINED] })
    const live = rowOf(w, 'dmitry@tessera.dev')
    expect(live.find('.inv-sent').text()).toContain('отправлено')
    expect(live.find('[data-testid="invite-resend"]').text()).toBe('Ещё раз')
    const no = rowOf(w, 'olga@partner.ru')
    expect(no.find('[data-testid="invite-declined"]').exists()).toBe(true)
    expect(no.find('[data-testid="invite-resend"]').text()).toBe('Продлить')
    // Отклонённое не считается «ждёт ответа».
    expect(w.text()).toContain('1 приглашение ждёт ответа')
  })

  it('«Ещё раз» шлёт заново и даёт скопировать новую ссылку', async () => {
    ws.resendInvitation.mockResolvedValue({ data: { ...LIVE, link: 'http://t/invite?token=new' } })
    const w = await open()
    await rowOf(w, 'dmitry@tessera.dev').find('[data-testid="invite-resend"]').trigger('click')
    await tick(w)
    expect(ws.resendInvitation).toHaveBeenCalledWith('ws-1', 'i-live')
    expect(msg.success).toHaveBeenCalled()
    expect(w.find('[data-testid="members-invite-link"]').text()).toBe('http://t/invite?token=new')
  })

  it('слишком рано — спокойная подсказка, не ошибка', async () => {
    const e = new Error('x')
    e.status = 429
    ws.resendInvitation.mockRejectedValue(e)
    const w = await open()
    await rowOf(w, 'dmitry@tessera.dev').find('[data-testid="invite-resend"]').trigger('click')
    await tick(w)
    expect(msg.info).toHaveBeenCalled()
    expect(msg.error).not.toHaveBeenCalled()
  })
})

describe('MembersModal: ссылка-приглашение', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    copied.length = 0
  })

  it('нет ссылки — «Создать»; созданная показывается и копируется', async () => {
    ws.createInviteLink.mockResolvedValue({
      data: { link: { ...LINK, uses: 0, url: 'https://tessera.dev/join/abc' } },
    })
    const w = await open()
    await w.find('[data-testid="members-shared-create"]').trigger('click')
    await tick(w)
    expect(ws.createInviteLink).toHaveBeenCalledWith('ws-1', {
      role: 'member',
      ttl_days: 7,
      max_uses: 25,
    })
    expect(w.find('[data-testid="members-shared-url"]').text()).toBe('tessera.dev/join/abc')
    expect(w.find('[data-testid="members-shared-meta"]').text()).toContain('Участник · 6 дней')
    await w.find('[data-testid="members-shared-copy"]').trigger('click')
    await tick(w)
    expect(copied).toEqual(['https://tessera.dev/join/abc'])
  })

  it('существующая ссылка после переоткрытия скрыта: копировать нечего, «Сбросить» выдаёт новую', async () => {
    ws.createInviteLink.mockResolvedValue({
      data: { link: { ...LINK, id: 'l2', uses: 0, url: 'https://tessera.dev/join/new' } },
    })
    const w = await open({ link: LINK })
    expect(w.find('[data-testid="members-shared-hidden"]').exists()).toBe(true)
    expect(w.find('[data-testid="members-shared-copy"]').exists()).toBe(false)
    await w.find('[data-testid="members-shared-reset"]').trigger('click')
    await tick(w)
    expect(ws.createInviteLink).toHaveBeenCalledWith('ws-1', {
      role: 'member',
      ttl_days: 7,
      max_uses: 25,
    })
    expect(w.find('[data-testid="members-shared-url"]').text()).toBe('tessera.dev/join/new')
  })
})
