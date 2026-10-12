// ⌘K palette, «Срок» (#3118): words in the field, the presets by digit once the
// arrows moved, the calendar under the field on Tab — and every write carrying
// the task's repeat along (a full-replace update without it would drop the rule).
import { describe, it, expect, beforeEach, afterEach, afterAll, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

const TZ_BEFORE = process.env.TZ
process.env.TZ = 'Europe/Moscow'
afterAll(() => {
  if (TZ_BEFORE === undefined) delete process.env.TZ
  else process.env.TZ = TZ_BEFORE
})

const RULE = { freq: 'weekly', interval: 1, weekdays: [1], tz: 'Europe/Moscow' }
const TASK = {
  id: 't1',
  number: 42,
  title: 'Редизайн доски',
  board_id: 'b1',
  column_id: 'c1',
  priority: 2,
  due_date: '2026-10-06T15:00:00.000Z', // вт 6 окт., 18:00 MSK
  start_date: null,
  recurrence: RULE,
  completed_at: null,
  assignees: [],
  tags: [],
}
const update = vi.fn(() => Promise.resolve({ data: {} }))

vi.mock('@/api', () => ({
  workspaces: {
    search: vi.fn(() => Promise.resolve({ data: {} })),
    members: vi.fn(() => Promise.resolve({ data: [] })),
    milestones: vi.fn(() => Promise.resolve({ data: [] })),
  },
  tasks: {
    get: vi.fn(() => Promise.resolve({ data: TASK })),
    update: (...a) => update(...a),
    move: vi.fn(),
  },
  boards: {
    get: vi.fn(() => Promise.resolve({ data: { id: 'b1', project_id: 'p1', workspace_id: 'w1' } })),
    columns: vi.fn(() => Promise.resolve({ data: [{ id: 'c1', name: 'К работе', position: 1 }] })),
    tasks: vi.fn(() => Promise.resolve({ data: [] })),
  },
  projects: { tags: vi.fn(() => Promise.resolve({ data: [] })) },
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), resolve: (p) => ({ href: String(p) }) }),
  useRoute: () => ({ params: {}, query: {}, path: '/' }),
}))

let wrapper
beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.setSystemTime(new Date(2026, 9, 3, 12, 0)) // сб 3 окт. 2026, полдень
  update.mockClear()
  Element.prototype.scrollIntoView = vi.fn()
})
afterEach(() => {
  wrapper?.unmount()
  vi.useRealTimers()
  document.body.innerHTML = ''
})

async function openDue() {
  const { default: CommandPalette } = await import('@/components/palette/CommandPalette.vue')
  const { usePaletteStore } = await import('@/stores/palette')
  const { useThemeStore } = await import('@/stores/theme')
  Object.assign(useThemeStore(), { timezone: 'Europe/Moscow', language: 'ru' })
  const palette = usePaletteStore()
  wrapper = mount(CommandPalette, { attachTo: document.body })
  palette.openTask('t1', 'due')
  await flushPromises()
  return palette
}
const input = () => document.querySelector('[data-testid=palette-input]')
async function key(k, opts = {}) {
  input().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...opts }))
  await flushPromises()
}
async function type(text) {
  const palette = (await import('@/stores/palette')).usePaletteStore()
  palette.query = text
  await flushPromises()
}
const sent = () => update.mock.calls.at(-1)?.[1]

describe('palette «Срок»', () => {
  it('shows the field chip, the presets with their digits and the calendar row', async () => {
    await openDue()
    expect(document.querySelector('[data-testid=palette-field-chip]').textContent).toContain('Срок')
    expect(document.querySelector('[data-row="due:tomorrow"]').textContent).toContain('вс, 4 окт.')
    expect(document.querySelector('[data-row="due:cal"]')).toBeTruthy()
  })

  it('applies what is typed, keeping the repeat', async () => {
    const palette = await openDue()
    await type('пт 9:30')
    expect(document.querySelector('[data-row="due:typed"]').textContent).toContain('пт, 9 окт.')
    await key('Enter')
    expect(sent().due_date).toBe('2026-10-09T06:30:00.000Z')
    expect(sent().recurrence).toMatchObject({ freq: 'weekly', weekdays: [1] })
    expect(palette.open).toBe(false) // opened straight onto the field: done
  })

  it('says what it did not understand and writes nothing', async () => {
    await openDue()
    await type('когда-нибудь')
    expect(document.querySelector('[data-row="due:bad"]').textContent).toContain('когда-нибудь')
    await key('Enter')
    expect(update).not.toHaveBeenCalled()
  })

  it('a digit is typing until the arrows move, then it is a preset', async () => {
    await openDue()
    await key('2')
    expect(update).not.toHaveBeenCalled()
    await key('ArrowDown')
    await key('2')
    // Завтра, keeping 18:00; a timed due keeps the profile zone in the rule.
    expect(sent().due_date).toBe('2026-10-04T15:00:00.000Z')
    expect(sent().recurrence.tz).toBe('Europe/Moscow')
  })

  it('«Без срока» clears the due', async () => {
    await openDue()
    await key('ArrowDown')
    await key('0')
    expect(sent().due_date).toBe(null)
  })

  it('Tab opens the calendar, the arrows walk it, ↵ picks the day', async () => {
    await openDue()
    await key('Tab')
    expect(document.querySelector('[data-testid=palette-due-cal]')).toBeTruthy()
    await key('ArrowRight') // вт 6 → ср 7
    await key('ArrowDown') // → ср 14
    await key('Enter')
    expect(sent().due_date).toBe('2026-10-14T15:00:00.000Z')
  })

  it('a second Tab goes back to the field', async () => {
    await openDue()
    await key('Tab')
    await key('Tab')
    expect(document.querySelector('[data-testid=palette-due-cal]')).toBeNull()
    expect(document.querySelector('[data-row="due:today"]')).toBeTruthy()
  })
})
