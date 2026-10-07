// «Вид доски» (#2986): an accordion of sections, each closed one with a summary;
// the sections follow the view (cards on the kanban and the matrix, the scale on
// the timelines, columns on the kanban); the scale section carries what the
// Gantt's own settings popover used to.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import BoardCustomizePanel from '@/components/BoardCustomizePanel.vue'

const STUBS = { teleport: true, TesseraIcon: true, IconColorPicker: true }

beforeEach(() => {
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

function mountPanel(props = {}, slots = {}) {
  return mount(BoardCustomizePanel, {
    props: {
      show: true,
      boardName: 'Спринт 14',
      fieldVis: { description: false },
      facetChips: [],
      addOptions: [],
      groupLabel: 'Статус',
      ...props,
    },
    slots,
    global: { stubs: STUBS },
    attachTo: document.body,
  })
}
const heads = (w) => w.findAll('.cp-head').map((h) => h.attributes('data-testid'))

describe('BoardCustomizePanel sections', () => {
  it('draws the kanban’s sections, the views first when the board gives them', () => {
    const w = mountPanel({ layout: 'board' }, { views: '<div class="views-probe" />' })
    expect(heads(w)).toEqual([
      'customize-sec-views',
      'customize-sec-cards',
      'customize-sec-facets',
      'customize-sec-columns',
      'customize-sec-board',
    ])
    expect(w.find('.views-probe').exists()).toBe(true)
    w.unmount()
  })

  it('swaps cards and columns for the scale on the timelines', () => {
    const w = mountPanel({ layout: 'gantt' })
    expect(heads(w)).toEqual(['customize-sec-scale', 'customize-sec-facets', 'customize-sec-board'])
    w.unmount()
  })

  it('keeps cards on the matrix and drops the board section in the archive', () => {
    const w = mountPanel({ layout: 'matrix', readonly: true })
    expect(heads(w)).toEqual([
      'customize-sec-cards',
      'customize-sec-matrix',
      'customize-sec-facets',
    ])
    w.unmount()
  })

  it('opens one section at a time, ⇧-click opens several', async () => {
    const w = mountPanel({ layout: 'board', defaultOpen: [] })
    await w.find('[data-testid="customize-sec-cards"]').trigger('click')
    expect(w.find('.cp-cards').exists()).toBe(true)
    await w.find('[data-testid="customize-sec-columns"]').trigger('click')
    expect(w.find('.cp-cards').exists()).toBe(false)
    expect(w.find('.cp-columns').exists()).toBe(true)
    await w.find('[data-testid="customize-sec-cards"]').trigger('click', { shiftKey: true })
    expect(w.find('.cp-cards').exists()).toBe(true)
    expect(w.find('.cp-columns').exists()).toBe(true)
    w.unmount()
  })

  it('summarises a closed section', () => {
    const w = mountPanel({ layout: 'board', defaultOpen: [], cardSize: 'medium' })
    const sum = (k) => w.find(`[data-testid="customize-sec-${k}"] .cp-head-sum`).text()
    expect(sum('cards')).toBe('Средние · 8 полей')
    expect(sum('facets')).toBe('Статус')
    expect(sum('board')).toBe('Спринт 14')
    w.unmount()
  })
})

describe('BoardCustomizePanel cards', () => {
  it('sets the size from the tiles and flips a field with its switch', async () => {
    const w = mountPanel({ layout: 'board', defaultOpen: ['cards'] })
    await w.find('[data-testid="card-size-large"]').trigger('click')
    expect(w.emitted('update:cardSize')).toEqual([['large']])
    const desc = w.find('[data-testid="field-description"]')
    expect(desc.attributes('aria-checked')).toBe('false')
    await desc.trigger('click')
    expect(w.emitted('set-field')).toEqual([['description', true]])
    w.unmount()
  })
})

describe('BoardCustomizePanel scale', () => {
  const ctl = (extra = {}) => ({
    scale: 'weeks',
    setScale: vi.fn(),
    leftCollapsed: false,
    toggleLeft: vi.fn(),
    counters: [
      { key: 'overdue', text: '3 просрочено', overdue: true },
      { key: 'unsched', text: '2 без дат' },
    ],
    hint: 'Тяните',
    ...extra,
  })

  it('carries the scale, the task column, the counters and the hint', async () => {
    const c = ctl()
    const w = mountPanel({ layout: 'timeline', chartCtl: c, defaultOpen: ['scale'] })
    const segs = w.findAll('.cp-seg-btn')
    expect(segs.map((b) => b.classes().includes('on'))).toEqual([false, true, false, false])
    await segs[2].trigger('click')
    expect(c.setScale).toHaveBeenCalledWith('months')
    // «Квартал» is not built yet (#3021): visible, inert, says when.
    expect(segs[3].attributes('aria-disabled')).toBe('true')
    expect(segs[3].attributes('title')).toContain('3021')
    const chips = w.findAll('.cp-counter')
    expect(chips.map((x) => x.text())).toEqual(['3 просрочено', '2 без дат'])
    expect(chips[0].classes()).toContain('overdue')
    expect(w.text()).toContain('Тяните')
    await w.find('.cp-scale button[role="switch"]').trigger('click')
    expect(c.toggleLeft).toHaveBeenCalledOnce()
    w.unmount()
  })
})

describe('BoardCustomizePanel matrix thresholds (#3018)', () => {
  it('summarises and sets the urgency window and the importance floor', async () => {
    const w = mountPanel({
      layout: 'matrix',
      defaultOpen: [],
      matrix: { dueDays: 3, minPriority: 3 },
    })
    expect(w.find('[data-testid="customize-sec-matrix"] .cp-head-sum').text()).toBe(
      '≤ 3 дня · ≥ Высокий',
    )
    await w.find('[data-testid="customize-sec-matrix"]').trigger('click')
    const days = w.findAll('[data-testid="matrix-due"] button')
    expect(days.map((b) => b.attributes('aria-checked'))).toEqual([
      'false',
      'true',
      'false',
      'false',
    ])
    await days[2].trigger('click')
    expect(w.emitted('update:matrix')).toEqual([[{ dueDays: 7, minPriority: 3 }]])
    const prio = w.findAll('[data-testid="matrix-priority"] button')
    await prio[2].trigger('click')
    expect(w.emitted('update:matrix')[1]).toEqual([{ dueDays: 3, minPriority: 4 }])
    w.unmount()
  })
})
