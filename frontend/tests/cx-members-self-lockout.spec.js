// Модалка участников не знала, кто в неё смотрит: пропы — только show и wsId,
// а строка рисовалась по одному признаку `m.role === 'owner'`. Значит на СВОЕЙ
// строке администратор видел тот же селект роли и ту же корзину, что и на чужой,
// и одним нажатием снимал с себя управление пространством — вернуть его может
// только другой админ или владелец. Тот же самообстрел проходил через поле
// «Пригласить по email»: CreateMembership — upsert, так что собственный адрес с
// ролью «Участник» по умолчанию понижал автора, и модалка отвечала зелёным
// «Участник добавлен» (#2932).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const members = vi.fn()
vi.mock('@/api', () => ({
  workspaces: {
    members: (...a) => members(...a),
    invitations: vi.fn(() => Promise.resolve({ data: [] })),
    addMember: vi.fn(() => Promise.resolve({ data: {} })),
    createInvitation: vi.fn(),
    removeMember: vi.fn(() => Promise.resolve({ data: {} })),
    updateMemberRole: vi.fn(() => Promise.resolve({ data: {} })),
    deleteInvitation: vi.fn(),
  },
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const OWNER = { user_id: 'u-owner', name: 'Владелец', email: 'owner@example.com', role: 'owner' }
const SELF = { user_id: 'u-self', name: 'Я сам', email: 'admin@example.com', role: 'admin' }
const OTHER = { user_id: 'u-other', name: 'Коллега', email: 'other@example.com', role: 'member' }

// Все правила ниже — про менеджера: рядовому участнику сервер отказывает раньше,
// на requireManager, и модалка теперь это отражает (см. cx-members-viewer).
const open = async (rows = [OWNER, SELF, OTHER], me = SELF) => {
  members.mockResolvedValue({ data: rows })
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser(me ? { id: me.user_id, email: me.email, name: me.name } : null)
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().list = [{ id: 'ws-1', my_role: me === OWNER ? 'owner' : 'admin' }]
  const MembersModal = (await import('@/components/MembersModal.vue')).default
  const w = mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
  await w.setProps({ show: true })
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
  return w
}

const rowOf = (w, name) => w.findAll('.member').find((r) => r.text().includes(name))

describe('MembersModal: своя строка', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('не даёт сменить себе роль', async () => {
    const w = await open()
    const mine = rowOf(w, 'Я сам')
    expect(mine).toBeTruthy()
    expect(mine.find('[data-testid="member-role"]').exists()).toBe(false)
  })

  it('не даёт удалить себя', async () => {
    const w = await open()
    expect(rowOf(w, 'Я сам').findAll('button').length).toBe(0)
  })

  it('называет причину, а не просто прячет кнопки', async () => {
    // Пропавший без объяснения элемент читается как поломка интерфейса; строка
    // должна сказать, что это правило, а не сбой.
    const w = await open()
    // v2 (#2975): the row keeps its role, marks «вы» and says why on hover.
    expect(rowOf(w, 'Я сам').text()).toContain('вы')
    expect(
      rowOf(w, 'Я сам').find('[data-testid="member-role-locked"]').attributes('title'),
    ).toContain('Себя')
  })

  it('оставляет управление чужими строками нетронутым', async () => {
    const w = await open()
    const other = rowOf(w, 'Коллега')
    expect(other.find('[data-testid="member-role"]').exists()).toBe(true)
    expect(other.findAll('button').length).toBeGreaterThan(0)
  })

  it('владелец остаётся владельцем, а не «это вы»', async () => {
    // Порядок правил сервера: сначала владелец. Владелец, открывший модалку,
    // должен видеть прежнюю плашку «Владелец».
    const w = await open([OWNER, SELF, OTHER], OWNER)
    const ownerRow = rowOf(w, 'Владелец')
    expect(ownerRow.text()).toContain('Владелец')
    // v2 (#2975): «вы» rides by the name, the pill keeps the role and the
    // reason is the owner rule, not the self one.
    const pill = ownerRow.find('[data-testid="member-role-locked"]')
    expect(pill.text()).toContain('Владелец')
    expect(pill.attributes('title')).toContain('Владельца')
  })
})

describe('MembersModal: свой адрес в поле приглашения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  const api = async () => (await import('@/api')).workspaces

  const typeAndInvite = async (w, mail) => {
    await w.find('.invite input').setValue(mail)
    await w.find('[data-testid="members-invite-submit"]').trigger('click')
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
  }

  it('не отправляет собственный адрес на сервер', async () => {
    const w = await open()
    const wsApi = await api()
    await typeAndInvite(w, 'admin@example.com')
    expect(wsApi.addMember).not.toHaveBeenCalled()
    expect(wsApi.createInvitation).not.toHaveBeenCalled()
  })

  it('объясняет отказ прямо под полем', async () => {
    const w = await open()
    await typeAndInvite(w, '  ADMIN@Example.com ')
    expect(w.find('.invite-err').text()).toContain('Себя')
  })

  it('не отправляет адрес владельца', async () => {
    const w = await open()
    const wsApi = await api()
    await typeAndInvite(w, 'owner@example.com')
    expect(wsApi.addMember).not.toHaveBeenCalled()
    expect(w.find('.invite-err').exists()).toBe(true)
  })

  it('чужой адрес по-прежнему уезжает на сервер', async () => {
    const w = await open()
    const wsApi = await api()
    await typeAndInvite(w, 'other@example.com')
    expect(wsApi.addMember).toHaveBeenCalledTimes(1)
    expect(w.find('.invite-err').exists()).toBe(false)
  })

  it('и незнакомый адрес тоже', async () => {
    const w = await open()
    const wsApi = await api()
    await typeAndInvite(w, 'stranger@example.com')
    expect(wsApi.addMember).toHaveBeenCalledTimes(1)
  })
})
