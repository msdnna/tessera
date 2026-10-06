import { describe, it, expect } from 'vitest'

import { viewLayout, conflictingViewLayout } from '@/utils/boardViews'

// A saved view is presented as belonging to one visualization: the load popover and
// the "overwrite:" chip row are both filtered by layout. The server key is not —
// (board_id, user_id, name) — so a name that looks free in the timeline list is in
// fact taken by the board view of the same name, and saving overwrites it.
describe('viewLayout', () => {
  it('defaults a legacy view with no stored layout to the board', () => {
    expect(viewLayout({ name: 'Спринт', config: {} })).toBe('board')
    expect(viewLayout({ name: 'Спринт' })).toBe('board')
    expect(viewLayout({ name: 'Спринт', config: { layout: 'gantt' } })).toBe('gantt')
  })
})

describe('conflictingViewLayout', () => {
  const views = [
    { id: '1', name: 'Спринт', config: { layout: 'board' } },
    { id: '2', name: 'Релиз', config: { layout: 'timeline' } },
    { id: '3', name: 'Старый', config: {} },
  ]

  it('reports the other visualization that already owns the name', () => {
    // Saving "Спринт" from the timeline silently destroys the board view of that
    // name — it is not among the overwrite chips the popover offers.
    expect(conflictingViewLayout(views, 'Спринт', 'timeline')).toBe('board')
    expect(conflictingViewLayout(views, 'Релиз', 'board')).toBe('timeline')
    // Legacy rows count as board views, so they can be shadowed the same way.
    expect(conflictingViewLayout(views, 'Старый', 'gantt')).toBe('board')
  })

  it('stays silent on a plain same-layout overwrite', () => {
    expect(conflictingViewLayout(views, 'Спринт', 'board')).toBe('')
    expect(conflictingViewLayout(views, 'Релиз', 'timeline')).toBe('')
  })

  it('stays silent on a free name', () => {
    expect(conflictingViewLayout(views, 'Новое', 'board')).toBe('')
    expect(conflictingViewLayout([], 'Спринт', 'timeline')).toBe('')
    expect(conflictingViewLayout(null, 'Спринт', 'timeline')).toBe('')
  })

  it('compares the trimmed name the save path actually sends', () => {
    expect(conflictingViewLayout(views, '  Спринт  ', 'timeline')).toBe('board')
  })
})
