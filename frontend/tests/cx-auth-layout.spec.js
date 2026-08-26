import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// The theme store persists through the api module; stub it so the language
// toggle never reaches the network. vi.mock is hoisted, hence vi.hoisted here.
const { updatePreferences, getAccessToken } = vi.hoisted(() => ({
  updatePreferences: vi.fn(() => Promise.resolve()),
  getAccessToken: vi.fn(() => ''),
}))
vi.mock('@/api', () => ({ users: { updatePreferences }, getAccessToken }))

import AuthLayout from '@/components/AuthLayout.vue'
import { useThemeStore } from '@/stores/theme'
import { i18n } from '@/i18n'

// Mounted layouts attach pointer listeners for the glow and keep a live store
// subscription; an un-unmounted one goes on reacting to later specs' state.
let wrapper = null
function mountLayout() {
  wrapper = mount(AuthLayout, {
    props: { title: 'Tessera' },
    global: {
      stubs: {
        DownloadAppButton: true,
        BrandLogo: true,
        NPopover: true,
        NIcon: true,
      },
    },
  })
  return wrapper
}

const toggle = () => wrapper.get('[data-testid="auth-lang-toggle"]')

describe('AuthLayout language toggle', () => {
  beforeEach(() => {
    localStorage.clear()
    updatePreferences.mockClear()
    getAccessToken.mockReturnValue('')
    setActivePinia(createPinia())
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    localStorage.clear()
    i18n.global.locale.value = 'ru'
  })

  it('labels itself with the language currently in use', () => {
    const theme = useThemeStore()
    theme.setLocale({ language: 'ru' })
    mountLayout()
    expect(toggle().text()).toBe('RU')
  })

  it('a click cycles the store language and relabels the button', async () => {
    const theme = useThemeStore()
    theme.setLocale({ language: 'ru' })
    mountLayout()

    await toggle().trigger('click')
    expect(theme.language).toBe('en')
    expect(toggle().text()).toBe('EN')

    // Two locales, so the cycle is a round trip.
    await toggle().trigger('click')
    expect(theme.language).toBe('ru')
    expect(toggle().text()).toBe('RU')
  })

  it('names the language it would switch to, not the current one', () => {
    const theme = useThemeStore()
    theme.setLocale({ language: 'ru' })
    mountLayout()
    // Russian bundle, and language names are endonyms in both bundles.
    expect(toggle().attributes('title')).toBe('Переключить на English')
    expect(toggle().attributes('aria-label')).toBe(toggle().attributes('title'))
  })

  it('keeps the choice locally and sends no PUT while signed out', async () => {
    mountLayout()
    await toggle().trigger('click')
    // The auth screens have no token, so persisting must stay in localStorage.
    expect(updatePreferences).not.toHaveBeenCalled()
    expect(JSON.parse(localStorage.getItem('tessera_prefs')).language).toBe(
      useThemeStore().language,
    )
  })

  it('falls back to a shippable label when the stored language is unknown', () => {
    // A stale/hand-edited pref must not render an empty or bogus button.
    const theme = useThemeStore()
    theme.setLocale({ language: 'de' })
    mountLayout()
    expect(toggle().text()).toBe('RU')
  })
})
