// Dropping a dependency arrow on the Gantt is destructive and undoable nowhere,
// so it has to ask first — like the same unlink does from the task modal's
// «Связи» tab (n-popconfirm on web, TConfirmPopover on Android).
//
// The ✕ here is an 8px SVG circle that appears on hover at the *midpoint* of the
// S-curve — exactly where crossing arrows overlap — and it used to call the
// delete straight from @click. These pin both halves: the click only arms the
// confirmation, and the relation goes only after it is accepted (#2932).
//
// The popconfirm is asserted through its component instance, not the DOM: it is
// anchored to the pointer (an SVG <g> cannot host Naive's trigger slot) and its
// body teleports out of the chart.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { NPopconfirm } from 'naive-ui'

const dependencies = vi.fn()
const removeRelation = vi.fn()
vi.mock('@/api', () => ({
  tasks: { update: vi.fn(), addRelation: vi.fn(), removeRelation: (...a) => removeRelation(...a) },
  boards: { dependencies: (...a) => dependencies(...a) },
}))

import BoardGanttView from '@/components/BoardGanttView.vue'

const stubs = {
  NIcon: { template: '<i />' },
  NTooltip: { template: '<div><slot name="trigger" /><slot /></div>' },
  NDropdown: { template: '<div><slot /></div>' },
  NPopover: { template: '<div><slot name="trigger" /><slot /></div>' },
}

const task = (id, number) => ({
  id,
  number,
  title: 'Задача ' + number,
  priority: 2,
  start_date: '2026-01-05T00:00:00Z',
  due_date: '2026-01-09T00:00:00Z',
})

function mountGantt(props = {}) {
  return mount(BoardGanttView, {
    props: {
      boardId: 'b1',
      tasks: [task('t1', 1), task('t2', 2)],
      statusColumns: [{ id: 'c1', name: 'К работе' }],
      ...props,
    },
    global: { stubs },
  })
}

// One blocking edge t1 → t2, so exactly one arrow (and one ✕) is drawn.
const EDGE = { id: 'r1', task_id: 't1', related_task_id: 't2', kind: 'blocks' }

// The chart's own unlink prompt. ChartTaskMenu keeps two more popconfirms mounted
// (delete / archive a task), but those live in a child at the end of the tree —
// ours is the one inside the chart body, next to the arrow overlay it belongs to.
function unlinkPrompt(w) {
  return w.find('.tl-body').findComponent(NPopconfirm)
}

describe('Gantt edge unlink confirmation', () => {
  beforeEach(() => {
    // The v2 Gantt reads the viewport through useResponsive; jsdom has no
    // matchMedia at all. Desktop width — these cases are about the edges.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    setActivePinia(createPinia())
    dependencies.mockReset()
    removeRelation.mockReset()
    dependencies.mockResolvedValue({ data: [EDGE] })
    removeRelation.mockResolvedValue({})
  })

  it('does not drop the relation on the bare ✕ click', async () => {
    const w = mountGantt()
    await nextTick()
    await nextTick()

    const del = w.find('.g-del')
    expect(del.exists()).toBe(true)
    expect(unlinkPrompt(w).props('show')).toBe(false)

    await del.trigger('click')
    await nextTick()

    expect(removeRelation).not.toHaveBeenCalled()
    expect(unlinkPrompt(w).props('show')).toBe(true)
    w.unmount()
  })

  it('drops it once the confirmation is accepted', async () => {
    const w = mountGantt()
    await nextTick()
    await nextTick()

    await w.find('.g-del').trigger('click')
    await nextTick()
    unlinkPrompt(w).vm.$emit('positive-click')
    await nextTick()
    await nextTick()

    expect(removeRelation).toHaveBeenCalledWith('r1')
    // Accepting closes the prompt; the edge list is re-read for the new arrows.
    expect(unlinkPrompt(w).props('show')).toBe(false)
    expect(dependencies.mock.calls.length).toBeGreaterThan(1)
    w.unmount()
  })

  it('keeps the relation when the prompt is dismissed', async () => {
    const w = mountGantt()
    await nextTick()
    await nextTick()

    await w.find('.g-del').trigger('click')
    await nextTick()
    // clickoutside / Esc both come back as update:show=false.
    unlinkPrompt(w).vm.$emit('update:show', false)
    await nextTick()

    expect(unlinkPrompt(w).props('show')).toBe(false)
    expect(removeRelation).not.toHaveBeenCalled()
    w.unmount()
  })

  it('offers no ✕ at all on a read-only chart', async () => {
    // The archive view reuses the chart read-only: nothing there may write.
    const w = mountGantt({ readonly: true })
    await nextTick()
    await nextTick()

    expect(w.find('.g-arrow').exists()).toBe(true)
    expect(w.find('.g-del').exists()).toBe(false)
    w.unmount()
  })
})
