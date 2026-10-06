// The calendar board layout must start the week where the user asked (#2932).
//
// Settings → Локализация has a "first day of the week", and everything else
// already obeys it: the Naive pickers (useFormat.firstDayOfWeek), the due-date
// calendar, formatRelative's "inside this week" window and the Gantt/Timeline
// week bands (utils/timeAxis buildWeekBands). The month grid of the calendar
// layout was built Monday-first by hand, so switching to Sunday left it — and
// only it — a week out of step with the calendar in the due editor right next
// to it.
//
// Pinned here at the grid level: the weekday header row and the day the first
// cell falls on, which is what a user actually reads off the screen.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api', () => ({
  tasks: {},
  getAccessToken: () => null,
}))

const BoardCalendarView = (await import('@/components/BoardCalendarView.vue')).default
const { useThemeStore } = await import('@/stores/theme')

let wrapper = null

function mountCalendar() {
  wrapper = mount(BoardCalendarView, { props: { tasks: [], statusColumns: [] } })
  return wrapper
}

// The grid always starts on the first day of the week on or before the 1st of
// the displayed month, and the component opens on the current one — so the
// expectation is derived the same way rather than pinned to a fixed date.
function expectedFirstCellDay(firstDow) {
  const first = new Date(new Date().getFullYear(), new Date().getMonth(), 1)
  const offset = (first.getDay() - firstDow + 7) % 7
  return new Date(first.getFullYear(), first.getMonth(), 1 - offset)
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

describe('BoardCalendarView month grid', () => {
  it('starts the week on Sunday when that is the preference', async () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    theme.weekStart = 0
    const w = mountCalendar()
    await nextTick()

    const headers = w.findAll('.cal-wd').map((el) => el.text())
    expect(headers).toHaveLength(7)
    expect(headers[0]).toMatch(/^вс/i)
    expect(headers[6]).toMatch(/^сб/i)

    const first = expectedFirstCellDay(0)
    expect(first.getDay()).toBe(0) // guard: the helper agrees with the preference
    expect(w.findAll('.cal-daynum')[0].text()).toBe(String(first.getDate()))
  })

  it('keeps Monday first when that is the preference', async () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    theme.weekStart = 1
    const w = mountCalendar()
    await nextTick()

    const headers = w.findAll('.cal-wd').map((el) => el.text())
    expect(headers[0]).toMatch(/^пн/i)
    expect(headers[6]).toMatch(/^вс/i)

    const first = expectedFirstCellDay(1)
    expect(first.getDay()).toBe(1)
    expect(w.findAll('.cal-daynum')[0].text()).toBe(String(first.getDate()))
  })
})
