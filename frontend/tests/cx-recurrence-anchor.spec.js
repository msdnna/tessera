// The editor's calendar has to preview the day the *server* will pick (#2932).
//
// A monthly/yearly rule carries a server-managed day-of-month anchor (`day`, plus
// `month` for yearly). It exists because a short month temporarily moves the due
// date: a task repeating on the 31st lands on Feb 28, and without the anchor every
// later occurrence would inherit the 28th and the series would silently walk
// backwards. `recurrenceToStore` (backend/handlers/tasks.go) therefore carries the
// stored anchor over whenever the due date wasn't touched, and re-derives it from
// the new due date (`WithAnchor`) when it was.
//
// The editor's own grid highlights the upcoming occurrences, and it rebuilds the
// rule from its local controls — which never held `day`/`month`. So the preview
// fell back to "the day the due date happens to be on" exactly in the case the
// anchor was invented for: the task is sitting on Feb 28, and the calendar promised
// March 28 while the server had always been going to fire on March 31.
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

const editor = (w) => w.findComponent(DueEditor)

// The day numbers the grid highlights as upcoming occurrences, for the month
// currently in view (cells spilling in from the neighbouring months are `out`).
function highlighted(w) {
  return w
    .findAll('.de-day')
    .filter((b) => b.classes('occ') && !b.classes('out'))
    .map((b) => Number(b.text()))
    .sort((a, b) => a - b)
}

// Walk the calendar `n` months forward (the grid opens on the due date's month).
async function forward(w, n) {
  for (let i = 0; i < n; i++) editor(w).vm.stepMonth(1)
  await nextTick()
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

describe('recurrence preview honours the stored anchor', () => {
  it('previews the anchored day, not the day February clamped the task to', async () => {
    // A "repeat on the 31st" task that February has parked on the 28th.
    const w = mountWith({
      due: new Date(2026, 1, 28, 9, 0).getTime(),
      recurrence: { freq: 'monthly', interval: 1, day: 31, tz: 'Europe/Moscow' },
    })
    await nextTick()
    await forward(w, 1) // March 2026
    expect(highlighted(w)).toEqual([31])
  })

  it('keeps clamping the anchor to short months, like the server does', async () => {
    const w = mountWith({
      due: new Date(2026, 1, 28, 9, 0).getTime(),
      recurrence: { freq: 'monthly', interval: 1, day: 31, tz: 'Europe/Moscow' },
    })
    await nextTick()
    await forward(w, 2) // April 2026 — 30 days
    expect(highlighted(w)).toEqual([30])
    await forward(w, 1) // May 2026 — back to the 31st
    expect(highlighted(w)).toEqual([31])
  })

  it('carries a yearly month+day anchor', async () => {
    const w = mountWith({
      due: new Date(2026, 1, 28, 9, 0).getTime(),
      recurrence: { freq: 'yearly', interval: 1, day: 31, month: 3, tz: 'Europe/Moscow' },
    })
    await nextTick()
    await forward(w, 13) // March 2027
    expect(highlighted(w)).toEqual([31])
  })

  // ── negative controls: the anchor must not leak where the server drops it ──

  it('previews from the due date when the rule has no anchor yet', async () => {
    const w = mountWith({ due: new Date(2026, 1, 20, 9, 0).getTime(), recurrence: null })
    await nextTick()
    editor(w).vm.setFreq('monthly')
    await nextTick()
    await forward(w, 1) // March 2026
    expect(highlighted(w)).toEqual([20])
  })

  it('re-derives the anchor from a due date picked in the editor', async () => {
    // Moving the due date is exactly the case where the server calls WithAnchor and
    // throws the old anchor away — the preview has to follow, not keep promising 31.
    const w = mountWith({
      due: new Date(2026, 1, 28, 9, 0).getTime(),
      recurrence: { freq: 'monthly', interval: 1, day: 31, tz: 'Europe/Moscow' },
    })
    await nextTick()
    editor(w).vm.onDayClick(new Date(2026, 1, 12))
    await nextTick()
    await forward(w, 1) // March 2026
    expect(highlighted(w)).toEqual([12])
  })

  it('drops the anchor when the frequency changes', async () => {
    // `recurrenceToStore` only carries the anchor across a rule of the *same* freq.
    const w = mountWith({
      due: new Date(2026, 1, 28, 9, 0).getTime(),
      recurrence: { freq: 'yearly', interval: 1, day: 31, month: 12, tz: 'Europe/Moscow' },
    })
    await nextTick()
    editor(w).vm.setFreq('monthly')
    await nextTick()
    await forward(w, 1) // March 2026
    expect(highlighted(w)).toEqual([28])
  })
})
