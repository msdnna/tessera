// useBoardViews (#2987, #3015, #3016): the loaded view and its «изменено»; ⌘S
// updates it, a locked one becomes «Сохранить как моё»; ⌘⌫ resets; 1–9 / 0
// pick; the star applies once per browser session; ⌫ deletes at once with an
// undo that puts the view back in its place.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent, h, reactive, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  views: vi.fn(),
  saveView: vi.fn(),
  updateView: vi.fn(),
  deleteView: vi.fn(),
}))
vi.mock('@/api', () => ({ boards: api }))

const { useBoardViews } = await import('@/composables/useBoardViews')
const { useUndoStore } = await import('@/stores/undo')

const VIEWS = [
  {
    id: 'm1',
    name: 'Мой спринт',
    scope: 'user',
    position: 1,
    editable: true,
    is_default: true,
    config: { layout: 'board', cardSize: 'medium' },
  },
  {
    id: 'm2',
    name: 'Только баги',
    scope: 'user',
    position: 2,
    editable: true,
    config: { layout: 'list' },
  },
  {
    id: 's1',
    name: 'Релизный Гант',
    scope: 'board',
    position: 1,
    editable: false,
    config: { layout: 'gantt' },
  },
]

function setup() {
  const state = reactive({ cfg: { layout: 'board', cardSize: 'medium' } })
  const applied = []
  let bv
  const w = mount(
    defineComponent({
      setup() {
        bv = reactive(
          useBoardViews({
            boardId: { value: 'b1' },
            currentConfig: () => ({ ...state.cfg }),
            applyConfig: (c) => {
              applied.push(c)
              state.cfg = { ...c }
            },
          }),
        )
        return () => h('div')
      },
    }),
  )
  return { w, bv, state, applied }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  sessionStorage.clear()
  for (const f of Object.values(api)) f.mockReset()
  api.views.mockResolvedValue({ data: VIEWS.map((v) => ({ ...v })) })
})

describe('useBoardViews', () => {
  it('applies the starred view on the first open of a session, not on the next', async () => {
    const { bv, applied, w } = setup()
    await bv.load()
    expect(bv.onOpen()).toBe(true)
    expect(bv.loadedId).toBe('m1')
    expect(applied).toHaveLength(1)
    // Same session, the board is opened again: the board keeps its own state.
    bv.forget()
    await bv.load()
    expect(bv.onOpen()).toBe(false)
    expect(bv.loadedId).toBe('m1') // remembered locally
    expect(applied).toHaveLength(1)
    w.unmount()
  })

  it('reads «изменено» off the board’s state and names what moved', async () => {
    const { bv, state, w } = setup()
    await bv.load()
    bv.apply(bv.views[0])
    expect(bv.changed).toBe(false)
    state.cfg = { ...state.cfg, cardSize: 'large' }
    await nextTick()
    expect(bv.changed).toBe(true)
    expect(bv.changes).toEqual([{ area: 'cards' }])
    bv.reset()
    expect(state.cfg.cardSize).toBe('medium')
    expect(bv.changed).toBe(false)
    w.unmount()
  })

  it('⌘S re-saves an editable view; a locked one asks for a name instead', async () => {
    const { bv, state, w } = setup()
    await bv.load()
    bv.apply(bv.views[0])
    state.cfg = { ...state.cfg, cardSize: 'compact' }
    api.updateView.mockResolvedValue({ data: { ...VIEWS[0], config: { ...state.cfg } } })
    await bv.update()
    expect(api.updateView).toHaveBeenCalledWith('m1', { config: state.cfg })
    expect(bv.changed).toBe(false)

    bv.apply(bv.views.find((v) => v.id === 's1'))
    state.cfg = { ...state.cfg, groupMode: 'tag' }
    await bv.update()
    expect(api.updateView).toHaveBeenCalledTimes(1)
    expect(bv.saveAsDraft).toMatchObject({ name: 'Релизный Гант' })
    w.unmount()
  })

  it('picks by place, 0 drops the view but keeps the board as it is', async () => {
    const { bv, applied, w } = setup()
    await bv.load()
    bv.pick(2)
    expect(bv.loadedId).toBe('m2')
    expect(applied.at(-1).layout).toBe('list')
    bv.pick(0)
    expect(bv.loadedId).toBe('')
    expect(applied).toHaveLength(1)
    w.unmount()
  })

  it('refuses a rename into a taken name without a request', async () => {
    const { bv, w } = setup()
    await bv.load()
    const err = await bv.rename(bv.views[1], 'мой СПРИНТ')
    expect(err).toBeTruthy()
    expect(api.updateView).not.toHaveBeenCalled()
    w.unmount()
  })

  it('deletes at once and the undo puts the view back between its neighbours', async () => {
    const { bv, w } = setup()
    await bv.load()
    bv.apply(bv.views[0])
    api.deleteView.mockResolvedValue({})
    await bv.remove(bv.views[0])
    expect(api.deleteView).toHaveBeenCalledWith('m1')
    expect(bv.views.map((v) => v.id)).toEqual(['m2', 's1'])
    expect(bv.loadedId).toBe('')

    const undo = useUndoStore()
    expect(undo.toasts).toHaveLength(1)
    api.saveView.mockResolvedValue({ data: { ...VIEWS[0], id: 'm1b' } })
    api.updateView.mockResolvedValue({ data: {} })
    api.views.mockResolvedValue({ data: [{ ...VIEWS[0], id: 'm1b' }, VIEWS[1], VIEWS[2]] })
    await undo.undo(undo.toasts[0].id)
    expect(api.saveView).toHaveBeenCalledWith('b1', {
      name: 'Мой спринт',
      scope: 'user',
      config: VIEWS[0].config,
      is_default: true,
    })
    expect(api.updateView).toHaveBeenCalledWith('m1b', { before_id: null, after_id: 'm2' })
    expect(bv.loadedId).toBe('m1b')
    w.unmount()
  })

  it('moves a view one step inside its list', async () => {
    const { bv, w } = setup()
    await bv.load()
    api.updateView.mockResolvedValue({ data: { ...VIEWS[1], position: 0.5 } })
    await bv.move(bv.views[1], -1)
    expect(api.updateView).toHaveBeenCalledWith('m2', { before_id: null, after_id: 'm1' })
    w.unmount()
  })

  it('leaves the star alone when the reader changed the board while it loaded', async () => {
    const { bv, applied, w } = setup()
    await bv.load()
    expect(bv.onOpen({ applyDefault: false })).toBe(false)
    expect(applied).toHaveLength(0)
    w.unmount()
  })
})
