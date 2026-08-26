import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// The theme store persists through the API on every change; nothing here should
// reach the network.
vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
}))

const { useFormat } = await import('@/composables/useFormat')
const { useDateLocale } = await import('@/composables/useDateLocale')
const { useThemeStore } = await import('@/stores/theme')

// The store-bound layer (#2798): what components see. The formatting itself is
// covered in ut-format.spec.js — here it's the wiring that matters, above all
// that a preference change is picked up without re-calling the composable.
// Every expectation below is written against Russian formatting, which used to
// be the store's hard-coded default. Since #2818 that default follows
// navigator.languages — 'en-US' under jsdom — so the assumption has to be stated
// rather than inherited.
function pinRussianBrowser() {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['ru-RU'])
}

describe('useFormat', () => {
  beforeEach(() => {
    localStorage.clear()
    pinRussianBrowser()
    setActivePinia(createPinia())
  })

  it('follows a language change made after the composable was called', () => {
    const theme = useThemeStore()
    theme.timezone = 'UTC'
    const { formatDate, locale } = useFormat()
    expect(formatDate('2026-12-31T12:00:00Z', { preset: 'medium' })).toBe('31 дек. 2026 г.')
    theme.language = 'en'
    expect(locale.value).toBe('en-GB')
    expect(formatDate('2026-12-31T12:00:00Z', { preset: 'medium' })).toBe('31 Dec 2026')
  })

  it('follows a timezone change', () => {
    const theme = useThemeStore()
    const { formatDate, timeZone } = useFormat()
    theme.timezone = 'Europe/Moscow'
    expect(timeZone.value).toBe('Europe/Moscow')
    expect(formatDate('2026-01-01T23:30:00Z')).toBe('02.01.2026')
    theme.timezone = 'America/New_York'
    expect(formatDate('2026-01-01T23:30:00Z')).toBe('01.01.2026')
  })

  it('follows the 12/24h preference', () => {
    const theme = useThemeStore()
    theme.timezone = 'UTC'
    const { formatTime } = useFormat()
    expect(formatTime('2026-01-01T18:30:00Z')).toBe('18:30')
    theme.timeFormat = '12h'
    // The day-period marker is the locale's: ru-RU capitalizes it, en-GB doesn't.
    expect(formatTime('2026-01-01T18:30:00Z')).toBe('06:30 PM')
  })

  it('maps week_start onto the Naive firstDayOfWeek (Mon=1 → 0, Sun=0 → 6)', () => {
    const theme = useThemeStore()
    const { firstDayOfWeek } = useFormat()
    theme.weekStart = 1
    expect(firstDayOfWeek.value).toBe(0)
    theme.weekStart = 0
    expect(firstDayOfWeek.value).toBe(6)
  })

  it('exposes picker patterns that track the preferences', () => {
    const theme = useThemeStore()
    const { datePattern, dateTimePattern } = useFormat()
    expect(datePattern.value).toBe('dd.MM.yyyy')
    theme.setLocale({ date_format: 'iso' })
    expect(datePattern.value).toBe('yyyy-MM-dd')
    theme.setLocale({ time_format: '12h' })
    expect(dateTimePattern.value).toBe('yyyy-MM-dd hh:mm a')
  })

  it('offers date presets as samples rendered in the current language', () => {
    const theme = useThemeStore()
    const { datePresetOptions } = useFormat()
    expect(datePresetOptions.value.map((o) => o.value)).toEqual(['short', 'medium', 'long', 'iso'])
    expect(datePresetOptions.value[1].label).toBe('31 дек. 2026 г.')
    theme.language = 'en'
    expect(datePresetOptions.value[1].label).toBe('31 Dec 2026')
  })

  it('hands out a formatter set for the pure helpers in utils/', () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/Moscow'
    const { formatters } = useFormat()
    expect(formatters.value.formatDate('2026-01-01T23:30:00Z')).toBe('02.01.2026')
  })
})

describe('theme store date_format', () => {
  beforeEach(() => {
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('is a preset, and a legacy pattern from the server is converted on the way in', () => {
    const theme = useThemeStore()
    expect(theme.dateFormat).toBe('short')
    theme.hydrate({ date_format: 'yyyy-MM-dd' })
    expect(theme.dateFormat).toBe('iso')
    theme.setLocale({ date_format: 'MM/dd/yyyy' })
    expect(theme.dateFormat).toBe('short')
  })

  it('drops back to the default preset on logout', () => {
    const theme = useThemeStore()
    theme.setLocale({ date_format: 'iso' })
    theme.reset()
    expect(theme.dateFormat).toBe('short')
  })
})

// useDateLocale is now a thin adapter; its 15+ callers must keep working.
describe('useDateLocale (adapter)', () => {
  beforeEach(() => {
    localStorage.clear()
    pinRussianBrowser()
    setActivePinia(createPinia())
  })

  it('still exposes the picker props and formatDue', () => {
    const theme = useThemeStore()
    theme.weekStart = 0
    const dl = useDateLocale()
    expect(dl.firstDayOfWeek.value).toBe(6)
    expect(dl.dateFormat.value).toBe('dd.MM.yyyy')
    expect(dl.dateTimeFormat.value).toBe('dd.MM.yyyy HH:mm')
    expect(typeof dl.formatDue).toBe('function')
  })
})
