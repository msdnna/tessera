import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'

// Mock the api so persist()'s users.updatePreferences never touches network.
// vi.mock is hoisted, so the fn must live in vi.hoisted.
// getAccessToken is how persist() tells "signed in" apart from "anonymous" since
// #2684 moved the access token out of localStorage into the api module.
const { updatePreferences, getAccessToken } = vi.hoisted(() => ({
  updatePreferences: vi.fn(() => Promise.resolve()),
  getAccessToken: vi.fn(() => ''),
}))
vi.mock('@/api', () => ({ users: { updatePreferences }, getAccessToken }))

import { useThemeStore, COLOR_THEMES } from '@/stores/theme'
import { DARK, LIGHT } from '@/styles/tokens'

// The store's language default reads navigator.languages (#2818), and jsdom's
// own value is 'en-US' — pin it per case so these tests don't hinge on that.
// Restored one spy at a time rather than via restoreAllMocks(), which would also
// strip the api mocks above of their implementations.
let langSpy = null
function stubLanguages(list) {
  langSpy = vi.spyOn(navigator, 'languages', 'get').mockReturnValue(list)
}

describe('theme store', () => {
  beforeEach(() => {
    localStorage.clear()
    updatePreferences.mockClear()
    getAccessToken.mockReturnValue('')
    setActivePinia(createPinia())
  })
  afterEach(() => {
    localStorage.clear()
    langSpy?.mockRestore()
    langSpy = null
  })

  it('defaults to purple accent and system theme mode', () => {
    const s = useThemeStore()
    expect(s.activeTheme.key).toBe('purple')
    expect(s.themeMode).toBe('system')
    expect(COLOR_THEMES[0].key).toBe('purple')
  })

  it('selectColor switches accent and persists to localStorage', () => {
    const s = useThemeStore()
    const blue = COLOR_THEMES.find((t) => t.key === 'blue')
    s.selectColor(blue)
    expect(s.activeTheme.key).toBe('blue')
    expect(s.primaryColor).toBe(blue.primary)
    expect(localStorage.getItem('tessera_color')).toBe('blue')
    const prefs = JSON.parse(localStorage.getItem('tessera_prefs'))
    expect(prefs.accent).toBe('blue')
  })

  it('setThemeMode dark makes isDark true and writes tessera_dark=1', () => {
    const s = useThemeStore()
    s.setThemeMode('dark')
    expect(s.isDark).toBe(true)
    expect(s.themeMode).toBe('dark')
    expect(localStorage.getItem('tessera_dark')).toBe('1')
    expect(s.palette).toEqual(DARK)
    s.setThemeMode('light')
    expect(s.isDark).toBe(false)
    expect(localStorage.getItem('tessera_dark')).toBe('0')
    expect(s.palette).toEqual(LIGHT)
  })

  it('toggle flips between light and dark', () => {
    const s = useThemeStore()
    s.setThemeMode('light')
    s.toggle()
    expect(s.isDark).toBe(true)
    s.toggle()
    expect(s.isDark).toBe(false)
  })

  it('persist calls updatePreferences only when a token is present', () => {
    const s = useThemeStore()
    s.setThemeMode('dark')
    expect(updatePreferences).not.toHaveBeenCalled()
    getAccessToken.mockReturnValue('abc')
    s.setThemeMode('light')
    expect(updatePreferences).toHaveBeenCalledTimes(1)
  })

  it('naiveTheme is null in light and a theme object in dark', () => {
    const s = useThemeStore()
    s.setThemeMode('light')
    expect(s.naiveTheme).toBeNull()
    s.setThemeMode('dark')
    expect(s.naiveTheme).toBeTruthy()
  })

  it('themeOverrides carries accent primary and dark overrides', () => {
    const s = useThemeStore()
    s.setThemeMode('light')
    const light = s.themeOverrides
    expect(light.common.primaryColor).toBe(s.activeTheme.primary)
    expect(light.common.borderRadius).toBe('8px')
    // Light mode has no dark-only component keys.
    expect(light.Modal).toBeUndefined()
    s.setThemeMode('dark')
    const dark = s.themeOverrides
    expect(dark.common.primaryColor).toBe(s.activeTheme.primary)
    expect(dark.Modal).toBeDefined()
    expect(dark.common.textColor1).toBe(DARK.text1)
  })

  it('applyCssVars exports the input-surface vars the hand-rolled editor uses', async () => {
    const s = useThemeStore()
    const css = () => document.documentElement.style
    // applyCssVars runs from a watcher, so let it flush after each mode change.
    s.setThemeMode('dark')
    await nextTick()
    // The boxed comment composer paints itself with these; they must track the
    // Naive Input skin (color / placeholderColor), not the modal surface.
    expect(css().getPropertyValue('--t-input-bg')).toBe(DARK.inputBg)
    expect(css().getPropertyValue('--t-placeholder')).toBe(DARK.placeholder)
    expect(DARK.inputBg).toBe(DARK.surfaceAlt)
    s.setThemeMode('light')
    await nextTick()
    expect(css().getPropertyValue('--t-input-bg')).toBe(LIGHT.inputBg)
    expect(css().getPropertyValue('--t-placeholder')).toBe(LIGHT.placeholder)
  })

  it('onPrimaryColor is white on the dark purple accent', () => {
    const s = useThemeStore()
    expect(s.onPrimaryColor).toBe('#ffffff')
    // Orange is light enough to require dark text.
    s.selectColor(COLOR_THEMES.find((t) => t.key === 'orange'))
    expect(s.onPrimaryColor).toBe('#1f1f1f')
  })

  it('hydrate applies server prefs without persisting to the server', () => {
    const s = useThemeStore()
    s.hydrate({
      accent: 'green',
      theme: 'dark',
      language: 'en',
      time_format: '12h',
      week_start: 0,
      board_background: 'grid',
    })
    expect(s.activeTheme.key).toBe('green')
    expect(s.themeMode).toBe('dark')
    expect(s.language).toBe('en')
    expect(s.timeFormat).toBe('12h')
    expect(s.weekStart).toBe(0)
    expect(s.boardBackground).toBe('grid')
    // hydrate must not push back to the server.
    expect(updatePreferences).not.toHaveBeenCalled()
    // But it does refresh the first-paint cache.
    expect(JSON.parse(localStorage.getItem('tessera_prefs')).accent).toBe('green')
  })

  it('hydrate(null) is a no-op', () => {
    const s = useThemeStore()
    const before = s.activeTheme.key
    s.hydrate(null)
    expect(s.activeTheme.key).toBe(before)
  })

  it('hydrate ignores an unknown accent key (keeps current)', () => {
    const s = useThemeStore()
    s.selectColor(COLOR_THEMES.find((t) => t.key === 'red'))
    s.hydrate({ accent: 'nonsense' })
    expect(s.activeTheme.key).toBe('red')
  })

  it('setLocale merges partial localizing fields and persists', () => {
    const s = useThemeStore()
    s.setLocale({ language: 'en', week_start: 0 })
    expect(s.language).toBe('en')
    expect(s.weekStart).toBe(0)
    // Untouched field keeps its default.
    expect(s.timeFormat).toBe('24h')
    expect(JSON.parse(localStorage.getItem('tessera_prefs')).language).toBe('en')
  })

  it('setBoardBackground normalizes falsy to empty string', () => {
    const s = useThemeStore()
    s.setBoardBackground('dots')
    expect(s.boardBackground).toBe('dots')
    s.setBoardBackground(null)
    expect(s.boardBackground).toBe('')
  })

  it('reset drops the accent to purple but keeps the theme mode', () => {
    const s = useThemeStore()
    s.selectColor(COLOR_THEMES.find((t) => t.key === 'teal'))
    s.setThemeMode('dark')
    s.setLocale({ language: 'en', country: 'US' })
    s.setBoardBackground('grid')
    updatePreferences.mockClear()
    stubLanguages(['de-DE'])
    s.reset()
    // Accent back to the brand purple so the auth screens stay on-brand (#2817).
    expect(s.activeTheme.key).toBe('purple')
    expect(s.primaryColor).toBe('#7c5cff')
    // Theme mode is device-level — a dark-mode user isn't flashed into white.
    expect(s.isDark).toBe(true)
    expect(s.themeMode).toBe('dark')
    // Account-bound prefs back to defaults. Language is the one that isn't a
    // constant: logout lands on the auth screens, so it re-follows the browser
    // (#2818) — here a locale we don't ship, hence the 'ru' default.
    expect(s.language).toBe('ru')
    expect(s.country).toBe('')
    expect(s.boardBackground).toBe('')
    // Local caches follow the reset accent, and no PUT goes out (token is gone).
    expect(localStorage.getItem('tessera_color')).toBe('purple')
    expect(JSON.parse(localStorage.getItem('tessera_prefs')).accent).toBe('purple')
    expect(localStorage.getItem('tessera_dark')).toBe('1')
    expect(updatePreferences).not.toHaveBeenCalled()
  })

  it('first visit takes the language from the browser', () => {
    stubLanguages(['en-GB', 'ru-RU'])
    expect(useThemeStore().language).toBe('en')
  })

  it('first visit falls back to ru when we ship none of the browser languages', () => {
    stubLanguages(['de-DE', 'fr'])
    expect(useThemeStore().language).toBe('ru')
  })

  it('a stored language choice beats the browser', () => {
    localStorage.setItem('tessera_prefs', JSON.stringify({ language: 'ru' }))
    stubLanguages(['en-US'])
    expect(useThemeStore().language).toBe('ru')
  })

  it('hydrate lets the account language override the browser guess', () => {
    stubLanguages(['en-US'])
    const s = useThemeStore()
    expect(s.language).toBe('en')
    s.hydrate({ language: 'ru' })
    expect(s.language).toBe('ru')
  })

  it('reset then hydrate brings the next user accent back', () => {
    const s = useThemeStore()
    s.selectColor(COLOR_THEMES.find((t) => t.key === 'teal'))
    s.reset()
    expect(s.activeTheme.key).toBe('purple')
    s.hydrate({ accent: 'green', theme: 'light' })
    expect(s.activeTheme.key).toBe('green')
    expect(s.primaryColor).toBe(COLOR_THEMES.find((t) => t.key === 'green').primary)
  })

  it('reads legacy tessera_color / tessera_dark keys on init', () => {
    localStorage.setItem('tessera_color', 'pink')
    localStorage.setItem('tessera_dark', '1')
    setActivePinia(createPinia())
    const s = useThemeStore()
    expect(s.activeTheme.key).toBe('pink')
    expect(s.themeMode).toBe('dark')
  })

  it('reads a cached tessera_prefs blob on init (wins over legacy)', () => {
    localStorage.setItem('tessera_color', 'pink')
    localStorage.setItem(
      'tessera_prefs',
      JSON.stringify({ accent: 'blue', theme: 'light', language: 'en' }),
    )
    setActivePinia(createPinia())
    const s = useThemeStore()
    expect(s.activeTheme.key).toBe('blue')
    expect(s.themeMode).toBe('light')
    expect(s.language).toBe('en')
  })
})
