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

describe('stage tab date pickers', () => {
  it('follow the date preset and the first day of the week', async () => {
    const theme = useThemeStore()
    theme.dateFormat = 'iso'
    theme.weekStart = 0 // Sunday → Naive's firstDayOfWeek 6
    const w = mountWith(ProjectStagesTab, {
      project: { id: '11111111-1111-1111-1111-111111111111', slug: 'tessera' },
    })
    await nextTick()
    w.findComponent(ProjectStagesTab).vm.startAdd()
    await nextTick()
    await nextTick()
    const pickers = w.findAllComponents(NDatePicker)
    expect(pickers.length).toBeGreaterThan(0)
    for (const p of pickers) {
      expect(p.props('format')).toBe('yyyy-MM-dd')
      expect(p.props('firstDayOfWeek')).toBe(6)
    }
  })

  it('render the chosen preset, not Naive’s ISO default', async () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    theme.dateFormat = 'short' // 31.12.2026
    theme.weekStart = 1
    const w = mountWith(ProjectStagesTab, {
      project: { id: '11111111-1111-1111-1111-111111111111', slug: 'tessera' },
    })
    await nextTick()
    w.findComponent(ProjectStagesTab).vm.startAdd()
    await nextTick()
    await nextTick()
    const pickers = w.findAllComponents(NDatePicker)
    expect(pickers.length).toBeGreaterThan(0)
    for (const p of pickers) {
      expect(p.props('format')).toBe('dd.MM.yyyy')
      expect(p.props('firstDayOfWeek')).toBe(0)
    }
  })
})
