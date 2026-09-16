// Модалка участников живёт в дереве WorkspaceTools постоянно — её закрывает
// проп `show`, а `wsId` ходит за сайдбаром. Поэтому ростер, список приглашений
// и коробка со ссылкой-инвайтом, лежащие в ref'ах setup'а, переживают смену
// пространства: открыв диалог в другом, видишь данные предыдущего, пока не
// приедет ответ. Упавший запрос не приезжает никогда — и чужой список остаётся
// на экране, выдавая себя за этот. Ссылка тут хуже всех: это носитель токена
// приглашения в ТО пространство (#2932).
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
const invitations = vi.fn()
vi.mock('@/api', () => ({
  workspaces: {
    members: (...a) => members(...a),
    invitations: (...a) => invitations(...a),
    addMember: vi.fn(),
    createInvitation: vi.fn(),
    removeMember: vi.fn(),
    updateMemberRole: vi.fn(),
    deleteInvitation: vi.fn(() => Promise.resolve({ data: {} })),
  },
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const ALICE = { user_id: 'u-a', name: 'Алиса', email: 'alice@example.com', role: 'admin' }
const BOB = { user_id: 'u-b', name: 'Боб', email: 'bob@example.com', role: 'member' }
const INVITE_WS1 = {
  id: 'i-1',
  email: 'pending@example.com',
  role: 'member',
  expires_at: '2126-09-20T12:00:00Z',
}

const flush = async (w) => {
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
}

// Оба пространства под управлением: приглашения и форма стоят за requireManager,
// и без роли менеджера половина диалога просто не рисуется.
const mountClosed = async () => {
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().list = [
    { id: 'ws-1', my_role: 'admin' },
    { id: 'ws-2', my_role: 'admin' },
  ]
  const MembersModal = (await import('@/components/MembersModal.vue')).default
  return mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
}

describe('MembersModal: смена пространства', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('не оставляет ростер и приглашения прошлого пространства, когда новый запрос упал', async () => {
    members.mockResolvedValue({ data: [ALICE, BOB] })
    invitations.mockResolvedValue({ data: [INVITE_WS1] })
    const w = await mountClosed()
    await w.setProps({ show: true })
    await flush(w)
    expect(w.text()).toContain('alice@example.com')
    expect(w.text()).toContain('pending@example.com')

    // Закрыли, переключились, открыли — и на этот раз сервер отказал.
    await w.setProps({ show: false })
    members.mockRejectedValue(Object.assign(new Error('Нет связи'), { status: 500 }))
    invitations.mockRejectedValue(Object.assign(new Error('Нет связи'), { status: 500 }))
    await w.setProps({ wsId: 'ws-2' })
    await w.setProps({ show: true })
    await flush(w)

    expect(w.text()).not.toContain('alice@example.com')
    expect(w.text()).not.toContain('bob@example.com')
    expect(w.findAll('.invite-row')).toHaveLength(0)
  })

  it('не переносит ссылку-приглашение в другое пространство', async () => {
    members.mockResolvedValue({ data: [ALICE] })
    invitations.mockResolvedValue({ data: [] })
    const w = await mountClosed()
    await w.setProps({ show: true })
    await flush(w)

    const wsApi = (await import('@/api')).workspaces
    wsApi.addMember.mockRejectedValue(
      Object.assign(new Error('нет такого пользователя'), { raw: 'no user with that email' }),
    )
    wsApi.createInvitation.mockResolvedValue({ data: { link: 'https://t/invite?token=secret' } })
    await w.find('.invite input').setValue('stranger@example.com')
    await w.findAll('.invite button')[0].trigger('click')
    await flush(w)
    expect(w.find('.link-box').exists()).toBe(true)

    await w.setProps({ show: false })
    await w.setProps({ wsId: 'ws-2' })
    await w.setProps({ show: true })
    await flush(w)

    expect(w.find('.link-box').exists()).toBe(false)
  })

  it('поздний ответ прошлого пространства не дописывается в открытый диалог', async () => {
    // Диалог переоткрыли на другом пространстве, пока первый запрос ещё шёл:
    // без проверки «это всё ещё тот wsId?» ответ ws-1 перезаписал бы ростер ws-2.
    members.mockImplementation((id) =>
      id === 'ws-1'
        ? new Promise((r) => setTimeout(() => r({ data: [ALICE] }), 20))
        : Promise.resolve({ data: [BOB] }),
    )
    invitations.mockResolvedValue({ data: [] })
    const w = await mountClosed()
    await w.setProps({ show: true })
    await w.setProps({ wsId: 'ws-2' })
    await new Promise((r) => setTimeout(r, 40))
    await w.vm.$nextTick()

    expect(w.text()).toContain('bob@example.com')
    expect(w.text()).not.toContain('alice@example.com')
  })
})
