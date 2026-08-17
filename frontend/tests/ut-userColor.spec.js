import { describe, it, expect } from 'vitest'
import { userColor, USER_COLORS } from '@/utils/userColor'

describe('userColor', () => {
  // The colour is a name, not decoration: "жёлтый блок — это Петя" only works if
  // Петя is yellow on every screen in the room. Anything derived from local
  // state (join order, a random draw) would make the cue point at two different
  // people at once.
  it('gives the same person the same colour every time', () => {
    const id = '7f3d6a10-1c2b-4f8e-9a11-0d5c6b7e8f90'
    expect(userColor(id)).toBe(userColor(id))
  })

  it('always returns a colour from the palette', () => {
    for (const id of ['a', 'b', 'c', '7f3d6a10-1c2b-4f8e-9a11-0d5c6b7e8f90', '']) {
      expect(USER_COLORS).toContain(userColor(id))
    }
  })

  it('spreads a handful of people across different colours', () => {
    const ids = Array.from({ length: 8 }, (_, i) => `user-${i}-9a11-0d5c6b7e8f90`)
    const distinct = new Set(ids.map(userColor))
    // Not "all 8 differ" — that would be asserting the hash, not the property.
    // Two collaborators sharing a colour is a hash collision; half of them
    // sharing one would mean the function is not spreading at all.
    expect(distinct.size).toBeGreaterThan(ids.length / 2)
  })

  it('survives a missing id instead of throwing at render time', () => {
    expect(USER_COLORS).toContain(userColor(undefined))
    expect(USER_COLORS).toContain(userColor(null))
  })
})
