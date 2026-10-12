// Naive UI pickers must speak the user's localization preferences (#2932).
//
// Settings → Локализация offers a date preset, a 12/24h clock and a first day of
// the week. Naive's pickers format and parse by a date-fns pattern of their own
// and default to `yyyy-MM-dd` / `HH:mm`, with the week starting wherever the
// *interface language* pack says — none of which follows those three settings.
// So every picker has to be handed `:format` (and `:first-day-of-week` where it
// shows a calendar); ConferencesView already does, and
// useFormat/useDateLocale exist for exactly this.
//
// What this pins is the wiring, not the patterns themselves (ut-format covers
// those): the props the pickers actually receive under a non-default preference.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NDatePicker, NMessageProvider } from 'naive-ui'

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: {
    projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))),
    listIntegrations: vi.fn(() => Promise.resolve({ data: { integrations: [] } })),
  },
}))

const DuePicker = (await import('@/components/due/DuePicker.vue')).default
const DateField = (await import('@/components/kit/DateField.vue')).default
const DueCalendar = (await import('@/components/due/DueCalendar.vue')).default
// The «Проект» window's stage tab (#2971) took over from MilestoneManager: its
// new-stage row carries the range picker.
const ProjectStagesTab = (await import('@/components/project/ProjectStagesTab.vue')).default
const { useThemeStore } = await import('@/stores/theme')

let wrapper = null

// Both components render Naive pickers and call useMessage(), so they only
// mount under a message provider.
function mountWith(component, props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(component, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

afterEach(() => {
  // An un-unmounted component keeps its watchers on the shared theme store and
  // silently poisons the next spec's scheduler.
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

// The due picker (#3118) has no Naive time picker any more: its clock is the
// «Время» field and the 09:00 / 12:00 / 18:00 chips, labelled through useFormat.
const chips = () =>
  [...document.querySelectorAll('.dp-chip[data-time]')]
    .filter((b) => b.dataset.time !== 'none')
    .map((b) => b.textContent.trim().toUpperCase())
const timeField = () => document.querySelector('[data-testid="due-time"]').value.toUpperCase()

describe('due picker clock', () => {
  it('follows the 12h clock preference', async () => {
    const theme = useThemeStore()
    theme.timeFormat = '12h'
    theme.timezone = 'UTC'
    mountWith(DuePicker, { show: true, due: Date.UTC(2026, 0, 15, 15, 0) })
    await nextTick()
    await nextTick()
    expect(chips()).toEqual(['09:00 AM', '12:00 PM', '06:00 PM'])
    expect(timeField()).toBe('03:00 PM')
  })

  it('keeps the 24h clock when that is the preference', async () => {
    const theme = useThemeStore()
    theme.timeFormat = '24h'
    theme.timezone = 'UTC'
    mountWith(DuePicker, { show: true, due: Date.UTC(2026, 0, 15, 15, 0) })
    await nextTick()
    await nextTick()
    expect(chips()).toEqual(['09:00', '12:00', '18:00'])
    expect(timeField()).toBe('15:00')
  })
})

// The stage tab's dates are the v2 DateField (#3034) on the due picker's grid:
// the calendar starts the week where the user chose, the label follows the preset.
async function openStageDates() {
  const w = mountWith(ProjectStagesTab, {
    project: { id: '11111111-1111-1111-1111-111111111111', slug: 'tessera' },
  })
  await nextTick()
  w.findComponent(ProjectStagesTab).vm.startAdd()
  await nextTick()
  await nextTick()
  expect(w.findAllComponents(NDatePicker)).toHaveLength(0)
  const field = w.findComponent(DateField)
  field.vm.open()
  await nextTick()
  await nextTick()
  return field
}

describe('stage tab date field', () => {
  it('starts the week on Sunday when that is the preference', async () => {
    useThemeStore().weekStart = 0
    const field = await openStageDates()
    expect(field.findComponent(DueCalendar).props('weekStart')).toBe(0)
  })

  it('starts the week on Monday by default and labels by the preset', async () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    theme.dateFormat = 'short' // 31.12.2026
    theme.weekStart = 1
    const field = await openStageDates()
    expect(field.findComponent(DueCalendar).props('weekStart')).toBe(1)
    // Two picks make the range; days 15–20 never repeat in a six-week grid.
    const day = (n) =>
      [...document.querySelectorAll('[data-testid="stage-new-dates-cal"] button')].find(
        (b) => b.textContent.trim() === String(n),
      )
    day(15).click()
    await nextTick()
    day(20).click()
    await nextTick()
    const now = new Date()
    const mm = String(now.getMonth() + 1).padStart(2, '0')
    expect(field.find('.df-text').text()).toBe(
      `15.${mm}.${now.getFullYear()} – 20.${mm}.${now.getFullYear()}`,
    )
  })
})
