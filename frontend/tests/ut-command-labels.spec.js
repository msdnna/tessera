import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { commandItems, matchCommands } from '@/utils/commands'
import { i18n, setI18nLocale } from '@/i18n'
import ru from '@/locales/ru'
import en from '@/locales/en'

// The registry rows as the API hands them over: Russian text, because that is
// what backend/internal/quickact/registry.go hard-codes.
const SERVER_ROWS = [
  {
    key: 'due',
    arg: 'date',
    description: 'Установить срок',
    example: '/due 2026-08-14',
  },
  {
    key: 'priority',
    arg: 'priority',
    description: 'Установить приоритет',
    example: '/priority высокий',
  },
]

describe('built-in command labels follow the interface language', () => {
  beforeEach(() => {
    i18n.global.locale.value = 'ru'
  })
  afterEach(() => {
    i18n.global.locale.value = 'ru'
  })

  it('renders the Russian wording on a Russian UI', () => {
    const [due] = commandItems(SERVER_ROWS, [])
    expect(due.description).toBe('Установить срок')
    expect(due.example).toBe('/due 2026-08-14')
  })

  it('replaces the registry wording with English on an English UI', async () => {
    await setI18nLocale('en')
    const [due, priority] = commandItems(SERVER_ROWS, [])
    // The whole point: the popup row under an English UI must not be Russian.
    expect(due.description).not.toMatch(/[А-Яа-я]/)
    expect(priority.description).not.toMatch(/[А-Яа-я]/)
    // The example is an argument the parser really accepts — args.go maps both
    // "высокий" and "high" onto priority 3, so the English hint is executable.
    expect(priority.example).toBe('/priority high')
    expect(due.example).not.toMatch(/[А-Яа-я]/)
  })

  it('keeps a workspace-defined command verbatim — its text is the user’s own', async () => {
    await setI18nLocale('en')
    const items = commandItems([], [{ key: 'approve', description: 'Одобрить план' }])
    expect(items[0].description).toBe('Одобрить план')
  })

  it('falls back to the server text for a command the bundle does not know', async () => {
    await setI18nLocale('en')
    const [row] = commandItems([{ key: 'teleport', description: 'Телепортировать' }], [])
    expect(row.description).toBe('Телепортировать')
  })

  it('searches the popup in the language it is displayed in', async () => {
    expect(matchCommands(commandItems(SERVER_ROWS, []), 'срок').map((c) => c.key)).toEqual(['due'])
    await setI18nLocale('en')
    expect(matchCommands(commandItems(SERVER_ROWS, []), 'due date').map((c) => c.key)).toEqual([
      'due',
    ])
  })
})

// The bundles are the display side of a list that lives on the backend, so a
// command added there has to arrive here too — otherwise it silently renders
// Russian through the fallback and only a reader notices.
describe('the bundles cover the backend registry', () => {
  const source = readFileSync(
    resolve(__dirname, '../../backend/internal/quickact/registry.go'),
    'utf8',
  )
  const registryBody = source.slice(
    source.indexOf('var Registry = []Command{'),
    source.indexOf('\n// byKey maps'),
  )
  const keys = [...registryBody.matchAll(/\{Key:\s*"([a-z_]+)"/g)].map((m) => m[1])

  it('reads a plausible registry off the backend source', () => {
    expect(keys.length).toBeGreaterThan(20)
    expect(keys).toContain('assign')
    expect(keys).toContain('unparent')
  })

  it('has a description and an example for every command, in both locales', () => {
    for (const [name, bundle] of [
      ['ru', ru],
      ['en', en],
    ]) {
      for (const key of keys) {
        const entry = bundle.commands?.builtin?.[key]
        expect(entry, `${name}:${key}`).toBeTruthy()
        expect(typeof entry.description, `${name}:${key}.description`).toBe('string')
        expect(typeof entry.example, `${name}:${key}.example`).toBe('string')
        expect(entry.example.startsWith(`/${key}`), `${name}:${key}.example`).toBe(true)
      }
    }
  })

  it('carries no Cyrillic in the English bundle', () => {
    for (const key of keys) {
      const entry = en.commands.builtin[key]
      expect(entry.description, `en:${key}`).not.toMatch(/[А-Яа-я]/)
      expect(entry.example, `en:${key}`).not.toMatch(/[А-Яа-я]/)
    }
  })
})
