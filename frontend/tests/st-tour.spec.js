import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { ref, nextTick } from 'vue'

const apiMock = vi.hoisted(() => ({
  acknowledgements: { ack: vi.fn(() => Promise.resolve()), list: vi.fn() },
}))
vi.mock('@/api', () => apiMock)

import { useTourStore, TOUR_DONE, TOUR_SKIPPED, TOUR_STEP_KEY } from '@/stores/tour'
import { anchorSelector } from '@/composables/useTourAnchor'
import { arcPath, arcPathBowed, tourArc, headPoints, layoutArrow } from '@/utils/tourArrow'

const STEPS = [
  { id: 'workspaces', anchor: 'ws-switch', title: 'Пространства', mode: 'info' },
  {
    id: 'open-menu',
    anchor: 'proj-add',
    title: 'Создать',
    mode: 'action',
    advanceOn: { click: 'proj-add' },
  },
  { id: 'done', anchor: 'footer-settings', title: 'Готово', mode: 'info' },
]

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
})
afterEach(() => localStorage.clear())

describe('tour store', () => {
  it('is inert until started', () => {
    const t = useTourStore()
    expect(t.active).toBe(false)
    expect(t.current).toBe(null)
    expect(t.anchors).toEqual([])
  })

  it('refuses to start on an empty scenario', () => {
    const t = useTourStore()
    expect(t.start([])).toBe(false)
    expect(t.active).toBe(false)
  })

  it('walks the steps and acks done at the end', async () => {
    const t = useTourStore()
    t.start(STEPS)
    expect(t.current.id).toBe('workspaces')
    expect(localStorage.getItem(TOUR_STEP_KEY)).toBe('workspaces')

    t.next()
    expect(t.current.id).toBe('open-menu')
    t.next()
    expect(t.current.id).toBe('done')
    expect(t.isLast).toBe(true)

    t.next()
    await nextTick()
    expect(t.active).toBe(false)
    expect(apiMock.acknowledgements.ack).toHaveBeenCalledWith(TOUR_DONE)
    // Nothing left to resume from after a reload.
    expect(localStorage.getItem(TOUR_STEP_KEY)).toBe(null)
  })

  it('resumes from a persisted step id', () => {
    const t = useTourStore()
    t.start(STEPS, { fromId: 'done' })
    expect(t.current.id).toBe('done')
  })

  it('falls back to the first step when the persisted id is gone', () => {
    const t = useTourStore()
    t.start(STEPS, { fromId: 'renamed-away' })
    expect(t.current.id).toBe('workspaces')
  })

  it('skip acks skipped once and ends the tour from any step', async () => {
    const t = useTourStore()
    t.start(STEPS)
    t.next()
    await t.skip()
    expect(t.active).toBe(false)
    expect(apiMock.acknowledgements.ack).toHaveBeenCalledWith(TOUR_SKIPPED)
    // A second skip is a no-op, not a second write.
    await t.skip()
    expect(apiMock.acknowledgements.ack).toHaveBeenCalledTimes(1)
  })

  it('survives a failed ack (offline)', async () => {
    apiMock.acknowledgements.ack.mockRejectedValueOnce(new Error('offline'))
    const t = useTourStore()
    t.start(STEPS)
    await expect(t.skip()).resolves.toBeUndefined()
    expect(t.active).toBe(false)
  })

  it('exposes the primary anchor first, then the extras', () => {
    const t = useTourStore()
    t.start([{ id: 'card', anchor: 'card-priority', extra: ['card-due', 'card-tags'] }])
    expect(t.anchors).toEqual(['card-priority', 'card-due', 'card-tags'])
  })

  it('scopes {project}/{board} tokens to the entity the user just created', () => {
    // #2753 rework: the board-add/board-open steps must point at the row the
    // guide created, not the first one in the tree.
    const t = useTourStore()
    const tokenised = '[data-tour-project="{project}"] [data-tour="board-add"]'
    t.start([{ id: 'board-add', anchor: tokenised, mode: 'action' }])

    // No id yet → the token collapses to a selector that matches nothing, so the
    // step waits instead of grabbing a stray element.
    expect(t.anchors).toEqual(['[data-tour-project=""] [data-tour="board-add"]'])

    t.noteCreated({ projectId: 'p-42' })
    expect(t.anchors).toEqual(['[data-tour-project="p-42"] [data-tour="board-add"]'])
    expect(t.resolve('[data-tour-board="{board}"]')).toBe('[data-tour-board=""]')

    t.noteCreated({ boardId: 'b-7' })
    expect(t.resolve('[data-tour-board="{board}"]')).toBe('[data-tour-board="b-7"]')
  })

  it('drops the created-entity context when the tour ends', () => {
    const t = useTourStore()
    t.start([{ id: 'board-add', anchor: 'x', mode: 'action' }])
    t.noteCreated({ projectId: 'p-42' })
    t.stop()
    // A fresh run must not inherit the previous walk's project.
    t.start([{ id: 'board-add', anchor: '[data-tour-project="{project}"]', mode: 'action' }])
    expect(t.anchors).toEqual(['[data-tour-project=""]'])
  })

  describe('advancing', () => {
    it('advances an action step on a click on its anchor', () => {
      const t = useTourStore()
      t.start(STEPS)
      t.next()
      t.clicked('open-menu')
      expect(t.current.id).toBe('done')
    })

    it('ignores a click reported for a step that is no longer current', () => {
      const t = useTourStore()
      t.start(STEPS)
      t.clicked('open-menu')
      expect(t.current.id).toBe('workspaces')
    })

    it('waits for the entity, not the click, when the step declares when()', async () => {
      const projects = ref([])
      const t = useTourStore()
      t.start([
        {
          id: 'create-project',
          anchor: 'menu-project',
          mode: 'action',
          advanceOn: {
            click: 'menu-project',
            snapshot: () => projects.value.length,
            when: (base) => projects.value.length > base,
          },
        },
        { id: 'after', anchor: 'node-add', mode: 'info' },
      ])

      // Clicking the menu item is not enough — the modal may still be cancelled.
      t.clicked('create-project')
      await nextTick()
      expect(t.current.id).toBe('create-project')

      projects.value = [{ id: 1 }]
      await nextTick()
      expect(t.current.id).toBe('after')
    })

    it('does not fire when() on a value that was already true on entry', async () => {
      // A user who restarts the guide with projects already in the tree must
      // still be walked through creating one — the baseline is per-step.
      const projects = ref([{ id: 1 }])
      const t = useTourStore()
      t.start([
        {
          id: 'create-project',
          anchor: 'menu-project',
          mode: 'action',
          advanceOn: {
            snapshot: () => projects.value.length,
            when: (base) => projects.value.length > base,
          },
        },
        { id: 'after', anchor: 'node-add', mode: 'info' },
      ])
      await nextTick()
      expect(t.current.id).toBe('create-project')

      projects.value = [{ id: 1 }, { id: 2 }]
      await nextTick()
      expect(t.current.id).toBe('after')
    })

    describe('advanceOn.count', () => {
      const COUNT_STEPS = [
        {
          id: 'create-board',
          anchor: 'board-name',
          mode: 'action',
          advanceOn: { count: 'board-row' },
        },
        { id: 'after', anchor: 'board-row', mode: 'info' },
      ]

      it('takes the first report as the baseline and advances when it grows', () => {
        const t = useTourStore()
        t.start(COUNT_STEPS)
        t.counted('create-board', 0)
        expect(t.current.id).toBe('create-board')
        t.counted('create-board', 1)
        expect(t.current.id).toBe('after')
      })

      it('does not advance on a count that was already there on entry', () => {
        // Re-running the guide with boards already in the tree still walks the
        // user through creating one.
        const t = useTourStore()
        t.start(COUNT_STEPS)
        t.counted('create-board', 3)
        t.counted('create-board', 3)
        expect(t.current.id).toBe('create-board')
        t.counted('create-board', 4)
        expect(t.current.id).toBe('after')
      })

      it('re-baselines on every step, so one report cannot advance two', () => {
        const t = useTourStore()
        t.start([
          { ...COUNT_STEPS[0], id: 'one' },
          { ...COUNT_STEPS[0], id: 'two' },
          COUNT_STEPS[1],
        ])
        t.counted('one', 0)
        t.counted('one', 1)
        expect(t.current.id).toBe('two')
        t.counted('two', 1)
        expect(t.current.id).toBe('two')
        t.counted('two', 2)
        expect(t.current.id).toBe('after')
      })

      it('ignores reports for another step and steps without a count', () => {
        const t = useTourStore()
        t.start(COUNT_STEPS)
        t.counted('after', 9)
        t.counted('create-board', undefined)
        expect(t.current.id).toBe('create-board')

        t.start(STEPS) // 'workspaces' is an info step, no advanceOn at all
        t.counted('workspaces', 0)
        t.counted('workspaces', 5)
        expect(t.current.id).toBe('workspaces')
      })
    })

    describe('advanceOn.set', () => {
      const SET_STEPS = [
        {
          id: 'tm-due',
          anchor: 'tm-due',
          mode: 'action',
          advanceOn: { set: '[data-tour="tm-due"][data-tour-set]' },
        },
        { id: 'after', anchor: 'tm-save', mode: 'info' },
      ]

      it('advances as soon as the field carries a value', () => {
        const t = useTourStore()
        t.start(SET_STEPS)
        t.counted('tm-due', 0)
        expect(t.current.id).toBe('tm-due')
        t.counted('tm-due', 1)
        expect(t.current.id).toBe('after')
      })

      it('takes no baseline, so an already-filled field cannot deadlock', () => {
        // Unlike count: a task that already has a due date would otherwise pin
        // the step at 1 forever, leaving only «Пропустить» to escape.
        const t = useTourStore()
        t.start(SET_STEPS)
        t.counted('tm-due', 1)
        expect(t.current.id).toBe('after')
      })
    })

    describe('advanceOn.moved', () => {
      const MOVED = {
        el: '[data-testid="task-card"]',
        within: '[data-column-name]',
        by: 'data-column-name',
      }
      const MOVED_STEPS = [
        { id: 'dnd-card', anchor: 'card', mode: 'action', advanceOn: { moved: MOVED } },
        { id: 'after', anchor: 'ws-switch', mode: 'info' },
      ]
      const COUNT_ONLY = [
        { id: 'create-board', anchor: 'board-name', mode: 'action', advanceOn: { count: 'row' } },
        { id: 'after', anchor: 'ws-switch', mode: 'info' },
      ]

      it('takes the first address as the baseline and advances when it changes', () => {
        const t = useTourStore()
        t.start(MOVED_STEPS)
        t.located('dnd-card', 'К работе')
        expect(t.current.id).toBe('dnd-card')
        // Same address again — the user picked the card up and put it back.
        t.located('dnd-card', 'К работе')
        expect(t.current.id).toBe('dnd-card')

        t.located('dnd-card', 'В процессе')
        expect(t.current.id).toBe('after')
      })

      it('ignores an empty address mid-drag', () => {
        // SortableJS lifts the node out of its list for a frame, so closest()
        // finds no container — advancing there would end the step while the card
        // is still in the air.
        const t = useTourStore()
        t.start(MOVED_STEPS)
        t.located('dnd-card', 'К работе')
        t.located('dnd-card', null)
        t.located('dnd-card', '')
        expect(t.current.id).toBe('dnd-card')
        t.located('dnd-card', 'В процессе')
        expect(t.current.id).toBe('after')
      })

      it('treats "no container" as a legitimate baseline (a project at the root)', () => {
        // The tree step starts with the project outside any group, i.e. with no
        // address at all — moving it into one has to count.
        const t = useTourStore()
        t.start([
          { id: 'dnd-project', anchor: 'row', mode: 'action', advanceOn: { moved: MOVED } },
          MOVED_STEPS[1],
        ])
        t.located('dnd-project', null)
        expect(t.current.id).toBe('dnd-project')
        t.located('dnd-project', 'g-1')
        expect(t.current.id).toBe('after')
      })

      it('re-baselines per step, so one report cannot advance two', () => {
        const t = useTourStore()
        t.start([
          { ...MOVED_STEPS[0], id: 'one' },
          { ...MOVED_STEPS[0], id: 'two' },
          MOVED_STEPS[1],
        ])
        t.located('one', 'a')
        t.located('one', 'b')
        expect(t.current.id).toBe('two')
        t.located('two', 'b')
        expect(t.current.id).toBe('two')
        t.located('two', 'c')
        expect(t.current.id).toBe('after')
      })

      it('ignores reports for another step, and steps that do not wait on a move', () => {
        const t = useTourStore()
        t.start(MOVED_STEPS)
        t.located('after', 'В процессе')
        expect(t.current.id).toBe('dnd-card')

        t.start(STEPS) // info step, no advanceOn at all
        t.located('workspaces', 'a')
        t.located('workspaces', 'b')
        expect(t.current.id).toBe('workspaces')

        // An action step that advances on something else must not move either.
        t.start(COUNT_ONLY)
        t.located('create-board', 'a')
        t.located('create-board', 'b')
        expect(t.current.id).toBe('create-board')
      })
    })

    it('skips a step whose anchor never showed up instead of hanging', () => {
      const t = useTourStore()
      t.start(STEPS)
      t.anchorMissing('workspaces')
      expect(t.current.id).toBe('open-menu')
      // A stale report from an already-left step changes nothing.
      t.anchorMissing('workspaces')
      expect(t.current.id).toBe('open-menu')
    })
  })
})

describe('anchorSelector', () => {
  it('wraps a bare key into a data-tour selector', () => {
    expect(anchorSelector('ws-switch')).toBe('[data-tour="ws-switch"]')
  })

  it('passes a raw CSS selector through', () => {
    const raw = '[data-column-name="К работе"] [data-testid="add-task-button"]'
    expect(anchorSelector(raw)).toBe(raw)
    expect(anchorSelector('.n-tabs [data-name="comments"]')).toBe('.n-tabs [data-name="comments"]')
  })

  it('is empty for a missing key', () => {
    expect(anchorSelector('')).toBe('')
    expect(anchorSelector(undefined)).toBe('')
  })
})

describe('tour arrow geometry', () => {
  it('draws a cubic from the popover to the target', () => {
    const d = arcPath({ x: 200, y: 120 }, { x: 60, y: 40 })
    expect(d).toMatch(/^M 200 120 C .+, .+, 60 40$/)
  })

  it('bows the same amount whichever side the popover sits on', () => {
    // A popover far above the target must not inherit a runaway bow from an
    // unsigned Math.min clamp.
    const below = arcPath({ x: 200, y: 900 }, { x: 60, y: 40 })
    const above = arcPath({ x: 200, y: 40 }, { x: 60, y: 900 })
    // first control point: "C cx1 cy1, …" — its y is the bowed one
    const bowOf = (d, y) => Math.abs(parseFloat(d.split('C ')[1].split(' ')[1]) - y)
    expect(bowOf(below, 900)).toBeCloseTo(20)
    expect(bowOf(above, 40)).toBeCloseTo(20)
  })

  it('bows the tour arrow off the chord even when it is horizontal', () => {
    // #2753 rework: a straight horizontal stub read as "an arrowhead from
    // nowhere". arcPathBowed curves it off the chord so it's a visible line.
    const d = arcPathBowed({ x: 100, y: 200 }, { x: 260, y: 200 })
    expect(d).toMatch(/^M 100 200 C /)
    // control points sit off the y=200 chord (perpendicular bow)
    const c = d.split('C ')[1].split(/[ ,]+/).map(Number)
    expect(Math.abs(c[1] - 200)).toBeGreaterThan(4)
    expect(Math.abs(c[3] - 200)).toBeGreaterThan(4)
    // starts and ends exactly on the endpoints
    expect(d.endsWith('260 200')).toBe(true)
  })

  it('makes the tour arrow arrive along the popover side (head points at target)', () => {
    // #2753 rework: for a target far off to the side, the head must point square
    // at it (up when the popover is below), not skid in diagonally. tourArc puts
    // the end control point straight out from the tip along the side axis.
    const from = { x: 680, y: 340 } // popover top edge, centred
    const to = { x: 250, y: 60 } // target far up-left (the layout switch)
    const c = tourArc(from, to, 'bottom').split('C ')[1].split(/[ ,]+/).map(Number)
    expect(c[2]).toBe(to.x) // c2.x == tip.x → vertical final approach → head up
    expect(c[3]).toBeGreaterThan(to.y)
    // but it still leaves the popover bowed off the chord (visible, non-degenerate)
    expect(c[0]).not.toBe(from.x)
  })

  it('builds a triangle around the tip', () => {
    const pts = headPoints({ x: 10, y: 0 }, { x: 0, y: 0 }, 5)
      .split(' ')
      .map((p) => p.split(',').map(Number))
    expect(pts[0]).toEqual([10, 0])
    // Base corners straddle the path, 2*halfW apart.
    expect(Math.abs(pts[1][1] - pts[2][1])).toBeCloseTo(10)
  })

  it('degrades to no head when the path cannot be measured (jsdom)', () => {
    const fake = { setAttribute: vi.fn() }
    expect(layoutArrow(fake, { x: 0, y: 0 }, { x: 10, y: 10 })).toEqual({ len: 0, head: '' })
    expect(fake.setAttribute).toHaveBeenCalledWith('d', expect.stringContaining('M 0 0 C'))
  })

  it('measures the head from a real path', () => {
    const path = {
      setAttribute: vi.fn(),
      getTotalLength: () => 100,
      getPointAtLength: (l) => ({ x: l, y: 0 }),
    }
    const { len, head } = layoutArrow(path, { x: 0, y: 0 }, { x: 100, y: 0 })
    expect(len).toBe(100)
    expect(head.split(' ')[0]).toBe('100,0')
  })
})
