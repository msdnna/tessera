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

  it('emits a shared dependent only once (diamond)', () => {
    // A→B, A→C, B→D, C→D. D must appear once, after its first visited blocker.
    const out = topoByDeps(tasks('A', 'B', 'C', 'D'), [
      { blocker: 'A', blocked: 'B' },
      { blocker: 'A', blocked: 'C' },
      { blocker: 'B', blocked: 'D' },
      { blocker: 'C', blocked: 'D' },
    ])
    expect(ids(out)).toEqual(['A', 'B', 'D', 'C'])
  })
})
