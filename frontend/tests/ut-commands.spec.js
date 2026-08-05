import { describe, it, expect } from 'vitest'
import {
  canonCommandKey,
  isValidCommandKey,
  detectSlashQuery,
  commandItems,
  matchCommands,
  commandInsertText,
  hasCommandLine,
} from '@/utils/commands'

describe('canonCommandKey', () => {
  it('strips leading slashes, trims and lowercases', () => {
    expect(canonCommandKey('/Approve ')).toBe('approve')
    expect(canonCommandKey('  //hold')).toBe('hold')
    expect(canonCommandKey('REVISE')).toBe('revise')
  })

  it('returns empty for nothing usable', () => {
    expect(canonCommandKey('')).toBe('')
    expect(canonCommandKey('   ')).toBe('')
    expect(canonCommandKey(null)).toBe('')
    expect(canonCommandKey('/')).toBe('')
  })
})

describe('isValidCommandKey', () => {
  it('accepts the storage form the backend accepts', () => {
    expect(isValidCommandKey('approve')).toBe(true)
    expect(isValidCommandKey('/Approve')).toBe(true)
    expect(isValidCommandKey('board_move')).toBe(true)
    expect(isValidCommandKey('a1-b2')).toBe(true)
  })

  it('rejects spaces, leading punctuation, cyrillic and overlong keys', () => {
    expect(isValidCommandKey('a b')).toBe(false)
    expect(isValidCommandKey('-lead')).toBe(false)
    expect(isValidCommandKey('одобрить')).toBe(false)
    expect(isValidCommandKey('x'.repeat(33))).toBe(false)
    expect(isValidCommandKey('')).toBe(false)
  })
})

describe('detectSlashQuery', () => {
  it('triggers at the very start of the text', () => {
    expect(detectSlashQuery('/')).toEqual({ start: 0, query: '' })
    expect(detectSlashQuery('/clo')).toEqual({ start: 0, query: 'clo' })
  })

  it('triggers at the start of a later line', () => {
    expect(detectSlashQuery('текст\n/as')).toEqual({ start: 6, query: 'as' })
  })

  // The whole reason the trigger is line-anchored: these are everyday task text.
  it('does not trigger mid-line', () => {
    expect(detectSlashQuery('cd /home')).toBeNull()
    expect(detectSlashQuery('src/utils')).toBeNull()
    expect(detectSlashQuery('24/7')).toBeNull()
    expect(detectSlashQuery('см. /docs/api')).toBeNull()
  })

  it('closes once the query stops looking like a key', () => {
    expect(detectSlashQuery('/assign @msdnna')).toBeNull()
    expect(detectSlashQuery('/close\n')).toBeNull()
    expect(detectSlashQuery('')).toBeNull()
  })
})

describe('commandItems', () => {
  it('flattens the registry response, built-ins first', () => {
    const items = commandItems(
      [{ key: 'due', arg: 'date', description: 'Установить срок', example: '/due 2026-08-14' }],
      [{ key: 'approve', description: 'Одобрить план' }],
    )
    expect(items.map((c) => c.key)).toEqual(['due', 'approve'])
    expect(items[0]).toMatchObject({ builtin: true, arg: 'date' })
    // Custom entries never take an argument — they are text, not actions.
    expect(items[1]).toMatchObject({ builtin: false, arg: 'none' })
  })

  it('tolerates missing halves', () => {
    expect(commandItems()).toEqual([])
    expect(commandItems(null, null)).toEqual([])
  })
})

describe('matchCommands', () => {
  const items = commandItems(
    [
      { key: 'assign', aliases: ['assignee'], arg: 'user', description: 'Назначить исполнителя' },
      { key: 'due', arg: 'date', description: 'Установить срок' },
      { key: 'close', arg: 'none', description: 'Закрыть задачу' },
    ],
    [{ key: 'approve', description: 'Одобрить план' }],
  )

  it('matches key, alias and description', () => {
    expect(matchCommands(items, 'ass').map((c) => c.key)).toEqual(['assign'])
    expect(matchCommands(items, 'assignee').map((c) => c.key)).toEqual(['assign'])
    expect(matchCommands(items, 'срок').map((c) => c.key)).toEqual(['due'])
    expect(matchCommands(items, 'appr').map((c) => c.key)).toEqual(['approve'])
  })

  it('lists everything for an empty query and honours the limit', () => {
    expect(matchCommands(items, '')).toHaveLength(4)
    expect(matchCommands(items, '', 2).map((c) => c.key)).toEqual(['assign', 'due'])
  })

  it('returns nothing for a miss, and survives an empty registry', () => {
    expect(matchCommands(items, 'zzz')).toEqual([])
    expect(matchCommands([], 'due')).toEqual([])
    expect(matchCommands(null, 'due')).toEqual([])
  })
})

describe('commandInsertText', () => {
  const [assign, close, custom] = commandItems(
    [
      { key: 'assign', arg: 'user', description: '' },
      { key: 'close', arg: 'none', description: '' },
    ],
    [{ key: 'approve', description: '' }],
  )

  it('leaves the caret after a space when an argument is expected', () => {
    expect(commandInsertText(assign)).toBe('/assign ')
  })

  it('ends the line for argument-less and custom commands', () => {
    expect(commandInsertText(close)).toBe('/close\n')
    expect(commandInsertText(custom)).toBe('/approve\n')
  })

  it('is safe on nothing', () => {
    expect(commandInsertText(null)).toBe('')
  })
})

describe('hasCommandLine', () => {
  it('detects a slash-led line anywhere in the body', () => {
    expect(hasCommandLine('/close')).toBe(true)
    expect(hasCommandLine('текст\n  /assign @msdnna')).toBe(true)
  })

  it('ignores paths and plain text', () => {
    expect(hasCommandLine('cd /home')).toBe(false)
    expect(hasCommandLine('просто текст')).toBe(false)
    expect(hasCommandLine('')).toBe(false)
  })
})
