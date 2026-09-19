import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import { hourTicksInWindow } from '@/utils/timeAxis'
import { createFormatters } from '@/utils/format'

// #2932 — the 12/24-hour preference reaches every clock this app prints.
//
// Two places rendered a time of day by hand and so ignored the setting entirely:
//
//  1. The HOUR RULER of the Gantt/Timeline header. It was the one band of that
//     header never given a formatter — `buildDays`/`buildMonthBands` next to it
//     take one precisely "so the axis speaks the user's language" — and it built
//     its labels as String(h).padStart(2,'0'). The contradiction was visible in a
//     single frame: the cursor pill hangs off the very tick it labels and goes
//     through `formatTime`, so a 12-hour user read "13" on the ruler and
//     "01:00 PM" in the chip touching it.
//
//  2. The two QUIET-HOURS selects in notification settings, where the option list
//     was a module-level constant of `${hh}:${mm}` strings — frozen not only on a
//     24-hour clock but, like every constant option list this screen used to have
//     (#2799), in the language of the first render.
//
// What must NOT change: the 24-hour rendering everyone who never touched the
// preference already sees — "04", not "4".

const at = (h, m = 0) => new Date(2026, 0, 1, h, m)

describe('hourTicksInWindow — метки линейки часов', () => {
  it('в 24-часовом формате оставляет ровно прежние метки с ведущим нулём', () => {
    const dayW = 240 // step = 4 → часы 4,8,12,16,20
    const fmt = createFormatters({ language: 'ru', timeFormat: '24h' })
    const ticks = hourTicksInWindow(1, dayW, 0, dayW, fmt)
    expect(ticks.map((t) => t.label)).toEqual(['04', '08', '12', '16', '20'])
  })

  it('по умолчанию (без formatters) метки те же — вызовы без пятого аргумента не сломаны', () => {
    const dayW = 240
    expect(hourTicksInWindow(1, dayW, 0, dayW).map((t) => t.label)).toEqual([
      '04',
      '08',
      '12',
      '16',
      '20',
    ])
  })

  it('в 12-часовом формате метка после полудня несёт час 1..12, а не 13..23', () => {
    const dayW = 240
    const fmt = createFormatters({ language: 'en', timeFormat: '12h' })
    const labels = hourTicksInWindow(1, dayW, 0, dayW, fmt).map((t) => t.label)
    // Ни одна метка не остаётся 24-часовой.
    expect(labels.some((l) => /^(1[3-9]|2[0-3])$/.test(l))).toBe(false)
    // 16:00 читается как 4 пополудни.
    const afternoon = labels[3]
    expect(afternoon).toMatch(/\b4\b/)
    expect(afternoon.toUpperCase()).toContain('PM')
    // 08:00 — до полудня.
    expect(labels[1].toUpperCase()).toContain('AM')
  })

  it('метка тика совпадает с тем, что пилюля курсора печатает для того же часа', () => {
    const dayW = 240
    for (const timeFormat of ['24h', '12h']) {
      const fmt = createFormatters({ language: 'en', timeFormat })
      const ticks = hourTicksInWindow(1, dayW, 0, dayW, fmt)
      for (const t of ticks) {
        const hour = (t.key % 24) + 0
        // То же, что cursorLabel в useChartTimeline: formatTime в браузерной зоне.
        const pill = fmt.formatTime(at(hour), { timeZone: null })
        const half = timeFormat === '12h' ? /AM|PM/i : null
        if (half) expect(pill.toUpperCase().slice(-2)).toBe(t.label.toUpperCase().slice(-2))
        // Час в метке — тот же, что в пилюле (пилюля несёт ещё и минуты).
        const hourOfPill = pill.match(/\d+/)[0]
        expect(t.label.match(/\d+/)[0].replace(/^0/, '')).toBe(hourOfPill.replace(/^0/, ''))
      }
    }
  })

  it('язык меток следует настройке (русский не печатает AM/PM)', () => {
    const dayW = 240
    const ru = createFormatters({ language: 'ru', timeFormat: '24h' })
    expect(hourTicksInWindow(1, dayW, 0, dayW, ru).every((t) => /^\d\d$/.test(t.label))).toBe(true)
  })

  it('одинаковый час в соседних днях окна получает одинаковую метку', () => {
    const dayW = 240
    const fmt = createFormatters({ language: 'en', timeFormat: '12h' })
    const ticks = hourTicksInWindow(3, dayW, 0, dayW * 3, fmt)
    const byHour = new Map()
    for (const t of ticks) {
      const h = t.key % 24
      if (byHour.has(h)) expect(t.label).toBe(byHour.get(h))
      else byHour.set(h, t.label)
    }
    expect(byHour.size).toBe(5)
  })

  it('шаг 0 (слишком узкий день) по-прежнему даёт пустой список', () => {
    expect(hourTicksInWindow(3, 20, 0, 60, createFormatters({}))).toEqual([])
  })
})

// ── quiet hours ──────────────────────────────────────────────────────────────

const chApi = { list: vi.fn(), remove: vi.fn(), update: vi.fn() }
const rtApi = { list: vi.fn(), remove: vi.fn(), update: vi.fn() }
const prefsApi = { get: vi.fn(), update: vi.fn() }

vi.mock('@/api', () => ({
  notificationChannels: chApi,
  notificationRoutes: rtApi,
  notificationPrefs: prefsApi,
}))

vi.mock('@/utils/device', () => ({
  getDeviceId: () => 'dev-1',
  notificationsSupported: () => false,
}))

// The screen reads the clock/language preferences off the theme store; the real
// one drags in the API client and the whole hydration for nothing.
const themeState = { timezone: '', language: 'ru', timeFormat: '24h', dateFormat: '', weekStart: 1 }
vi.mock('@/stores/theme', () => ({ useThemeStore: () => themeState }))

// The two quiet selects are the last two on the screen and show their current
// value as rendered text — which is exactly what the user reads, so the labels
// are asserted off the DOM rather than off the options prop.
async function quietLabels(start, end) {
  chApi.list.mockResolvedValue({ data: [] })
  rtApi.list.mockResolvedValue({ data: [] })
  prefsApi.get.mockResolvedValue({
    data: { quiet_enabled: true, quiet_start_minutes: start, quiet_end_minutes: end },
  })
  const NotificationSettings = (await import('@/components/NotificationSettings.vue')).default
  const w = mount(NotificationSettings)
  await flushPromises()
  const shown = w.findAll('.n-base-selection-label').map((n) => n.text())
  expect(shown.length, 'селекты расписания не отрисовались').toBeGreaterThanOrEqual(5)
  w.unmount()
  return shown.slice(-2)
}

describe('NotificationSettings — тихие часы в формате пользователя', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.resetModules()
    Object.assign(themeState, {
      timezone: '',
      language: 'ru',
      timeFormat: '24h',
      dateFormat: '',
      weekStart: 1,
    })
  })

  it('в 24-часовом формате подписи прежние — «23:00», «07:30»', async () => {
    expect(await quietLabels(1380, 450)).toEqual(['23:00', '07:30'])
  })

  it('полночь в 24-часовом формате остаётся «00:00», а не «24:00»', async () => {
    expect((await quietLabels(0, 30))[0]).toBe('00:00')
  })

  it('в 12-часовом формате вечерний час не остаётся 24-часовым', async () => {
    themeState.timeFormat = '12h'
    themeState.language = 'en'
    const [from, to] = await quietLabels(1380, 450)
    expect(from).not.toBe('23:00')
    expect(from).toMatch(/\b11\b/)
    expect(from.toUpperCase()).toContain('PM')
    // formatTime keeps a 2-digit hour in both clocks, so the morning value is
    // "07:30 AM" — the half-of-day marker is the part that was missing.
    expect(to).toContain('07:30')
    expect(to.toUpperCase()).toContain('AM')
  })

  it('в 12-часовом формате полночь читается как 12 AM, а не «00:00»', async () => {
    themeState.timeFormat = '12h'
    themeState.language = 'en'
    const [from] = await quietLabels(0, 30)
    expect(from).not.toBe('00:00')
    expect(from.toUpperCase()).toContain('AM')
    expect(from).toMatch(/\b12\b/)
  })
})
