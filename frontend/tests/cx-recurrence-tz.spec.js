// A recurrence rule has to carry the zone its calendar was drawn in (#2932).
//
// Every field of the rule names a calendar day — a weekday, a day of the month, a
// hand-picked date, "not on a weekend" — but the due date leaves the editor as an
// instant, and a freshly picked date defaults to *local* midnight. The backend used
// to re-read those days in its own zone (UTC), so for anyone east of UTC the whole
// series came out a day off the days the editor's own calendar highlighted: "every
// Monday" fired on Tuesdays, skip-weekends skipped Monday instead of Saturday.
//
// format.js already states the contract — anything the server evaluates against a
// clock has to be handed the resolved zone, not the blank. This pins that the rule
// the editor emits carries it.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: { projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))) },
}))

const DueEditor = (await import('@/components/DueEditor.vue')).default
const { useThemeStore } = await import('@/stores/theme')
const { resolvedTimezone } = await import('@/utils/format')

let wrapper = null

function mountWith(props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(DueEditor, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

const editor = (w) => w.findComponent(DueEditor)

// The rule of the last `apply` the editor emitted.
function lastRule(w) {
  const events = editor(w).emitted('apply') || []
  return events.length ? events[events.length - 1][0].recurrence : null
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

describe('recurrence rule timezone', () => {
  it('carries the profile zone into the rule', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Europe/Moscow'
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('weekly')
    await nextTick()
    expect(lastRule(w)).toMatchObject({ freq: 'weekly', tz: 'Europe/Moscow' })
  })

  it('falls back to the browser zone when no profile zone is set', async () => {
    const theme = useThemeStore()
    theme.timezone = ''
    const browser = resolvedTimezone('')
    const w = mountWith({ due: Date.UTC(2026, 3, 5, 21, 0), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('daily')
    await nextTick()
    // An environment with no Intl data resolves to '' — there the rule may omit it.
    if (browser) expect(lastRule(w).tz).toBe(browser)
    else expect(lastRule(w).tz).toBeUndefined()
  })

  it('keeps the zone on a rule the editor loaded and re-emitted', async () => {
    const theme = useThemeStore()
    theme.timezone = 'Asia/Vladivostok'
    const w = mountWith({
      due: Date.UTC(2026, 3, 5, 14, 0),
      recurrence: { freq: 'weekly', interval: 1, weekdays: [1], tz: 'Asia/Vladivostok' },
    })
    await nextTick()
    editor(w).vm.toggleWeekday(3)
    await nextTick()
    const rule = lastRule(w)
    expect(rule.weekdays).toEqual([1, 3])
    expect(rule.tz).toBe('Asia/Vladivostok')
  })
})
