import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { ref, defineComponent, h, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

import { useBoardAutosave, AUTOSAVE_MS } from '@/composables/useBoardAutosave'

// Regression test for #2932 — the loaded saved view followed you onto the next board.
//
// Saved views are strictly per-board (`boards.views(boardId)`), but KanbanBoard does
// NOT remount when you switch boards: `watch(() => props.boardId)` re-runs restore +
// load on the same instance. `currentViewName` / `currentViewLayout` were never reset
// there, so board B inherited board A's loaded view:
//
//   • the views button stayed primary with «Текущий вид: <A's view>», and the popover
//     highlighted a same-named stranger;
//   • `saveView` defaults its name to `currentViewName`, so Save on B silently
//     proposed A's name;
//   • `autosaveView` is per-board localStorage, so it can be ON here — the first
//     toolbar change on B then PUT a view named after A's onto B;
//   • the autosave layout guard compared against A's `currentViewLayout`.
//
// KanbanBoard is never mounted in this suite (it pulls the whole board stack), so the
// reset and the watcher are read out of the source the way
// cx-view-load-keeps-collapse-overrides.spec.js does; the disarm the ordering rests on
// is exercised against the real `useBoardAutosave` below. Vitest runs with frontend/
// as cwd; __dirname doesn't exist in ESM.
const root = process.cwd()
const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// Body of a function declaration, by brace matching from its `function <name>(`.
function fnBody(source, name) {
  const start = source.indexOf(`function ${name}(`)
  if (start === -1) return null
  const open = source.indexOf('{', start)
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

// Body of the `watch(() => props.boardId, async (id) => { … })` callback.
function boardIdWatcher(source) {
  const at = source.indexOf('() => props.boardId')
  if (at === -1) return null
  const open = source.indexOf('{', source.indexOf('=>', source.indexOf('(id)', at)))
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}' && --depth === 0) return source.slice(open + 1, i)
  }
  return null
}

describe('a board switch drops the loaded saved view (#2932)', () => {
  it('has a reset for everything the loaded view owns', () => {
    const body = fnBody(board, 'clearLoadedView')
    expect(body, 'clearLoadedView() must exist').toBeTruthy()
    // The name is what the button shows and what Save defaults to; the layout gates
    // autosave; the list belongs to the board it was fetched for.
    expect(body).toMatch(/currentViewName\.value = ''/)
    expect(body).toMatch(/currentViewLayout\.value = ''/)
    expect(body).toMatch(/savedViews\.value = \[\]/)
  })

  it('also drops a dialog aimed at the board being left', () => {
    const body = fnBody(board, 'clearLoadedView')
    // `saveView` posts to `props.boardId` at call time: a save popover left open over
    // the switch would write the name typed for the other board onto this one.
    expect(body).toMatch(/newViewName\.value = ''/)
    expect(body).toMatch(/showSaveView\.value = false/)
    expect(body).toMatch(/showLoadView\.value = false/)
  })

  it('the boardId watcher calls it', () => {
    const body = boardIdWatcher(board)
    expect(body, 'watch(() => props.boardId, …) must exist').toBeTruthy()
    expect(body).toMatch(/clearLoadedView\(\)/)
  })

  it('calls it before the toolbar is restored', () => {
    const body = boardIdWatcher(board)
    // Clearing the name empties the autosave signature, which disarms a write armed
    // on the board we are leaving instead of letting it land on this one.
    expect(body.indexOf('clearLoadedView()')).toBeLessThan(body.indexOf('restoreView()'))
  })

  it('reloads the list it just emptied', () => {
    const body = boardIdWatcher(board)
    expect(body.indexOf('clearLoadedView()')).toBeLessThan(body.indexOf('loadViews()'))
  })

  // ── the disarm the ordering above rests on, against the real composable ──
  describe('clearing the name stands autosave down', () => {
    function mountAutosave() {
      const enabled = ref(true)
      const viewName = ref('Спринт')
      const state = ref('status')
      const save = vi.fn()
      const wrapper = mount(
        defineComponent({
          setup() {
            useBoardAutosave({
              enabled,
              viewName,
              config: () => ({ groupMode: state.value }),
              save,
              guarded: () => false,
            })
            return () => h('div')
          },
        }),
      )
      return { enabled, viewName, state, save, wrapper }
    }

    beforeEach(() => vi.useFakeTimers())
    afterEach(() => vi.useRealTimers())

    it('a write armed on the previous board never lands', async () => {
      const { viewName, state, save } = mountAutosave()
      state.value = 'assignee' // last toolbar change on board A
      await nextTick()
      viewName.value = '' // the switch: clearLoadedView()
      await nextTick()
      vi.advanceTimersByTime(AUTOSAVE_MS * 2)
      expect(save).not.toHaveBeenCalled()
    })

    it('and edits on the new board save nothing until a view is loaded', async () => {
      const { viewName, state, save } = mountAutosave()
      viewName.value = ''
      await nextTick()
      state.value = 'assignee' // board B's own toolbar change, autosave still on
      await nextTick()
      vi.advanceTimersByTime(AUTOSAVE_MS * 2)
      expect(save).not.toHaveBeenCalled()
    })

    it('a view loaded on the new board autosaves normally again', async () => {
      const { viewName, state, save } = mountAutosave()
      viewName.value = ''
      await nextTick()
      viewName.value = 'Мой вид' // applyView on board B
      await nextTick()
      state.value = 'assignee'
      await nextTick()
      vi.advanceTimersByTime(AUTOSAVE_MS)
      expect(save).toHaveBeenCalledTimes(1)
      expect(save.mock.calls[0][0]).toBe('Мой вид')
    })
  })
})
