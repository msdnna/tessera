// Кнопка «Участники» в панели инструментов показана ВСЕМ, и это намеренно:
// ListMembers стоит за requireMember — состав пространства вправе прочитать
// любой его участник. А вот сама модалка была написана так, будто её открывает
// только менеджер:
//
//   1) load() тянул участников и приглашения одним Promise.all, но
//      ListInvitations — уже requireManager. Рядовой участник получал 403 на
//      втором запросе, Promise.all реджектился, и список участников, который
//      сервер УЖЕ отдал, выбрасывался: пустой диалог за красным тостом.
//   2) Даже загрузись он — селект роли, корзина и форма приглашения вели на
//      AddMember/UpdateMemberRole/RemoveMember, каждая за requireManager. Три
//      контрола, отвечающие 403.
//
// Канон соседних фич: скрытая кнопка — неудобство, кнопка с 403 — баг-репорт
// (#2932).
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
    addMember: vi.fn(() => Promise.resolve({ data: {} })),
    createInvitation: vi.fn(),
    removeMember: vi.fn(),
    updateMemberRole: vi.fn(),
    deleteInvitation: vi.fn(),
  },
}))

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
}

const OWNER = { user_id: 'u-owner', name: 'Владелец', email: 'owner@example.com', role: 'owner' }
const ADMIN = { user_id: 'u-admin', name: 'Админ', email: 'admin@example.com', role: 'admin' }
const ME = { user_id: 'u-me', name: 'Я рядовой', email: 'me@example.com', role: 'member' }
const ROWS = [OWNER, ADMIN, ME]

// Отказ сервера в том виде, в каком его отдаёт слой @/api: requireManager.
const forbidden = () => Promise.reject(Object.assign(new Error('forbidden'), { status: 403 }))

const open = async (myRole) => {
  members.mockResolvedValue({ data: ROWS })
  // Рядовому участнику сервер на этот запрос отвечает 403 — мок обязан врать
  // так же, иначе тест проверяет не тот мир.
  invitations.mockImplementation(() =>
    myRole === 'member' ? forbidden() : Promise.resolve({ data: [] }),
  )
  const { useAuthStore } = await import('@/stores/auth')
  useAuthStore().setUser({ id: ME.user_id, email: ME.email, name: ME.name })
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().list = [{ id: 'ws-1', my_role: myRole }]
  const MembersModal = (await import('@/components/MembersModal.vue')).default
  const w = mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
  await w.setProps({ show: true })
  await new Promise((r) => setTimeout(r, 0))
  await w.vm.$nextTick()
  return w
}

const rowOf = (w, name) => w.findAll('.member').find((r) => r.text().includes(name))

describe('MembersModal глазами рядового участника', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('показывает состав пространства, хотя приглашения ему запрещены', async () => {
    // Собственно регрессия: 403 на приглашениях больше не уносит с собой список
    // участников, ради которого модалку и открывали.
    const w = await open('member')
    expect(w.findAll('.member').length).toBe(3)
    expect(w.text()).toContain('Владелец')
    expect(w.text()).toContain('me@example.com')
  })

  it('не ходит за приглашениями, раз сервер их всё равно не отдаст', async () => {
    const w = await open('member')
    expect(invitations).not.toHaveBeenCalled()
    expect(w.findAll('.invite-row').length).toBe(0)
  })

  it('не предлагает ни смену роли, ни удаление ни на одной строке', async () => {
    const w = await open('member')
    for (const row of w.findAll('.member')) {
      expect(row.find('[data-testid="member-role"]').exists()).toBe(false)
      expect(row.findAll('button').length).toBe(0)
    }
  })

  it('прячет форму приглашения целиком', async () => {
    const w = await open('member')
    expect(w.find('.invite').exists()).toBe(false)
  })

  it('говорит, почему управления нет, а не молчит', async () => {
    // Исчезнувший без слова контрол читается как поломка — тот же канон, что у
    // плашек «Владелец» и «Это вы».
    const w = await open('member')
    expect(w.text()).toContain('только для чтения')
  })

  it('строки продолжают называть себя: владелец, роль, и «это вы» про себя', async () => {
    // Блокировка у всех трёх строк одна и та же (requireManager), но подпись —
    // не причина отказа: роспись «Участник» на всех строках сказала бы читателю
    // меньше, чем он знал до фикса.
    const w = await open('member')
    expect(rowOf(w, 'Владелец').text()).toContain('Владелец')
    expect(rowOf(w, 'Админ').text()).toContain('Админ')
    expect(rowOf(w, 'Я рядовой').find('.mw-you').exists()).toBe(true)
  })
})

describe('MembersModal глазами менеджера', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('оставляет админу и форму, и управление чужими строками', async () => {
    const w = await open('admin')
    expect(w.find('.invite').exists()).toBe(true)
    expect(invitations).toHaveBeenCalledTimes(1)
    const other = rowOf(w, 'Админ') // не он сам: сам он здесь ME с ролью member
    expect(other.find('[data-testid="member-role"]').exists()).toBe(true)
  })

  it('упавшие приглашения больше не уносят список участников и у менеджера', async () => {
    // Сеть моргнула на одном из двух запросов — второй список всё равно виден.
    members.mockResolvedValue({ data: ROWS })
    invitations.mockImplementation(() => Promise.reject(new Error('network')))
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    useWorkspacesStore().list = [{ id: 'ws-1', my_role: 'admin' }]
    const MembersModal = (await import('@/components/MembersModal.vue')).default
    const w = mount(MembersModal, { props: { show: false, wsId: 'ws-1' }, global: { stubs } })
    await w.setProps({ show: true })
    await new Promise((r) => setTimeout(r, 0))
    await w.vm.$nextTick()
    expect(w.findAll('.member').length).toBe(3)
  })
})
