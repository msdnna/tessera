import { describe, it, expect } from 'vitest'
import { taskLink } from '@/utils/taskLink'

describe('taskLink', () => {
  const origin = 'https://tessera.example'

  it('prefers the per-workspace number over the uuid', () => {
    const t = { board_id: 'b1', number: 252, id: 'uuid-1' }
    expect(taskLink(t, origin)).toBe('https://tessera.example/board/b1?task=252')
  })

  it('falls back to the uuid when there is no number', () => {
    const t = { board_id: 'b1', number: null, id: 'uuid-1' }
    expect(taskLink(t, origin)).toBe('https://tessera.example/board/b1?task=uuid-1')
  })

  it('returns null without a board to point at', () => {
    expect(taskLink({ number: 5 }, origin)).toBeNull()
    expect(taskLink(null, origin)).toBeNull()
  })

  it('returns null when neither number nor id is present', () => {
    expect(taskLink({ board_id: 'b1' }, origin)).toBeNull()
  })

  it('uses the given origin verbatim', () => {
    const t = { board_id: 'b9', number: 1 }
    expect(taskLink(t, 'http://localhost:5174')).toBe('http://localhost:5174/board/b9?task=1')
  })
})
