// A custom-date recurrence has two different things to say about a day, and the
// grid used to say them with the same mark (#2932).
//
// `occ` is a promise: the server is going to fire on this day. For a custom rule
// the editor handed the whole picked list to that promise and returned before it
// ever looked at `once` — and «Повторять всегда» is offered for custom rules like
// for any other (no `v-if` on the checkbox). Untick it with five dates picked and
// the calendar promised four more firings while the server, by its own test
// (`recurrence_once_end_test.go`), hands over exactly one and then drops the rule.
// The same shortcut lit up dates the series had already walked past.
//
// The picked list still has to be visible — this grid is where it is picked — so
// the days that aren't occurrences keep a mark of their own (`pick`) instead of
// borrowing the one that promises a firing.
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { NMessageProvider } from 'naive-ui'

// Pinned so "local midnight" is a fixed thing and the grid is reproducible.
const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Europe/Moscow'
afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

vi.mock('@/api', () => ({
  users: { updatePreferences: vi.fn(() => Promise.resolve()) },
  getAccessToken: () => null,
  projects: { milestones: vi.fn(() => Promise.resolve({ data: [] })) },
  milestones: {},
  gitlab: { projectConfig: vi.fn(() => Promise.reject(new Error('no integration'))) },
}))

const DueEditor = (await import('@/components/DueEditor.vue')).default

let wrapper = null

function mountWith(props) {
  wrapper = mount(
    { render: () => h(NMessageProvider, null, { default: () => h(DueEditor, props) }) },
    { attachTo: document.body },
  )
  return wrapper
}

// Day numbers carrying `cls` in the month currently in view (cells spilling in
// from the neighbouring months are `out`).
function marked(w, cls) {
  return w
    .findAll('.de-day')
    .filter((b) => b.classes(cls) && !b.classes('out'))
    .map((b) => Number(b.text()))
    .sort((a, b) => a - b)
}

const MARCH = ['2026-03-02', '2026-03-09', '2026-03-16', '2026-03-23']

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

describe('custom-date recurrence previews only the days that will fire', () => {
  it('promises one more firing when «repeat forever» is off', async () => {
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(marked(w, 'occ')).toEqual([9])
  })

  it('still shows the dates picked past the end of a one-off rule', async () => {
    // They are in the list and this grid is where the list is edited — they just
    // aren't occurrences.
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(marked(w, 'pick')).toEqual([16, 23])
  })

  it('marks a date the series has already walked past as picked, not upcoming', async () => {
    const w = mountWith({
      due: new Date(2026, 2, 9, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(marked(w, 'occ')).toEqual([16, 23])
    expect(marked(w, 'pick')).toEqual([2])
  })

  // ── negative controls ──

  it('keeps promising the whole list while the rule repeats forever', async () => {
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(marked(w, 'occ')).toEqual([9, 16, 23])
    expect(marked(w, 'pick')).toEqual([])
  })

  it('leaves non-custom rules without any picked days', async () => {
    // `pick` is the custom list's own mark; a weekly rule has no list to show.
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'weekly', interval: 1, once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(marked(w, 'occ')).toEqual([9])
    expect(marked(w, 'pick')).toEqual([])
  })
})
