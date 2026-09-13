// Приглашение живёт неделю (backend/handlers/invitations.go invitationTTL), но
// список «Приглашения» в модалке участников фильтрует только accepted_at —
// истёкшее остаётся в выдаче ListInvitations навсегда. Пути приёма при этом
// истёкшие отбрасывают все до одного (AcceptInvitation, acceptInvitationToken,
// ListPendingInvitationsByEmail), так что строка без срока врёт администратору:
// он видит «пригласил, ещё не принял», а по ссылке приходит «invalid or expired
// invitation». Уникального ключа на (workspace_id, email) нет — повторные
// приглашения копятся, и отличить живое от мёртвого в списке нечем.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'

// useMessage() падает без <n-message-provider>; глушим только его, как и
// остальные спеки компонентов.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const invitations = vi.fn()
vi.mock('@/api', () => ({
  workspaces: {
    members: vi.fn(() => Promise.resolve({ data: [] })),
    invitations: (...a) => invitations(...a),
    addMember: vi.fn(),
    createInvitation: vi.fn(),
    removeMember: vi.fn(),
    updateMemberRole: vi.fn(),
    deleteInvitation: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

// n-modal телепортирует тело наружу — держим его внутри обёртки, как в
// cx-smoke-components.
const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

// Полдень UTC: дата совпадает в любой правдоподобной зоне прогона.
const LIVE = {
  id: 'i-live',
  email: 'live@example.com',
  role: 'member',
  expires_at: '2126-09-20T12:00:00Z',
  created_at: '2126-09-13T12:00:00Z',
}
const DEAD = {
  id: 'i-dead',
  email: 'dead@example.com',
  role: 'admin',
  expires_at: '2020-01-10T12:00:00Z',
  created_at: '2020-01-03T12:00:00Z',
}

// Загрузка висит на watch без immediate — модалка монтируется закрытой и
// открывается сменой пропа, ровно как в оболочке.
const open = async (rows) => {
  invitations.mockResolvedValue({ data: rows })
  const MembersModal = (await import('@/components/MembersModal.vue')).default
  const w = mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
  await w.setProps({ show: true })
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
  return w
}

const rowOf = (w, email) => w.findAll('.invite-row').find((r) => r.text().includes(email))

describe('MembersModal: срок приглашения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('помечает истёкшее приглашение, чтобы оно не читалось как «ждёт ответа»', async () => {
    const w = await open([LIVE, DEAD])
    const dead = rowOf(w, 'dead@example.com')
    expect(dead).toBeTruthy()
    expect(dead.find('.inv-expired').exists()).toBe(true)
    expect(dead.text()).toContain('Истекло')
  })

  it('у живого приглашения показывает, до какого числа оно действует', async () => {
    const w = await open([LIVE, DEAD])
    const live = rowOf(w, 'live@example.com')
    expect(live.find('.inv-expired').exists()).toBe(false)
    // Разделитель даёт useFormat по языку профиля — здесь важно само число.
    expect(live.find('.inv-until').text()).toMatch(/20[./]09[./]2126/)
  })

  // Истёкшая строка — единственный способ убрать мусор из списка: отзыв обязан
  // остаться и на ней.
  it('оставляет отзыв доступным и на истёкшем приглашении', async () => {
    const w = await open([DEAD])
    const dead = rowOf(w, 'dead@example.com')
    expect(dead.findAll('button').length).toBeGreaterThan(0)
  })
})
