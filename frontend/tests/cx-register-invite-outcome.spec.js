// Регистрация по приглашению — единственная развилка, где токен тратит сервер
// внутри вызова register, а не страница приёма. Ссылку инвайтер часто копирует
// руками (при выключенном SMTP она для этого и возвращается) и присылает в чат,
// так что приглашённый НЕ знает, на какой адрес выписано приглашение, и вводит в
// форме свой обычный. Сервер в таком случае создаёт аккаунт и не даёт членства —
// а ответ регистрации был неотличим от успешного, форма делала push('/'), и
// человек оказывался на доске с личным пространством, без пространства, куда его
// звали, и без единого намёка; единственная копия токена — в том самом сообщении
// (#2932).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import * as naive from 'naive-ui'

vi.mock('@/api', () => ({
  auth: { providers: vi.fn(() => Promise.resolve({ data: { gitlab: false } })) },
  accountFlows: { acceptInvitation: vi.fn() },
  setAccessToken: () => {},
}))

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

const register = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    register,
    login: vi.fn(),
    logout: vi.fn(),
    isAuthenticated: true,
  }),
}))

vi.mock('@/stores/workspaces', () => ({
  useWorkspacesStore: () => ({
    loadWorkspaces: vi.fn(() => Promise.resolve()),
    selectWorkspace: vi.fn(() => Promise.resolve()),
  }),
}))

import RegisterView from '@/views/RegisterView.vue'
import AcceptInviteView from '@/views/AcceptInviteView.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  RouterLink: { template: '<a><slot /></a>' },
}

// Заполняет форму и жмёт «Зарегистрироваться».
async function signUp() {
  const w = mount(RegisterView, { global: { stubs } })
  await flushPromises()
  await w.find('[data-testid="register-name"] input').setValue('Новичок')
  await w.find('[data-testid="register-email"] input').setValue('personal@test.local')
  await w.find('[data-testid="register-password"] input').setValue('password-123')
  await w.find('[data-testid="register-submit"]').trigger('click')
  await flushPromises()
  return w
}

async function openInvite() {
  const w = mount(AcceptInviteView, { global: { stubs } })
  await flushPromises()
  return w
}

describe('регистрация докладывает исход приглашения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    register.mockReset()
    push.mockReset()
    replace.mockReset()
    query = { invite: 'inv-tok' }
  })

  it('промах по адресу уводит на страницу приглашения, а не на доску', async () => {
    register.mockResolvedValue('email_mismatch')
    await signUp()
    expect(replace).toHaveBeenCalledWith({
      path: '/invite',
      query: { token: 'inv-tok', outcome: 'email_mismatch' },
    })
    expect(push).not.toHaveBeenCalledWith('/')
  })

  it('протухший или потраченный токен — тоже не молча', async () => {
    register.mockResolvedValue('invalid')
    await signUp()
    expect(replace).toHaveBeenCalledWith({
      path: '/invite',
      query: { token: 'inv-tok', outcome: 'invalid' },
    })
  })

  it('применённое приглашение ведёт прямо на доску', async () => {
    register.mockResolvedValue('applied')
    await signUp()
    expect(push).toHaveBeenCalledWith('/')
    expect(replace).not.toHaveBeenCalled()
  })

  it('обычная регистрация без приглашения — как раньше', async () => {
    query = {}
    register.mockResolvedValue('')
    await signUp()
    expect(push).toHaveBeenCalledWith('/')
    expect(replace).not.toHaveBeenCalled()
  })

  // Старый бэкенд поля не отдаёт — форма не должна на этом уводить в отказ.
  it('молчание сервера про исход не выдумывает отказ', async () => {
    register.mockResolvedValue(undefined)
    await signUp()
    expect(push).toHaveBeenCalledWith('/')
    expect(replace).not.toHaveBeenCalled()
  })
})

describe('страница приглашения показывает готовый исход', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    replace.mockReset()
    push.mockReset()
  })

  it('промах по адресу назван и лечится сменой аккаунта', async () => {
    query = { token: 'inv-tok', outcome: 'email_mismatch' }
    const w = await openInvite()
    expect(w.text()).toContain('другим аккаунтом')
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(true)
  })

  it('потраченный токен — обычная строка отказа, без ложного лечения', async () => {
    query = { token: 'inv-tok', outcome: 'invalid' }
    const w = await openInvite()
    expect(w.text()).toContain('истекло')
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(false)
  })

  it('сбой применения не выдаётся за истекшее приглашение', async () => {
    query = { token: 'inv-tok', outcome: 'failed' }
    const w = await openInvite()
    expect(w.text()).toContain('Не удалось применить')
  })

  // Приём уже состоялся на сервере: повторный accept ответил бы «уже
  // использовано» про то самое приглашение, которое минуту назад применилось.
  it('с готовым исходом страница не зовёт accept повторно', async () => {
    const { accountFlows } = await import('@/api')
    query = { token: 'inv-tok', outcome: 'email_mismatch' }
    await openInvite()
    expect(accountFlows.acceptInvitation).not.toHaveBeenCalled()
  })

  // Значение приходит из URL: незнакомое (новый сервер, старая сборка) — всё
  // равно отказ, а не пустая страница.
  it('незнакомый исход всё равно объяснён', async () => {
    query = { token: 'inv-tok', outcome: 'что-то новое' }
    const w = await openInvite()
    expect(w.text()).toContain('истекло')
  })
})
