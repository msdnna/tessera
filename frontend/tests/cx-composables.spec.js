import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { ref, nextTick, defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

// The api module is imported (transitively) by several composables; mock it so no
// real axios/HTTP is constructed under jsdom.
vi.mock('@/api', () => ({
  tasks: {
    update: vi.fn().mockResolvedValue({}),
    move: vi.fn().mockResolvedValue({}),
    remove: vi.fn().mockResolvedValue({}),
    archive: vi.fn().mockResolvedValue({}),
  },
  users: { updatePrefs: vi.fn(), me: vi.fn() },
  columns: { update: vi.fn(), remove: vi.fn() },
}))

// ── useDateLocale ────────────────────────────────────────────────────────────
import { useDateLocale } from '@/composables/useDateLocale'
import { useThemeStore } from '@/stores/theme'

describe('useDateLocale', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('maps week_start to Naive firstDayOfWeek (Mon=1 → 0, Sun=0 → 6)', () => {
    const theme = useThemeStore()
    theme.weekStart = 1 // Monday
    const { firstDayOfWeek } = useDateLocale()
    expect(firstDayOfWeek.value).toBe(0)
    theme.weekStart = 0 // Sunday
    expect(firstDayOfWeek.value).toBe(6)
    theme.weekStart = 3 // Wednesday-ish
    expect(firstDayOfWeek.value).toBe(2)
  })

  it('builds a date/time format from the date format + 12h/24h preference', () => {
    const theme = useThemeStore()
    // The date part is language-derived, and the default language follows the
    // browser since #2818 — pin it, since what's under test is the time suffix.
    theme.language = 'ru'
    theme.dateFormat = 'dd.MM.yyyy'
    theme.timeFormat = '24h'
    let dl = useDateLocale()
    expect(dl.dateTimeFormat.value).toBe('dd.MM.yyyy HH:mm')
    theme.timeFormat = '12h'
    dl = useDateLocale()
    expect(dl.dateTimeFormat.value).toBe('dd.MM.yyyy hh:mm a')
  })

  it('formatDue returns relative labels for today/tomorrow/yesterday (ru)', () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    theme.weekStart = 1
    const { formatDue } = useDateLocale()

    // Build a date-only (UTC-midnight) value for "today" so it reads as relative.
    const now = new Date()
    const todayUTC = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()))
    expect(formatDue(todayUTC.toISOString())).toBe('сегодня')

    const tomorrow = new Date(todayUTC.getTime() + 86400000)
    expect(formatDue(tomorrow.toISOString())).toBe('завтра')

    const yesterday = new Date(todayUTC.getTime() - 86400000)
    expect(formatDue(yesterday.toISOString())).toBe('вчера')
  })

  it('formatDue returns English relative labels when language=en', () => {
    const theme = useThemeStore()
    theme.language = 'en'
    const { formatDue } = useDateLocale()
    const now = new Date()
    const todayUTC = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()))
    expect(formatDue(todayUTC.toISOString())).toBe('Today')
  })

  it('formatDue with long=true capitalizes the relative label', () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    const { formatDue } = useDateLocale()
    const now = new Date()
    const todayUTC = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()))
    expect(formatDue(todayUTC.toISOString(), { long: true })).toBe('Сегодня')
  })

  it('formatDue returns "" for an empty input', () => {
    setActivePinia(createPinia())
    const { formatDue } = useDateLocale()
    expect(formatDue('')).toBe('')
    expect(formatDue(null)).toBe('')
  })

  it('formatDue renders a far-off date as day + month (no relative)', () => {
    const theme = useThemeStore()
    theme.language = 'ru'
    const { formatDue } = useDateLocale()
    // 40 days out: outside the current week → absolute label, not a weekday.
    const now = new Date()
    const far = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) + 40 * 86400000)
    const label = formatDue(far.toISOString())
    expect(label).not.toBe('')
    expect(['сегодня', 'завтра', 'вчера']).not.toContain(label)
  })
})

// ── useTreeExpand ─────────────────────────────────────────────────────────────
describe('useTreeExpand', () => {
  beforeEach(() => localStorage.clear())

  it('returns the caller default for untouched nodes and the stored value after set', async () => {
    const { useTreeExpand } = await import('@/composables/useTreeExpand')
    const { isExpanded, setExpanded } = useTreeExpand()
    // Untouched node uses the passed default.
    expect(isExpanded('grp-1', true)).toBe(true)
    expect(isExpanded('proj-1', false)).toBe(false)
    // Explicit override wins over the default.
    setExpanded('proj-1', true)
    expect(isExpanded('proj-1', false)).toBe(true)
    setExpanded('grp-1', false)
    expect(isExpanded('grp-1', true)).toBe(false)
  })

  it('persists overrides to localStorage', async () => {
    const { useTreeExpand } = await import('@/composables/useTreeExpand')
    const { setExpanded } = useTreeExpand()
    setExpanded('node-persist', true)
    await nextTick()
    const raw = JSON.parse(localStorage.getItem('tessera_tree_expanded') || '{}')
    expect(raw['node-persist']).toBe(true)
  })
})

// ── useApiImage ───────────────────────────────────────────────────────────────
describe('useApiImage', () => {
  it('is a no-op on web (returns the URL unchanged)', async () => {
    const { useApiImage } = await import('@/composables/useApiImage')
    expect(useApiImage('/api/x').value).toBe('/api/x')
    expect(useApiImage(ref('/api/y')).value).toBe('/api/y')
    expect(useApiImage(() => '').value).toBe('')
  })
})

// ── useConnection ─────────────────────────────────────────────────────────────
describe('useConnection', () => {
  it('balances pending and surfaces the non-blocking bar past the show delay', async () => {
    vi.useFakeTimers()
    const mod = await import('@/composables/useConnection')
    const { connection, reqStart, reqEnd, setOffline } = mod

    // Fresh singleton for the assertions below.
    connection.pending = 0
    connection.active = false
    connection.offline = false

    reqStart()
    reqStart()
    expect(connection.pending).toBe(2)

    // Not yet shown — under the ~250ms show delay (fast calls never flash the bar).
    vi.advanceTimersByTime(100)
    expect(connection.active).toBe(false)

    // Past the delay with work still in flight → the top bar surfaces.
    vi.advanceTimersByTime(300)
    expect(connection.active).toBe(true)

    reqEnd(true) // reached the server → clears offline
    expect(connection.offline).toBe(false)
    reqEnd(true)
    expect(connection.pending).toBe(0)
    // The bar lingers for its minimum-visible window, then clears.
    vi.advanceTimersByTime(500)
    expect(connection.active).toBe(false)

    setOffline(true)
    expect(connection.offline).toBe(true)
    vi.useRealTimers()
  })

  it('never lets pending go negative', async () => {
    const { connection, reqEnd } = await import('@/composables/useConnection')
    connection.pending = 0
    reqEnd(false)
    expect(connection.pending).toBe(0)
  })
})

// ── useResponsive ─────────────────────────────────────────────────────────────
describe('useResponsive', () => {
  let listeners
  beforeEach(() => {
    listeners = []
    // Stub matchMedia: matches=true (mobile) with change-listener capture.
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation((query) => ({
        matches: true,
        media: query,
        addEventListener: (_e, cb) => listeners.push(cb),
        removeEventListener: (_e, cb) => {
          listeners = listeners.filter((l) => l !== cb)
        },
      })),
    )
    // matchMedia is read off `window` in the composable.
    window.matchMedia = globalThis.matchMedia
  })
  afterEach(() => vi.unstubAllGlobals())

  it('reflects the initial match and reacts to change events (mounted in a wrapper)', async () => {
    const { useResponsive } = await import('@/composables/useResponsive')
    const Wrapper = defineComponent({
      setup() {
        const { isMobile } = useResponsive(768)
        return { isMobile }
      },
      template: '<div>{{ isMobile }}</div>',
    })
    const w = mount(Wrapper)
    // onMounted seeds from mql.matches (true).
    expect(w.vm.isMobile).toBe(true)
    // Fire a synthetic change event turning mobile off.
    listeners.forEach((cb) => cb({ matches: false }))
    await nextTick()
    expect(w.vm.isMobile).toBe(false)
    w.unmount()
  })
})

// ── useLongPress ──────────────────────────────────────────────────────────────
describe('useLongPress', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('fires open() after the delay with the touch coordinates', async () => {
    const { useLongPress } = await import('@/composables/useLongPress')
    const open = vi.fn()
    let api
    const Wrapper = defineComponent({
      setup() {
        api = useLongPress(open, 450)
        return () => null
      },
    })
    mount(Wrapper)
    api.start({ touches: [{ clientX: 12, clientY: 34 }] })
    // Before the delay: nothing.
    vi.advanceTimersByTime(400)
    expect(open).not.toHaveBeenCalled()
    // After the delay: open with coordinates. pressMoved() is false (no touchmove).
    vi.advanceTimersByTime(100)
    expect(open).toHaveBeenCalledWith({ clientX: 12, clientY: 34 })
  })

  it('cancel() prevents the pending open', async () => {
    const { useLongPress } = await import('@/composables/useLongPress')
    const open = vi.fn()
    let api
    mount(
      defineComponent({
        setup() {
          api = useLongPress(open, 450)
          return () => null
        },
      }),
    )
    api.start({ touches: [{ clientX: 1, clientY: 2 }] })
    api.cancel()
    vi.advanceTimersByTime(1000)
    expect(open).not.toHaveBeenCalled()
  })

  it('does nothing when the event has no touch point', async () => {
    const { useLongPress } = await import('@/composables/useLongPress')
    const open = vi.fn()
    let api
    mount(
      defineComponent({
        setup() {
          api = useLongPress(open)
          return () => null
        },
      }),
    )
    api.start({ touches: [] })
    vi.advanceTimersByTime(1000)
    expect(open).not.toHaveBeenCalled()
  })
})

// ── useOverlayBack ────────────────────────────────────────────────────────────
describe('useOverlayBack', () => {
  it('pushes a history entry when opened and closes on Back (popstate)', async () => {
    const { useOverlayBack } = await import('@/composables/useOverlayBack')
    const isOpen = ref(false)
    const close = vi.fn(() => (isOpen.value = false))
    const pushSpy = vi.spyOn(window.history, 'pushState')

    mount(
      defineComponent({
        setup() {
          useOverlayBack(isOpen, close)
          return () => null
        },
      }),
    )

    isOpen.value = true
    await nextTick()
    expect(pushSpy).toHaveBeenCalled()

    // Simulate the browser Back button while the overlay is open.
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(close).toHaveBeenCalled()
    pushSpy.mockRestore()
  })
})

// ── useTaskMenu ───────────────────────────────────────────────────────────────
import { useTaskMenu } from '@/composables/useTaskMenu'
import { tasks as tasksApi } from '@/api'

describe('useTaskMenu', () => {
  beforeEach(() => vi.clearAllMocks())

  it('builds base options and a move submenu from the given columns', () => {
    const columns = [
      { id: 'c1', name: 'To-Do' },
      { id: 'c2', name: 'Doing' },
    ]
    const { options, open, show, x, y } = useTaskMenu({ columns })
    // open() stamps the target + coordinates (rAF flips show → true later).
    open({ clientX: 5, clientY: 7 }, { id: 't1', column_id: 'c1' })
    expect(x.value).toBe(5)
    expect(y.value).toBe(7)
    expect(show.value).toBe(false) // flips on the next rAF frame

    const keys = options.value.map((o) => o.key)
    expect(keys).toContain('open')
    expect(keys).toContain('toggle')
    expect(keys).toContain('prio')
    // The move submenu excludes the task's current column (c1).
    const move = options.value.find((o) => o.key === 'move')
    expect(move).toBeTruthy()
    const colKeys = move.children.map((c) => c.key)
    expect(colKeys).toEqual(['col:c2'])
  })

  it('omits the move submenu when no other columns exist', () => {
    const { options, open } = useTaskMenu({ columns: [{ id: 'c1', name: 'Only' }] })
    open({ clientX: 0, clientY: 0 }, { id: 't', column_id: 'c1' })
    expect(options.value.find((o) => o.key === 'move')).toBeUndefined()
  })

  it('toggle select calls tasks.update with the flipped completed flag', async () => {
    const onChanged = vi.fn()
    const { open, select } = useTaskMenu({ onChanged })
    open({ clientX: 0, clientY: 0 }, { id: 't9', title: 'X', completed_at: null })
    await select('toggle')
    expect(tasksApi.update).toHaveBeenCalledWith('t9', expect.objectContaining({ completed: true }))
    expect(onChanged).toHaveBeenCalled()
  })

  it('delete select defers to a confirm flag, then confirmDelete calls the api', async () => {
    const onChanged = vi.fn()
    const { open, select, deleteConfirmShow, confirmDelete } = useTaskMenu({ onChanged })
    open({ clientX: 0, clientY: 0 }, { id: 't5', title: 'Y' })
    await select('delete')
    expect(deleteConfirmShow.value).toBe(true)
    await confirmDelete()
    expect(tasksApi.remove).toHaveBeenCalledWith('t5')
  })

  it('col: select moves the task to the chosen column', async () => {
    const { open, select } = useTaskMenu({})
    open({ clientX: 0, clientY: 0 }, { id: 't2', title: 'Z', column_id: 'c1' })
    await select('col:c9')
    expect(tasksApi.move).toHaveBeenCalledWith('t2', {
      column_id: 'c9',
      before_id: null,
      after_id: null,
    })
  })

  // The menu acts on tasks that came from a board/list payload, and those are
  // stripped of their description (backend task_list_dto.go). The update body is
  // full-replace with description as the one tri-state field: sending an empty
  // one would wipe the stored text, so it must be absent entirely.
  it('never sends a description with an inline update', async () => {
    const { open, select } = useTaskMenu({})
    open({ clientX: 0, clientY: 0 }, { id: 't7', title: 'Без описания' })
    await select('toggle')
    await select('prio:3')
    for (const [, body] of tasksApi.update.mock.calls) {
      expect(body).not.toHaveProperty('description')
    }
  })

  it('splices caller items in and routes their keys to onSelect', async () => {
    const onSelect = vi.fn()
    const { options, open, select } = useTaskMenu({
      onSelect,
      extra: (t) => (t.id === 'main' ? [{ label: 'Создать подзадачу', key: 'subtask' }] : []),
    })
    open({ clientX: 0, clientY: 0 }, { id: 'main' })
    expect(options.value.map((o) => o.key)).toContain('subtask')
    await select('subtask')
    expect(onSelect).toHaveBeenCalledWith('subtask', expect.objectContaining({ id: 'main' }))
    // An unknown key must not fall through to an api call.
    expect(tasksApi.update).not.toHaveBeenCalled()

    // A child row gets no "create subtask" item.
    open({ clientX: 0, clientY: 0 }, { id: 'child' })
    expect(options.value.map((o) => o.key)).not.toContain('subtask')
  })
})
