// Приглашение приходит на рабочий адрес, а в браузере открыт личный аккаунт —
// самый обычный случай, и единственный отказ приёма, который лечится действием
// самого приглашённого (выйти и войти под нужным адресом). Страница же гасила
// все причины в один catch: «недействительно, истекло или предназначено другому
// адресу», а из выходов — только ссылка «На главную». Токен при этом уже
// потрачен на визит, второй ссылки в интерфейсе нет — за ней надо возвращаться в
// почту (#2932).
//
// v2 (#3059): страница сначала показывает превью приглашения, принимается оно
// кнопкой «Принять приглашение» — отказы и выходы из них прежние.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import * as naive from 'naive-ui'

const acceptInvitation = vi.fn()
const lookupInvitation = vi.fn()
vi.mock('@/api', () => ({
  accountFlows: {
    acceptInvitation: (...a) => acceptInvitation(...a),
    lookupInvitation: (...a) => lookupInvitation(...a),
  },
}))

const PREVIEW = {
  kind: 'email',
  state: 'pending',
  role: 'member',
  email: 'me@example.com',
  workspace: { name: 'Студия', name_key: null, color: '' },
  projects: 2,
  members: 3,
  inviter: { name: 'Илья Петров' },
}

const replace = vi.fn()
const push = vi.fn()
let query = {}
vi.mock('vue-router', async () => {
  const actual = await vi.importActual('vue-router')
  return {
    ...actual,
    useRoute: () => ({ query }),
    useRouter: () => ({ replace, push }),
  }
})

const logout = vi.fn()
let authed = true
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return authed
    },
    get user() {
      return authed ? { email: 'me@example.com' } : null
    },
    logout,
  }),
}))

vi.mock('@/stores/workspaces', () => ({
  useWorkspacesStore: () => ({
    loadWorkspaces: vi.fn(() => Promise.resolve()),
    selectWorkspace: vi.fn(() => Promise.resolve()),
  }),
}))

import AcceptInviteView from '@/views/AcceptInviteView.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
  teleport: true,
  // No router instance here (useRouter is mocked), so the real RouterLink has
  // nothing to resolve against.
  RouterLink: { template: '<a><slot /></a>' },
}

function refusal(raw, status) {
  const e = new Error('localized text')
  e.raw = raw
  e.status = status
  return e
}

// Opens the page on its preview and presses «Принять приглашение».
async function open() {
  const w = mount(AcceptInviteView, { global: { stubs } })
  await flushPromises()
  await w.find('[data-testid="invite-accept"]').trigger('click')
  await flushPromises()
  return w
}

describe('accepting an invitation under the wrong account', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    acceptInvitation.mockReset()
    lookupInvitation.mockReset()
    lookupInvitation.mockResolvedValue({ data: PREVIEW })
    replace.mockReset()
    push.mockReset()
    logout.mockReset()
    authed = true
    query = { token: 'inv-tok' }
  })

  it('names the wrong-account refusal instead of the catch-all line', async () => {
    acceptInvitation.mockRejectedValue(refusal('this invitation is for a different email', 403))
    const w = await open()
    expect(w.text()).toContain('другим аккаунтом')
    expect(w.text()).not.toContain('истекло')
  })

  it('offers a way out: sign out and come back to this very invitation', async () => {
    acceptInvitation.mockRejectedValue(refusal('this invitation is for a different email', 403))
    const w = await open()
    const btn = w.find('[data-testid="invite-switch-account"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    await flushPromises()
    expect(logout).toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith({
      path: '/login',
      query: { next: '/invite?token=inv-tok' },
    })
  })

  it('keeps the catch-all line for a spent or expired invitation', async () => {
    acceptInvitation.mockRejectedValue(refusal('invalid or expired invitation', 400))
    const w = await open()
    expect(w.text()).toContain('истекло')
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(false)
  })

  // A 403 the server phrased some other way must not be read as "wrong account"
  // — the offered cure (sign out) would be plain wrong advice.
  it('does not guess: another 403 stays the catch-all line', async () => {
    acceptInvitation.mockRejectedValue(refusal('forbidden', 403))
    const w = await open()
    expect(w.text()).toContain('истекло')
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(false)
  })

  it('a signed-out visitor still goes to the register form with the token', async () => {
    authed = false
    await open()
    expect(push).toHaveBeenCalledWith({ path: '/register', query: { invite: 'inv-tok' } })
    expect(acceptInvitation).not.toHaveBeenCalled()
  })

  it('accepts as before when the account matches', async () => {
    acceptInvitation.mockResolvedValue({ data: { id: 'ws-1' } })
    const w = await open()
    expect(w.text()).toContain('присоединились')
    expect(w.find('[data-testid="invite-switch-account"]').exists()).toBe(false)
  })
})
