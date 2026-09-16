// Третий конец развилки приглашения — дверь «через GitLab» (#2932).
//
// Страница приёма отправляет любого незалогиненного на регистрацию, а там рядом
// с формой стоит кнопка SSO. Переход к провайдеру — полная смена страницы, и
// `?invite=` / `?next=` на ней заканчиваются: приглашённый возвращался с
// callback'а на доску, с личным пространством, без членства и без единого
// намёка, что приглашение не применено — единственная копия токена в почте.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import * as naive from 'naive-ui'
import { takePendingInvite, stashPendingInvite } from '@/utils/pendingInvite'

vi.mock('@/api', () => ({
  auth: {
    providers: vi.fn(() => Promise.resolve({ data: { gitlab: true } })),
    gitlabAuthorizeUrl: () => 'https://gitlab.example/authorize',
  },
  setAccessToken: () => {},
}))

// Десктопная ветка входа сюда не едет (там системный браузер) — гоним веб.
vi.mock('@/utils/serverBase', async (importOriginal) => ({
  ...(await importOriginal()),
  isTauri: () => false,
}))

let query = {}
const replace = vi.fn()
vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query }),
    useRouter: () => ({ push: vi.fn(), replace }),
  }
})

let loginWithTokens = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    login: vi.fn(),
    register: vi.fn(),
    isAuthenticated: false,
    loginWithTokens: (...a) => loginWithTokens(...a),
  }),
}))

import LoginView from '@/views/LoginView.vue'
import RegisterView from '@/views/RegisterView.vue'
import OAuthCallbackView from '@/views/OAuthCallbackView.vue'

const RouterLinkStub = defineComponent({
  props: { to: { type: [String, Object], required: true } },
  render() {
    return h('a', {}, this.$slots.default?.())
  },
})

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  RouterLink: RouterLinkStub,
}

// Кнопка SSO — единственная в форме без data-testid, ищем по подписи локали.
async function clickGitlab(view, label) {
  const w = mount(view, { global: { stubs } })
  await flushPromises()
  const btn = w.findAll('button').find((b) => b.text().includes(label))
  expect(btn, 'кнопка входа через GitLab не нарисована').toBeTruthy()
  await btn.trigger('click')
  await flushPromises()
  w.unmount()
}

describe('приглашение переживает круг через GitLab', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    query = {}
    sessionStorage.clear()
    replace.mockClear()
    loginWithTokens = vi.fn()
    // jsdom не умеет уходить со страницы — подменяем адрес, иначе клик по SSO
    // роняет в консоль «Not implemented: navigation».
    delete window.location
    window.location = { href: '', hash: '', pathname: '/oauth/callback' }
  })

  it('регистрация: токен из ?invite паркуется перед уходом к провайдеру', async () => {
    query = { invite: 'inv-tok' }
    await clickGitlab(RegisterView, 'GitLab')
    expect(window.location.href).toBe('https://gitlab.example/authorize')
    expect(takePendingInvite()).toBe('inv-tok')
  })

  it('вход: токен достаётся из ?next и паркуется', async () => {
    query = { next: '/invite?token=inv-tok' }
    await clickGitlab(LoginView, 'GitLab')
    expect(takePendingInvite()).toBe('inv-tok')
  })

  it('обычный вход через SSO ничего не паркует', async () => {
    await clickGitlab(LoginView, 'GitLab')
    expect(takePendingInvite()).toBe('')
  })

  // Чужой ?next — не наше приглашение: тот же запрет, что и у ссылки-развилки.
  it('чужой ?next не превращается в приглашение', async () => {
    query = { next: 'https://evil.example/invite?token=inv-tok' }
    await clickGitlab(LoginView, 'GitLab')
    expect(takePendingInvite()).toBe('')
  })

  it('возврат с callback уводит на страницу приёма, а не на доску', async () => {
    stashPendingInvite('inv-tok')
    window.location.hash = '#access_token=acc'
    const w = mount(OAuthCallbackView, { global: { stubs } })
    await flushPromises()
    expect(loginWithTokens).toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith('/invite?token=inv-tok')
    // Одноразово: перезагрузка callback'а не должна снова уводить на приём.
    expect(takePendingInvite()).toBe('')
    w.unmount()
  })

  it('возврат без приглашения по-прежнему ведёт на доску', async () => {
    window.location.hash = '#access_token=acc'
    const w = mount(OAuthCallbackView, { global: { stubs } })
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('/')
    w.unmount()
  })

  // Провалившийся вход тратит приглашение: оставленное, оно встретило бы
  // следующий вход в этой вкладке уже протухшим токеном.
  it('провал входа не оставляет приглашение следующему', async () => {
    stashPendingInvite('inv-tok')
    window.location.hash = ''
    const w = mount(OAuthCallbackView, { global: { stubs } })
    await flushPromises()
    expect(replace).toHaveBeenCalledWith({ path: '/login', query: { oauth_error: 'no_token' } })
    expect(takePendingInvite()).toBe('')
    w.unmount()
  })
})
