// Naive UI pickers must speak the user's localization preferences (#2932).
//
// Settings → Локализация offers a date preset, a 12/24h clock and a first day of
// the week. Naive's pickers format and parse by a date-fns pattern of their own
// and default to `yyyy-MM-dd` / `HH:mm`, with the week starting wherever the
// *interface language* pack says — none of which follows those three settings.
// So every picker has to be handed `:format` (and `:first-day-of-week` where it
// shows a calendar); RemindersView and ConferencesView already do, and
// useFormat/useDateLocale exist for exactly this.
//
// What this pins is the wiring, not the patterns themselves (ut-format covers
// those): the props the pickers actually receive under a non-default preference.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NTimePicker, NDatePicker, NMessageProvider } from 'naive-ui'

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: { projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))) },
}))

const DueEditor = (await import('@/components/DueEditor.vue')).default
const MilestoneManager = (await import('@/components/MilestoneManager.vue')).default
const { useThemeStore } = await import('@/stores/theme')

let wrapper = null

// Both components render Naive pickers; MilestoneManager hides them behind a
// modal, which teleports — findAllComponents still walks the component tree.
// It also calls useMessage(), so it only mounts under a message provider.
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

describe('DueEditor time picker', () => {
  it('follows the 12h clock preference', async () => {
    const theme = useThemeStore()
    theme.timeFormat = '12h'
    const w = mountWith(DueEditor, { due: Date.UTC(2026, 0, 15, 15, 0) })
    await nextTick()
    expect(w.findComponent(NTimePicker).props('format')).toBe('hh:mm a')
  })

  it('keeps the 24h clock when that is the preference', async () => {
    const theme = useThemeStore()
    theme.timeFormat = '24h'
    const w = mountWith(DueEditor, { due: Date.UTC(2026, 0, 15, 15, 0) })
    await nextTick()
    expect(w.findComponent(NTimePicker).props('format')).toBe('HH:mm')
  })
})

describe('MilestoneManager date pickers', () => {
  it('follow the date preset and the first day of the week', async () => {
    const theme = useThemeStore()
    theme.dateFormat = 'iso'
    theme.weekStart = 0 // Sunday → Naive's firstDayOfWeek 6
    const w = mountWith(MilestoneManager, {
      show: true,
      projectId: '11111111-1111-1111-1111-111111111111',
      projectName: 'Tessera',
      wsId: '22222222-2222-2222-2222-222222222222',
    })
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
    const w = mountWith(MilestoneManager, {
      show: true,
      projectId: '11111111-1111-1111-1111-111111111111',
      projectName: 'Tessera',
      wsId: '22222222-2222-2222-2222-222222222222',
    })
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
