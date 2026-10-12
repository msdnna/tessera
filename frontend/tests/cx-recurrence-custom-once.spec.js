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
// The picked list still has to be visible — it is where the list is edited. The
// due picker (#3118) keeps the two apart by place: the main grid marks only the
// occurrences (`rep`), the «Повтор» panel's own grid — where «По датам» is ticked
// — marks the picked list.
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { nextTick } from 'vue'
import { loadPicker, mountPicker, highlighted } from './helpers/duePicker'
import { setActivePinia, createPinia } from 'pinia'

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

const DuePicker = await loadPicker()

let wrapper = null

function mountWith(props) {
  wrapper = mountPicker(DuePicker, props)
  return wrapper
}

// The picked dates in the «Повтор» panel's grid (the due itself is `sel` there).
async function picked(w) {
  const p = w.findComponent(DuePicker).vm
  await p.openPane('rep')
  await nextTick()
  const days = [...document.querySelectorAll('[data-testid="due-rep-cal"] .dc-day.rep')]
    .filter((b) => !b.classList.contains('out'))
    .map((b) => Number(b.textContent.trim()))
    .sort((a, b) => a - b)
  p.pane = 'main'
  await nextTick()
  return days
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
    mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted()).toEqual([9])
  })

  it('still shows the dates picked past the end of a one-off rule', async () => {
    // They are in the list and this grid is where the list is edited — they just
    // aren't occurrences.
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted()).toEqual([9])
    expect(await picked(w)).toEqual([9, 16, 23])
  })

  it('marks a date the series has already walked past as picked, not upcoming', async () => {
    const w = mountWith({
      due: new Date(2026, 2, 9, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted()).toEqual([16, 23])
    expect(await picked(w)).toEqual([2, 16, 23])
  })

  // ── negative controls ──

  it('keeps promising the whole list while the rule repeats forever', async () => {
    mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'custom', dates: [...MARCH], tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted()).toEqual([9, 16, 23])
  })

  it('leaves non-custom rules without any picked days', async () => {
    // The picked list is the custom rule's own; a weekly rule has none to show.
    const w = mountWith({
      due: new Date(2026, 2, 2, 9, 0).getTime(),
      recurrence: { freq: 'weekly', interval: 1, once: true, tz: 'Europe/Moscow' },
    })
    await nextTick()
    expect(highlighted()).toEqual([9])
    expect(await picked(w)).toEqual([])
  })
})
