import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mentions outside a task (#3048): a document's comment and a meeting's chat
// line hand the server the members they @-mention, resolved against the
// workspace roster the way the task composer resolves them.
const wsApi = vi.hoisted(() => ({ members: vi.fn() }))
vi.mock('@/api', () => ({ workspaces: wsApi }))

const { mentionIdsFor, addedMentionIds, resetMentionRoster } = await import('@/utils/mentionRoster')

const ROSTER = [
  { user_id: 'u-anya', name: 'Аня', email: 'anya@example.com', role: 'member' },
  { user_id: 'u-ilya', name: 'Илья', email: 'ilya@example.com', role: 'member' },
]

beforeEach(() => {
  resetMentionRoster()
  wsApi.members.mockReset()
  wsApi.members.mockResolvedValue({ data: ROSTER })
})

describe('mentionRoster', () => {
  it('resolves @-handles to member ids, once each, in order', async () => {
    expect(await mentionIdsFor('ws1', '@Илья и @Аня, гляньте; @Илья ещё раз')).toEqual([
      'u-ilya',
      'u-anya',
    ])
  })

  it('asks for the roster only when there is an «@», and once per workspace', async () => {
    expect(await mentionIdsFor('ws1', 'без упоминаний')).toEqual([])
    expect(wsApi.members).not.toHaveBeenCalled()
    await mentionIdsFor('ws1', '@Аня')
    await mentionIdsFor('ws1', '@Илья')
    expect(wsApi.members).toHaveBeenCalledTimes(1)
  })

  it('an edit hands over only the people it added', async () => {
    expect(await addedMentionIds('ws1', '@Аня глянь', '@Аня и @Илья гляньте')).toEqual(['u-ilya'])
  })

  it('a roster that cannot be read leaves the comment without mentions', async () => {
    wsApi.members.mockRejectedValue(new Error('offline'))
    expect(await mentionIdsFor('ws2', '@Аня')).toEqual([])
  })
})
