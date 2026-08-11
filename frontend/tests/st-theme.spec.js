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

describe('theme store', () => {
  beforeEach(() => {
    localStorage.clear()
    updatePreferences.mockClear()
    getAccessToken.mockReturnValue('')
    setActivePinia(createPinia())
  })
  afterEach(() => localStorage.clear())

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

  it('reset restores account-bound prefs but keeps appearance', () => {
    const s = useThemeStore()
    s.selectColor(COLOR_THEMES.find((t) => t.key === 'teal'))
    s.setThemeMode('dark')
    s.setLocale({ language: 'en', country: 'US' })
    s.setBoardBackground('grid')
    s.reset()
    // Appearance kept.
    expect(s.activeTheme.key).toBe('teal')
    expect(s.isDark).toBe(true)
    // Account-bound prefs back to defaults.
    expect(s.language).toBe('ru')
    expect(s.country).toBe('')
    expect(s.boardBackground).toBe('')
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
