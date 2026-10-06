import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { useBoardAutosave, AUTOSAVE_MS } from '@/composables/useBoardAutosave'

// Autosave re-saves the *named* view 700 ms after the toolbar stops changing. The
// watcher used to bail out of its two "don't save" branches BEFORE touching the
// timer, so a refusal only declined to arm a new one — whatever was already armed
// survived and fired.
//
// That turns «Автосохранение» into a switch that does not hold: the reader edits a
// filter (timer armed), immediately flips the toggle off because they did not mean
// to change the saved view, and 700 ms later the view is rewritten with that edit
// anyway. The same leak makes applying a sibling view emit an extra silent PUT —
// the swap replaces the state the pending save was going to capture, so what it
// sends is no longer what anyone edited. (The layout-swap half of that was already
// covered from the other side, by KanbanBoard refusing a config whose `layout`
// left the view's own.)
//
// Mounted in a real component: the composable registers onBeforeUnmount.
function host(setup) {
  let api
  const wrapper = mount(
    defineComponent({
      setup() {
        api = setup()
        return () => h('div')
      },
    }),
  )
  return { api, wrapper }
}

function mountAutosave() {
  const enabled = ref(true)
  const viewName = ref('Спринт')
  const groupMode = ref('status')
  const guardedFlag = ref(false)
  const save = vi.fn()
  const config = () => ({ layout: 'board', groupMode: groupMode.value })
  const { wrapper } = host(() =>
    useBoardAutosave({
      enabled,
      viewName,
      config,
      save,
      guarded: () => guardedFlag.value,
    }),
  )
  return { wrapper, enabled, viewName, groupMode, guardedFlag, save }
}

describe('useBoardAutosave — a refusal disarms the pending save', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('switching autosave off inside the debounce window cancels the pending write', async () => {
    const { enabled, groupMode, save } = mountAutosave()
    groupMode.value = 'tag'
    await nextTick()

    enabled.value = false
    await nextTick()
    vi.advanceTimersByTime(AUTOSAVE_MS * 2)

    expect(save).not.toHaveBeenCalled()
  })

  it('the loaded view going away cancels it too', async () => {
    // Deleting the view that is loaded clears currentViewName in KanbanBoard.
    const { viewName, groupMode, save } = mountAutosave()
    groupMode.value = 'tag'
    await nextTick()

    viewName.value = ''
    await nextTick()
    vi.advanceTimersByTime(AUTOSAVE_MS * 2)

    expect(save).not.toHaveBeenCalled()
  })

  it('a view load starting after an edit cancels the save that edit armed', async () => {
    const { guardedFlag, groupMode, save } = mountAutosave()
    groupMode.value = 'tag'
    await nextTick()

    // applyView: guard up, the whole toolbar replaced, guard down on nextTick.
    guardedFlag.value = true
    groupMode.value = 'assignee'
    await nextTick()
    guardedFlag.value = false
    vi.advanceTimersByTime(AUTOSAVE_MS * 2)

    expect(save).not.toHaveBeenCalled()
  })

  it('a guarded load does not swallow the edits that follow it', async () => {
    const { guardedFlag, groupMode, save } = mountAutosave()
    guardedFlag.value = true
    groupMode.value = 'tag'
    await nextTick()
    guardedFlag.value = false

    groupMode.value = 'milestone'
    await nextTick()
    vi.advanceTimersByTime(AUTOSAVE_MS)

    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][1].groupMode).toBe('milestone')
  })

  it('switching autosave back on arms again and saves the current state once', async () => {
    const { enabled, groupMode, save } = mountAutosave()
    groupMode.value = 'tag'
    await nextTick()
    enabled.value = false
    await nextTick()

    enabled.value = true
    await nextTick()
    vi.advanceTimersByTime(AUTOSAVE_MS * 2)

    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][0]).toBe('Спринт')
    expect(save.mock.calls[0][1].groupMode).toBe('tag')
  })

  it('an ordinary edit still saves exactly once (the switch stays usable)', async () => {
    const { groupMode, save } = mountAutosave()
    groupMode.value = 'tag'
    await nextTick()
    vi.advanceTimersByTime(AUTOSAVE_MS)

    expect(save).toHaveBeenCalledTimes(1)
    expect(save.mock.calls[0][1].groupMode).toBe('tag')
  })
})
