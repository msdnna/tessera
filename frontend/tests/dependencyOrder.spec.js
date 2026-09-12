import { describe, it, expect } from 'vitest'
import { topoByDeps } from '@/utils/dependencyOrder'

const ids = (arr) => arr.map((t) => t.id)
const tasks = (...names) => names.map((id) => ({ id }))

describe('topoByDeps', () => {
  it('places a dependent right after its blocker, before the next root', () => {
    // A blocks Б; В sat right after A in the incoming order → expect A, Б, В.
    const out = topoByDeps(tasks('A', 'Б', 'В'), [{ blocker: 'A', blocked: 'Б' }])
    expect(ids(out)).toEqual(['A', 'Б', 'В'])
  })

  it('returns the list unchanged when there are no deps', () => {
    const list = tasks('A', 'Б', 'В')
    expect(topoByDeps(list, [])).toBe(list)
  })

  it('walks a chain depth-first', () => {
    const out = topoByDeps(tasks('A', 'B', 'C'), [
      { blocker: 'A', blocked: 'B' },
      { blocker: 'B', blocked: 'C' },
    ])
    expect(ids(out)).toEqual(['A', 'B', 'C'])
  })

  it('orders siblings of a node by incoming order', () => {
    // A blocks both C and B; incoming order has B before C → A, B, C.
    const out = topoByDeps(tasks('A', 'B', 'C'), [
      { blocker: 'A', blocked: 'C' },
      { blocker: 'A', blocked: 'B' },
    ])
    expect(ids(out)).toEqual(['A', 'B', 'C'])
  })

  it('keeps every task exactly once even with a cycle', () => {
    const out = topoByDeps(tasks('A', 'B', 'C'), [
      { blocker: 'A', blocked: 'B' },
      { blocker: 'B', blocked: 'A' }, // 2-cycle
    ])
    expect(ids(out).sort()).toEqual(['A', 'B', 'C'])
    expect(out).toHaveLength(3)
  })

  it('ignores edges pointing at absent tasks', () => {
    const out = topoByDeps(tasks('A', 'B'), [{ blocker: 'A', blocked: 'ghost' }])
    expect(ids(out)).toEqual(['A', 'B'])
  })

  it('emits a shared dependent only once, after ALL of its blockers (diamond)', () => {
    // A→B, A→C, B→D, C→D. D must appear once — and below C, not between B and C:
    // the Gantt draws finish→start arrows between rows, so a dependent placed
    // above one of its blockers gets an arrow pointing back up the chart.
    const out = topoByDeps(tasks('A', 'B', 'C', 'D'), [
      { blocker: 'A', blocked: 'B' },
      { blocker: 'A', blocked: 'C' },
      { blocker: 'B', blocked: 'D' },
      { blocker: 'C', blocked: 'D' },
    ])
    expect(ids(out)).toEqual(['A', 'B', 'C', 'D'])
  })

  it('holds a task with two blockers until the second one is emitted', () => {
    // The minimal shape of the same rule: A and B both block C.
    const out = topoByDeps(tasks('A', 'B', 'C'), [
      { blocker: 'A', blocked: 'C' },
      { blocker: 'B', blocked: 'C' },
    ])
    expect(ids(out)).toEqual(['A', 'B', 'C'])
  })

  it('never places a task above one of its blockers', () => {
    // Two roots feeding a joined tail, with the incoming order deliberately
    // interleaved so a first-blocker-wins walk would emit Г before В.
    const out = topoByDeps(tasks('A', 'Б', 'В', 'Г'), [
      { blocker: 'A', blocked: 'Г' },
      { blocker: 'В', blocked: 'Г' },
      { blocker: 'Б', blocked: 'В' },
    ])
    const at = (id) => ids(out).indexOf(id)
    expect(at('Г')).toBeGreaterThan(at('A'))
    expect(at('Г')).toBeGreaterThan(at('В'))
    expect(at('В')).toBeGreaterThan(at('Б'))
  })
})
