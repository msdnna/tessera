// Оба конца развилки «есть аккаунт / нет аккаунта». Страница приёма отправляет
// любого незалогиненного на регистрацию, но у половины приглашённых аккаунт уже
// есть — они жмут «Войти», и раньше токен на этом шаге терялся: вход уводил на
// главную, приглашение оставалось непринятым, а единственная ссылка на него — в
// почте. Обратная ссылка («нет аккаунта — зарегистрируйтесь») роняла его так же
// (#2932).
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import * as naive from 'naive-ui'

vi.mock('@/api', () => ({
  auth: { providers: vi.fn(() => Promise.resolve({ data: { gitlab: false } })) },
  setAccessToken: () => {},
}))

let query = {}
vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query }),
    useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ login: vi.fn(), register: vi.fn(), isAuthenticated: false }),
}))

import LoginView from '@/views/LoginView.vue'
import RegisterView from '@/views/RegisterView.vue'

// Records the route object the link was handed, which is the whole assertion —
// `href` would need a router instance to resolve against.
const RouterLinkStub = defineComponent({
  props: { to: { type: [String, Object], required: true } },
  render() {
    return h('a', { 'data-to': JSON.stringify(this.to) }, this.$slots.default?.())
  },
})

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  RouterLink: RouterLinkStub,
}

async function links(view) {
  const w = mount(view, { global: { stubs } })
  await flushPromises()
  return w.findAll('a[data-to]').map((a) => JSON.parse(a.attributes('data-to')))
}

// A bare `to="/login"` and a `{ path: '/login' }` are the same destination, so
// the plain cases stay green either way: what the two invite cases assert is the
// query riding along, not the shape of the prop.
const has = (all, path) =>
  all.map((r) => (typeof r === 'string' ? { path: r } : r)).find((r) => r.path === path)

describe('the login/register fork carries an invitation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    query = {}
  })

  it('register → login replays the invitation after signing in', async () => {
    query = { invite: 'inv-tok' }
    expect(has(await links(RegisterView), '/login')).toEqual({
      path: '/login',
      query: { next: '/invite?token=inv-tok' },
    })
  })

  it('login → register hands the token to the sign-up form', async () => {
    query = { next: '/invite?token=inv-tok' }
    expect(has(await links(LoginView), '/register')).toEqual({
      path: '/register',
      query: { invite: 'inv-tok' },
    })
  })

  it('a plain visit keeps both links plain', async () => {
    expect(has(await links(RegisterView), '/login')).toEqual({ path: '/login' })
    expect(has(await links(LoginView), '/register')).toEqual({ path: '/register' })
  })

  // ?next is just a query parameter on a link anyone can hand out — only our own
  // invite path may turn into a token the sign-up form then presents.
  it('a foreign ?next does not become an invitation', async () => {
    query = { next: 'https://evil.example/invite?token=inv-tok' }
    expect(has(await links(LoginView), '/register')).toEqual({ path: '/register' })
  })
})
