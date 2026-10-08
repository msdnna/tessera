// The matrix's keyboard selection (#2973): arrows walk a quadrant's grid of
// cards and cross into the neighbouring quadrant at its edge.
import { describe, it, expect } from 'vitest'
import { matrixMove, neighbourQuad, firstInMatrix } from '@/utils/matrixNav'

// q0 has five cards (two columns: a b / c d / e), q1 two, q2 none, q3 one.
const Q = [['a', 'b', 'c', 'd', 'e'], ['f', 'g'], [], ['h']]

describe('neighbourQuad', () => {
  it('knows the 2×2 grid', () => {
    expect(neighbourQuad(0, 'right')).toBe(1)
    expect(neighbourQuad(0, 'down')).toBe(2)
    expect(neighbourQuad(3, 'left')).toBe(2)
    expect(neighbourQuad(3, 'up')).toBe(1)
    expect(neighbourQuad(0, 'left')).toBeNull()
    expect(neighbourQuad(1, 'up')).toBeNull()
  })
})

describe('matrixMove', () => {
  it('starts on the first card of the matrix', () => {
    expect(matrixMove(Q, 2, null, 'down')).toEqual({ q: 0, id: 'a' })
    expect(firstInMatrix([[], [], ['x'], []])).toEqual({ q: 2, id: 'x' })
  })

  it('walks the grid inside a quadrant', () => {
    expect(matrixMove(Q, 2, { q: 0, id: 'a' }, 'right')).toEqual({ q: 0, id: 'b' })
    expect(matrixMove(Q, 2, { q: 0, id: 'a' }, 'down')).toEqual({ q: 0, id: 'c' })
    expect(matrixMove(Q, 2, { q: 0, id: 'd' }, 'down')).toEqual({ q: 0, id: 'e' })
    expect(matrixMove(Q, 1, { q: 0, id: 'a' }, 'down')).toEqual({ q: 0, id: 'b' })
  })

  it('crosses into the neighbour on the same row, or stays at an empty one', () => {
    expect(matrixMove(Q, 2, { q: 0, id: 'b' }, 'right')).toEqual({ q: 1, id: 'f' })
    expect(matrixMove(Q, 2, { q: 0, id: 'd' }, 'right')).toEqual({ q: 1, id: 'f' })
    expect(matrixMove(Q, 2, { q: 1, id: 'f' }, 'left')).toEqual({ q: 0, id: 'b' })
    expect(matrixMove(Q, 2, { q: 0, id: 'e' }, 'down')).toEqual({ q: 0, id: 'e' })
    expect(matrixMove(Q, 2, { q: 1, id: 'g' }, 'down')).toEqual({ q: 3, id: 'h' })
    expect(matrixMove(Q, 2, { q: 3, id: 'h' }, 'up')).toEqual({ q: 1, id: 'f' })
  })
})
