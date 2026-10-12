// Превью приглашения и «Отклонить» (#3059), ссылка-приглашение на той же
// странице (#3062), срок ссылки сброса (#3060).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import * as naive from 'naive-ui'

const api = vi.hoisted(() => ({
  lookupInvitation: vi.fn(),
  declineInvitation: vi.fn(),
  acceptInvitation: vi.fn(),
  checkResetToken: vi.fn(),
  resetPassword: vi.fn(),
}))
vi.mock('@/api', () => ({ accountFlows: api }))

const push = vi.fn()
const replace = vi.fn()
let query = {}
vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query }),
    useRouter: () => ({ push, replace }),
  }
})

let authed = false
let me = 'maria@tessera.dev'
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return authed
    },
    get user() {
      return authed ? { email: me } : null
    },
    logout: vi.fn(),
  }),
}))
vi.mock('@/stores/workspaces', () => ({
  useWorkspacesStore: () => ({
    loadWorkspaces: vi.fn(() => Promise.resolve()),
    selectWorkspace: vi.fn(() => Promise.resolve()),
  }),
}))

import AcceptInviteView from '@/views/AcceptInviteView.vue'
import ResetPasswordView from '@/views/ResetPasswordView.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' },
}

const PREVIEW = {
  kind: 'email',
  state: 'pending',
  role: 'member',
  email: 'maria@tessera.dev',
  expires_at: '2126-01-01T12:00:00Z',
  workspace: { name: 'Студия «Север»', name_key: null, color: '#2f80ed' },
  projects: 4,
  members: 9,
  inviter: { name: 'Илья Петров' },
}

async function open(view = AcceptInviteView) {
  const w = mount(view, { global: { stubs } })
  await flushPromises()
  return w
}

describe('превью приглашения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach((f) => f.mockReset())
    push.mockReset()
    replace.mockReset()
    authed = false
    me = 'maria@tessera.dev'
    query = { token: 'tok' }
    api.lookupInvitation.mockResolvedValue({ data: { ...PREVIEW } })
  })

  it('показывает, кто зовёт, куда и с какой ролью — как в макете', async () => {
    const w = await open()
    const p = w.find('[data-testid="invite-preview"]')
    expect(p.exists()).toBe(true)
    expect(p.text()).toContain('Илья Петров приглашает в')
    expect(w.find('[data-testid="invite-space"]').text()).toBe('Студия «Север»')
    expect(p.text()).toContain('4 проекта')
    expect(p.text()).toContain('9 участников')
    expect(w.find('[data-testid="invite-role"]').text()).toBe('роль: участник')
    expect(w.find('[data-testid="invite-email"]').text()).toContain('maria@tessera.dev')
    expect(p.text()).toContain('СС')
  })

  it('без аккаунта «Принять» ведёт на регистрацию с токеном, «Войти» — на вход с ним же', async () => {
    const w = await open()
    await w.find('[data-testid="invite-accept"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/register', query: { invite: 'tok' } })
    expect(api.acceptInvitation).not.toHaveBeenCalled()
    const login = JSON.parse(w.find('[data-testid="invite-login"]').attributes('data-to'))
    expect(login).toEqual({ path: '/login', query: { next: '/invite?token=tok' } })
  })

  it('«Отклонить» гасит приглашение', async () => {
    api.declineInvitation.mockResolvedValue({})
    const w = await open()
    await w.find('[data-testid="invite-decline"]').trigger('click')
    await flushPromises()
    expect(api.declineInvitation).toHaveBeenCalledWith('tok')
    expect(w.find('[data-testid="invite-state"]').text()).toContain('больше не действует')
  })

  it('чужой аккаунт виден до нажатия — вместо «Принять» «Войти под другим»', async () => {
    authed = true
    me = 'personal@example.com'
    const w = await open()
    expect(w.find('[data-testid="invite-mismatch"]').text()).toContain('personal@example.com')
    expect(w.find('[data-testid="invite-accept"]').exists()).toBe(false)
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(true)
  })

  it('свой аккаунт принимает с первой кнопки', async () => {
    authed = true
    api.acceptInvitation.mockResolvedValue({ data: { id: 'ws-1' } })
    const w = await open()
    await w.find('[data-testid="invite-accept"]').trigger('click')
    await flushPromises()
    expect(api.acceptInvitation).toHaveBeenCalledWith('tok')
    expect(w.text()).toContain('присоединились')
  })

  it('уже отклонённое и неизвестное — без превью', async () => {
    api.lookupInvitation.mockResolvedValue({ data: { kind: 'email', state: 'declined' } })
    let w = await open()
    expect(w.find('[data-testid="invite-preview"]').exists()).toBe(false)
    expect(w.text()).toContain('Приглашение отклонено')
    const e = new Error('x')
    e.status = 404
    api.lookupInvitation.mockRejectedValue(e)
    w = await open()
    expect(w.text()).toContain('Приглашение не действует')
  })

  it('ссылка-приглашение: без адреса, «Отклонить» просто уходит', async () => {
    api.lookupInvitation.mockResolvedValue({
      data: { ...PREVIEW, kind: 'link', email: undefined },
    })
    const w = await open()
    expect(w.find('[data-testid="invite-email"]').exists()).toBe(false)
    expect(w.text()).toContain('Ссылка-приглашение')
    await w.find('[data-testid="invite-decline"]').trigger('click')
    expect(api.declineInvitation).not.toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith('/login')
  })
})

describe('срок ссылки сброса пароля', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach((f) => f.mockReset())
    query = { token: 'reset-tok' }
  })

  it('«Ссылка действует ещё 54 минуты»', async () => {
    api.checkResetToken.mockResolvedValue({
      data: { expires_at: new Date(Date.now() + 54 * 60000 - 5000).toISOString() },
    })
    const w = await open(ResetPasswordView)
    expect(api.checkResetToken).toHaveBeenCalledWith('reset-tok')
    expect(w.text()).toContain('Ссылка действует ещё 54 минуты')
    expect(w.find('[data-testid="reset-password"]').exists()).toBe(true)
  })

  it('мёртвая ссылка сказана до того, как введён пароль', async () => {
    const e = new Error('x')
    e.status = 400
    api.checkResetToken.mockRejectedValue(e)
    const w = await open(ResetPasswordView)
    expect(w.text()).toContain('Ссылка устарела')
    expect(w.find('[data-testid="reset-password"]').exists()).toBe(false)
    expect(w.find('[data-testid="reset-request-new"]').exists()).toBe(true)
  })

  it('недоступный сервер не хоронит ссылку — форма остаётся', async () => {
    const e = new Error('x')
    e.offline = true
    api.checkResetToken.mockRejectedValue(e)
    const w = await open(ResetPasswordView)
    expect(w.find('[data-testid="reset-password"]').exists()).toBe(true)
  })
})
