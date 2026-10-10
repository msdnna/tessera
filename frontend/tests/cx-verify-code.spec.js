// «Проверьте почту» (#3057) и «Изменить email» (#3058): экран кода. Числа на
// экране — куда ушёл код, сколько попыток, когда можно повторить — приходят с
// сервера; здесь проверяется, что экран их показывает и правильно ветвится по
// сырым отказам (`err.raw`), а не по переведённым фразам.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import * as naive from 'naive-ui'

const api = vi.hoisted(() => ({
  verification: vi.fn(),
  verifyCode: vi.fn(),
  resendVerification: vi.fn(),
  startEmailChange: vi.fn(),
  cancelEmailChange: vi.fn(),
  verifyEmail: vi.fn(),
}))
vi.mock('@/api', () => ({ accountFlows: api }))

const push = vi.fn()
let query = {}
vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query, fullPath: '/verify-email' }),
    useRouter: () => ({ push, replace: vi.fn() }),
  }
})

let authed = true
const setUser = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return authed
    },
    user: { email: 'maria@tessera.dev', email_verified: false },
    setUser,
  }),
}))

import VerifyEmailView from '@/views/VerifyEmailView.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  RouterLink: { props: ['to'], template: '<a><slot /></a>' },
}

const STATE = {
  email: 'maria@tessera.dev',
  verified: false,
  target: 'maria@tessera.dev',
  change: false,
  expires_at: '2126-01-01T00:00:00Z',
  attempts_left: 5,
  resend_in: 42,
  mail: true,
  has_password: true,
}

function refusal(raw, status, data) {
  const e = new Error('localized')
  e.raw = raw
  e.status = status
  e.data = data
  return e
}

async function open() {
  const w = mount(VerifyEmailView, { global: { stubs } })
  await flushPromises()
  return w
}

async function type(w, code) {
  const input = w.find('[data-testid="code-input"]')
  input.element.value = code
  await input.trigger('input')
  await flushPromises()
}

describe('экран кода подтверждения', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach((f) => f.mockReset())
    push.mockReset()
    setUser.mockReset()
    authed = true
    query = {}
    api.verification.mockResolvedValue({ data: { ...STATE } })
  })
  afterEach(() => vi.useRealTimers())

  it('говорит, куда ушёл код, и считает паузу до повтора', async () => {
    const w = await open()
    expect(w.text()).toContain('Проверьте почту')
    expect(w.text()).toContain('Код отправлен на maria@tessera.dev')
    expect(w.findAll('[data-testid="code-cell"]')).toHaveLength(6)
    expect(w.find('[data-testid="code-resend-wait"]').text()).toContain('0:42')
    expect(w.find('[data-testid="code-resend"]').exists()).toBe(false)
    // Код живой — экран сам ничего не шлёт.
    expect(api.resendVerification).not.toHaveBeenCalled()
  })

  it('вставка целиком: лишнее отбрасывается, шесть цифр уходят сами', async () => {
    api.verifyCode.mockResolvedValue({ data: { user: { email_verified: true } } })
    const w = await open()
    await type(w, '48 19-37')
    expect(api.verifyCode).toHaveBeenCalledWith('481937')
    expect(w.find('[data-testid="verify-done"]').text()).toContain('maria@tessera.dev')
    expect(setUser).toHaveBeenCalled()
  })

  it('неверный код: сколько попыток осталось, ячейки очищены', async () => {
    api.verifyCode.mockRejectedValue(refusal('invalid code', 400, { attempts_left: 3 }))
    const w = await open()
    await type(w, '000000')
    expect(w.find('[data-testid="code-error"]').text()).toContain('осталось попыток: 3')
    expect(w.find('[data-testid="code-input"]').element.value).toBe('')
  })

  it('попытки кончились и код устарел — свои слова', async () => {
    api.verifyCode.mockRejectedValueOnce(refusal('too many attempts', 400, { attempts_left: 0 }))
    const w = await open()
    await type(w, '111111')
    expect(w.find('[data-testid="code-error"]').text()).toContain('Попытки кончились')
    api.verifyCode.mockRejectedValueOnce(refusal('code expired', 400))
    await type(w, '222222')
    expect(w.find('[data-testid="code-error"]').text()).toContain('Код устарел')
  })

  it('нет живого кода — экран отправляет его сам', async () => {
    api.verification.mockResolvedValue({
      data: { ...STATE, expires_at: null, resend_in: 0 },
    })
    api.resendVerification.mockResolvedValue({ data: { ...STATE, resend_in: 60 } })
    const w = await open()
    expect(api.resendVerification).toHaveBeenCalledTimes(1)
    expect(w.find('[data-testid="code-resend-wait"]').text()).toContain('1:00')
  })

  it('повтор раньше времени: 429 перезапускает отсчёт, без ошибки', async () => {
    api.verification.mockResolvedValue({ data: { ...STATE, resend_in: 0 } })
    api.resendVerification.mockRejectedValue(refusal('too many requests', 429, { retry_after: 17 }))
    const w = await open()
    await w.find('[data-testid="code-resend"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="code-resend-wait"]').text()).toContain('0:17')
    expect(w.find('[data-testid="code-error"]').exists()).toBe(false)
  })

  it('«Изменить email»: пароль у подтверждённого аккаунта, потом код на новый адрес', async () => {
    api.verification.mockResolvedValue({ data: { ...STATE, verified: true, expires_at: null } })
    api.startEmailChange.mockResolvedValue({
      data: { ...STATE, verified: true, change: true, target: 'new@tessera.dev', resend_in: 60 },
    })
    query = { change: '1', next: '/settings?s=profile' }
    const w = await open()
    expect(w.find('[data-testid="change-password"]').exists()).toBe(true)
    await w.find('[data-testid="change-email"] input').setValue('new@tessera.dev')
    await w.find('[data-testid="change-password"] input').setValue('secret-123')
    await w.find('[data-testid="change-submit"]').trigger('click')
    await flushPromises()
    expect(api.startEmailChange).toHaveBeenCalledWith('new@tessera.dev', 'secret-123')
    expect(w.text()).toContain('Код отправлен на new@tessera.dev')
    expect(w.find('[data-testid="code-cancel-change"]').exists()).toBe(true)

    api.verifyCode.mockResolvedValue({ data: { user: { email: 'new@tessera.dev' } } })
    await type(w, '123456')
    expect(w.text()).toContain('Почта изменена')
    await w.find('[data-testid="verify-continue"]').trigger('click')
    expect(push).toHaveBeenCalledWith('/settings?s=profile')
  })

  it('неподтверждённый аккаунт меняет адрес без пароля (исправить опечатку)', async () => {
    const w = await open()
    await w.find('[data-testid="code-change-email"]').trigger('click')
    expect(w.find('[data-testid="change-password"]').exists()).toBe(false)
  })

  it('без сессии — предлагает войти, ничего не спрашивая у сервера', async () => {
    authed = false
    const w = await open()
    expect(w.find('[data-testid="verify-signed-out"]').exists()).toBe(true)
    expect(api.verification).not.toHaveBeenCalled()
  })

  it('ссылка из письма работает как раньше', async () => {
    query = { token: 'tok' }
    api.verifyEmail.mockResolvedValue({})
    const w = await open()
    expect(api.verifyEmail).toHaveBeenCalledWith('tok')
    expect(w.find('[data-testid="verify-state"]').text()).toContain('подтверждена')
  })

  it('чужой ?next не уводит из приложения', async () => {
    api.verification.mockResolvedValue({ data: { ...STATE, verified: true } })
    query = { next: 'https://evil.example' }
    const w = await open()
    await w.find('[data-testid="verify-continue"]').trigger('click')
    expect(push).toHaveBeenCalledWith('/')
  })
})
