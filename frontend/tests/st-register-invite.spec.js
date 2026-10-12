import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Only the register call matters here; stub the client so the test can assert on
// the body that was sent rather than on axios.
const { register } = vi.hoisted(() => ({ register: vi.fn() }))
vi.mock('@/api', () => ({
  auth: { register },
  setAccessToken: () => {},
}))

// hydrate() would fire a preferences round-trip on every register.
vi.mock('@/stores/theme', () => ({
  useThemeStore: () => ({ language: 'ru', hydrate: () => {}, setLocale: () => {} }),
}))

import { useAuthStore } from '@/stores/auth'

// The invitation token is the invitee's proof they own the invited address —
// without it the backend joins them to nothing, so the register call must carry
// it through whenever the invite page handed one over (#2932).
describe('register carries the invitation token', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    register.mockReset()
    register.mockResolvedValue({ data: { access_token: 't', user: { id: 'u1' } } })
  })

  it('sends invite_token when the invite page passed one', async () => {
    await useAuthStore().register('a@b.c', 'A', 'password-123', 'inv-token')
    expect(register).toHaveBeenCalledWith({
      email: 'a@b.c',
      name: 'A',
      password: 'password-123',
      invite_token: 'inv-token',
    })
  })

  it('omits invite_token on a plain sign-up', async () => {
    await useAuthStore().register('a@b.c', 'A', 'password-123')
    expect(register).toHaveBeenCalledWith({ email: 'a@b.c', name: 'A', password: 'password-123' })
  })
})
