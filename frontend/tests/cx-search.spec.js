import { describe, it, expect, vi, beforeEach } from 'vitest'
import { reactive } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

// The workspace search dropdown (#2932).
//
// Everything this file asserts is a way for the panel to lie quietly, which is
// why it is a test and not a reading — each failure mode renders as a perfectly
// ordinary-looking dropdown:
//
//   - a request that failed drew "Ничего не найдено". The reader concludes the
//     task does not exist, when the server was down, the session had expired, or
//     the laptop was on a train. This is the only screen in the app where an
//     error is indistinguishable from an answer.
//   - a search fired before the workspace id was known left the spinner running
//     forever: the early return skipped the one line that clears it, and nothing
//     retries until the next keystroke.
//   - the server caps every kind at 25 hits. A panel that shows the cap with no
//     sign of it presents an arbitrary slice, ordered by task number, as the
//     whole answer.
//   - the box lives in the topbar and is never remounted, so after a workspace
//     switch it kept showing the previous workspace's hits. Rows that open a
//     board the reader can no longer reach, under a term that was never asked of
//     the workspace they are now in.
//
// The happy path is asserted alongside each of them: a fix that painted an error
// banner over every successful search would satisfy the first assertion on its
// own.

const search = vi.fn()
vi.mock('@/api', () => ({ workspaces: { search: (...a) => search(...a) } }))

const push = vi.fn()
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

// The two stores this component reads. `currentId` is a plain property rather
// than a ref so a spec can move it between mounts — but the object is reactive,
// because the component watches that property: a switch has to reach it the way
// the real Pinia store's does.
const ws = reactive({ currentId: 'ws-1' })
vi.mock('@/stores/workspaces', () => ({ useWorkspacesStore: () => ws }))
vi.mock('@/stores/help', () => ({
  useHelpStore: () => ({ find: () => [], openDrawer: vi.fn() }),
}))

const { default: SearchBar } = await import('@/components/SearchBar.vue')

// hit builds one task row of the shape the endpoint returns.
function hit(n) {
  return { id: 'task-' + n, board_id: 'b1', number: n, title: 'Задача ' + n }
}

function answer(tasks, more = {}) {
  return {
    data: {
      tasks,
      notes: [],
      documents: [],
      has_more: { tasks: false, notes: false, documents: false, ...more },
    },
  }
}

// type drives the component the way a user does: the watcher debounces by 220ms,
// so the timers have to be run forward before anything has been asked of the API.
async function type(w, term) {
  w.vm.q = term
  await flushPromises()
  vi.advanceTimersByTime(300)
  await flushPromises()
  await flushPromises()
}

describe('SearchBar', () => {
  beforeEach(() => {
    // v2 (#2954/#2957): the box also reads the board view and palette stores.
    setActivePinia(createPinia())
    vi.useFakeTimers()
    search.mockReset()
    push.mockReset()
    ws.currentId = 'ws-1'
  })

  it('says the search failed instead of saying there is nothing', async () => {
    // What the api interceptor hands a caller: a fresh Error whose message is
    // already humanized, with no .response (see api/index.js).
    const err = new Error('Что-то пошло не так')
    err.status = 500
    search.mockRejectedValue(err)

    const w = mount(SearchBar)
    await type(w, 'отчёт')

    const text = w.text()
    expect(text).not.toContain('Ничего не найдено')
    expect(text).toContain('Что-то пошло не так')
    w.unmount()
  })

  it('names the network, not the result set, when the request never left', async () => {
    const err = new Error('Нет соединения с сервером')
    err.offline = true
    search.mockRejectedValue(err)

    const w = mount(SearchBar)
    await type(w, 'отчёт')

    expect(w.text()).not.toContain('Ничего не найдено')
    expect(w.text()).toContain('Нет соединения')
    w.unmount()
  })

  it('still says "nothing found" when the server really found nothing', async () => {
    // The negative control for both cases above: an empty answer is an answer.
    search.mockResolvedValue(answer([]))

    const w = mount(SearchBar)
    await type(w, 'мираж')

    expect(w.text()).toContain('Ничего не найдено')
    w.unmount()
  })

  it('drops the error the moment a later search succeeds', async () => {
    search.mockRejectedValueOnce(new Error('Что-то пошло не так'))
    search.mockResolvedValue(answer([hit(7)]))

    const w = mount(SearchBar)
    await type(w, 'отч')
    expect(w.text()).toContain('Что-то пошло не так')

    await type(w, 'отчёт')
    expect(w.text()).not.toContain('Что-то пошло не так')
    expect(w.text()).toContain('Задача 7')
    w.unmount()
  })

  it('does not spin forever when no workspace is selected yet', async () => {
    // The shell mounts the search box before the workspace list has landed, so
    // this is the first thing an early typist meets.
    ws.currentId = ''
    search.mockResolvedValue(answer([]))

    const w = mount(SearchBar)
    await type(w, 'отчёт')

    expect(search).not.toHaveBeenCalled()
    // The spinner's own element, not the component: `findComponent({name})`
    // does not resolve a globally-registered naive component here and would
    // report false whatever the panel is actually showing.
    expect(w.find('.loading').exists()).toBe(false)
    w.unmount()
  })

  it('marks a page that filled up, and leaves a short one alone', async () => {
    const full = Array.from({ length: 25 }, (_, i) => hit(i + 1))
    search.mockResolvedValue(answer(full, { tasks: true }))

    const w = mount(SearchBar)
    await type(w, 'а')

    // A dedicated element, not the digits: twenty-five rows titled "Задача 25"
    // put the number on screen all by themselves.
    expect(w.find('.more').exists()).toBe(true)
    w.unmount()

    search.mockResolvedValue(answer([hit(1), hit(2)]))
    const w2 = mount(SearchBar)
    await type(w2, 'ровно два')
    expect(w2.find('.more').exists()).toBe(false)
    w2.unmount()
  })

  it('does not keep the previous workspace on screen after a switch', async () => {
    search.mockResolvedValue(answer([hit(7)]))

    const w = mount(SearchBar)
    await type(w, 'отчёт')
    expect(w.text()).toContain('Задача 7')

    // The reader clicks the workspace switcher. The component is not remounted —
    // it lives in the topbar — and the term stays in the box.
    search.mockResolvedValue(answer([hit(42)]))
    ws.currentId = 'ws-2'
    await flushPromises()
    vi.advanceTimersByTime(300)
    await flushPromises()
    await flushPromises()

    expect(w.text()).not.toContain('Задача 7')
    expect(w.text()).toContain('Задача 42')
    // And the re-ask went to the workspace the reader is now in.
    expect(search).toHaveBeenLastCalledWith('ws-2', 'отчёт')
    w.unmount()
  })

  it('drops the answer to a request the workspace switch outran', async () => {
    // The race the switch also has to settle: a request sent to the old
    // workspace resolves *after* the switch. Without the sequence bump its rows
    // land in a panel that now belongs to a different workspace.
    let release
    search.mockImplementationOnce(
      () => new Promise((res) => (release = () => res(answer([hit(7)])))),
    )

    const w = mount(SearchBar)
    await type(w, 'отчёт')
    expect(search).toHaveBeenCalledTimes(1)

    search.mockResolvedValue(answer([hit(42)]))
    ws.currentId = 'ws-2'
    await flushPromises()

    release()
    await flushPromises()
    vi.advanceTimersByTime(300)
    await flushPromises()
    await flushPromises()

    expect(w.text()).not.toContain('Задача 7')
    expect(w.text()).toContain('Задача 42')
    w.unmount()
  })

  it('clears the panel on a switch even with an empty box', async () => {
    // Clearing the box leaves `q` empty; a later switch must not fire a request
    // for an empty term, and must not leave the spinner up either.
    search.mockResolvedValue(answer([hit(7)]))
    const w = mount(SearchBar)
    await type(w, 'отчёт')
    await type(w, '')
    search.mockClear()

    ws.currentId = 'ws-2'
    await flushPromises()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(search).not.toHaveBeenCalled()
    expect(w.find('.loading').exists()).toBe(false)
    w.unmount()
  })
})
