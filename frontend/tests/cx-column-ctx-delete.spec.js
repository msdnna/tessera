// Deleting a column is the most destructive thing a board offers: the server
// cascades through tasks.column_id and purges the uploads of every card standing
// in it, and there is no undo. The «…» popover has always asked first; the
// right-click menu on the same header used to fire the API call straight from the
// menu item, so one misclick emptied the column for good. Pinned here: the menu
// only arms a confirm, and the call happens on the confirm's positive click.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { NDropdown, NPopconfirm } from 'naive-ui'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const remove = vi.fn()
vi.mock('@/api', () => ({ columns: { remove: (...a) => remove(...a), update: vi.fn() } }))

const { default: ColumnHeader } = await import('@/components/ColumnHeader.vue')

const DCOL = { key: 'c1', name: 'К работе', rawName: 'К работе' }
const mountHead = () =>
  mount(ColumnHeader, {
    props: { dcol: DCOL, editable: true },
    global: { stubs: { TesseraIcon: true } },
  })

// The confirm armed by the context menu is the coordinate-anchored one (it is
// given :x/:y); the popover's own confirm has neither.
const ctxConfirm = (w) => w.findAllComponents(NPopconfirm).find((p) => p.props('x') !== undefined)

describe('ColumnHeader right-click delete', () => {
  beforeEach(() => remove.mockClear())

  it('asks before deleting instead of calling the API from the menu', async () => {
    const w = mountHead()
    await w.get('.col-head').trigger('contextmenu')
    w.findComponent(NDropdown).vm.$emit('select', 'delete')
    await w.vm.$nextTick()

    expect(remove).not.toHaveBeenCalled()
    expect(ctxConfirm(w).props('show')).toBe(true)
    w.unmount()
  })

  it('deletes once the confirm is accepted', async () => {
    const w = mountHead()
    await w.get('.col-head').trigger('contextmenu')
    w.findComponent(NDropdown).vm.$emit('select', 'delete')
    await w.vm.$nextTick()

    ctxConfirm(w).vm.$emit('positive-click')
    await w.vm.$nextTick()

    expect(remove).toHaveBeenCalledWith('c1')
    expect(w.emitted('changed')).toBeTruthy()
    w.unmount()
  })

  it('leaves the column alone when the confirm is dismissed', async () => {
    const w = mountHead()
    await w.get('.col-head').trigger('contextmenu')
    w.findComponent(NDropdown).vm.$emit('select', 'delete')
    await w.vm.$nextTick()

    ctxConfirm(w).vm.$emit('clickoutside')
    await w.vm.$nextTick()

    expect(remove).not.toHaveBeenCalled()
    expect(ctxConfirm(w).props('show')).toBe(false)
    w.unmount()
  })

  it('does not arm the confirm for the other menu entries', async () => {
    const w = mountHead()
    await w.get('.col-head').trigger('contextmenu')
    w.findComponent(NDropdown).vm.$emit('select', 'done')
    await w.vm.$nextTick()

    expect(ctxConfirm(w).props('show')).toBe(false)
    expect(w.emitted('set-done')).toBeTruthy()
    w.unmount()
  })
})
