import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ru from '@/locales/ru'

// A GitLab-provisioned account has no password hash, so "change password" can
// only ever come back "current password is incorrect" — a form that cannot be
// won and whose refusal reads like a typo. The server now says so up front
// (`has_password`), and Settings offers to send a link instead (#2932).

const apiMock = vi.hoisted(() => ({
  users: { updateProfile: vi.fn(), changePassword: vi.fn() },
  accountFlows: {
    forgotPassword: vi.fn(() => Promise.resolve({ data: { ok: true } })),
    resendVerification: vi.fn(),
  },
}))
vi.mock('@/api', () => apiMock)

const authState = vi.hoisted(() => ({
  user: null,
  setUser: vi.fn(),
  setAuth: vi.fn(),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))

// v2 (#3106): Security is its own section component; the settings page shows
// one section at a time, so the gate is checked on the section itself.
import SettingsSecurity from '@/components/settings/SettingsSecurity.vue'

let wrapper = null

function mountWith(user) {
  authState.user = user
  const i18n = createI18n({ legacy: false, locale: 'ru', messages: { ru } })
  wrapper = mount(SettingsSecurity, { global: { plugins: [i18n] } })
  return wrapper
}

// Every mount is torn down: one left open keeps its scheduler and the next
// test's re-render never lands (ut-vitest leak, see cx-whats-new-history).
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

beforeEach(() => {
  apiMock.accountFlows.forgotPassword.mockClear()
})

const pwFields = (w) => w.findAll('input[type="password"]')
const linkBtn = (w) =>
  w.findAll('button').find((b) => b.text() === ru.settings.security.setLink) || null

describe('Settings → Security for an account without a password', () => {
  it('replaces the unwinnable form with a way to set one', async () => {
    const w = mountWith({ id: 'u1', email: 'oauth@example.com', has_password: false })
    expect(pwFields(w)).toHaveLength(0)
    expect(w.text()).toContain('oauth@example.com')

    const btn = linkBtn(w)
    expect(btn).not.toBeNull()
    await btn.trigger('click')
    await flushPromises()

    expect(apiMock.accountFlows.forgotPassword).toHaveBeenCalledWith('oauth@example.com')
    // The button gives way to the confirmation — no second link on a double tap.
    expect(linkBtn(w)).toBeNull()
  })

  it('keeps the form for an ordinary account', () => {
    const w = mountWith({ id: 'u2', email: 'local@example.com', has_password: true })
    expect(pwFields(w)).toHaveLength(3)
    expect(linkBtn(w)).toBeNull()
  })

  it('keeps the form when the flag is missing', () => {
    // A profile cached in localStorage by a build that predates the field: the
    // absent flag must not lock a password-holding account out of its own form.
    const w = mountWith({ id: 'u3', email: 'cached@example.com' })
    expect(pwFields(w)).toHaveLength(3)
    expect(linkBtn(w)).toBeNull()
  })
})
