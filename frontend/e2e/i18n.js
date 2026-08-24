import { readFileSync, readdirSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import { SEED_FILE } from './global-setup.js'

// Interface copy for the language the run was seeded with (#2800).
//
// A spec that asserts on interface text has to ask the catalog for it: hardcoded
// `'На согласовании'` passes on `ru` and fails on `E2E_LANG=en` for the wrong
// reason — the translation is there, the expectation isn't. Asserting through
// the catalog keeps the check honest (the screen must show *this* message) while
// staying language-agnostic. Data the spec seeds itself is a different matter:
// that text is ours and stays literal.
//
// The JSON is read rather than imported: `import … with { type: 'json' }` is the
// only spelling Node accepts, Playwright's loader does not transpile it, and the
// bundle shape here is just the file list of the locale directory.
const here = dirname(fileURLToPath(import.meta.url))
export const LANG = JSON.parse(readFileSync(SEED_FILE, 'utf-8')).language || 'ru'

const dir = resolve(here, '../src/locales', LANG)
const messages = Object.fromEntries(
  readdirSync(dir)
    .filter((f) => f.endsWith('.json'))
    .map((f) => [f.slice(0, -5), JSON.parse(readFileSync(resolve(dir, f), 'utf-8'))]),
)

// Marks a placeholder so tRe can tell it apart from the message's own words. A
// control character cannot occur in a translation, which a word-shaped sentinel
// could.
const MARK = '\u0001'

function lookup(key) {
  const value = key.split('.').reduce((acc, part) => (acc == null ? acc : acc[part]), messages)
  if (typeof value !== 'string') throw new Error(`e2e i18n: no message for "${key}" in ${LANG}`)
  // Plural messages carry every form in one string (`1 файл | 2 файла | 5 файлов`)
  // and the branch depends on the locale's plural rule — a spec that wants one
  // must name the form it expects, not get a random slice of the alternatives.
  if (value.includes('|'))
    throw new Error(`e2e i18n: "${key}" is a plural message — assert on one form explicitly`)
  return value
}

// t resolves a dotted key and fills `{placeholders}`. Deliberately not vue-i18n:
// its runtime wants a Vue app, and the two things a spec needs from it are a
// lookup and an interpolation.
export function t(key, params = {}) {
  return lookup(key).replace(/\{(\w+)\}/g, (m, name) => (name in params ? String(params[name]) : m))
}

// tRe builds a RegExp from a message whose placeholders the caller fills with
// patterns: `tRe('documents.links.meta', { revision: '\\d+' })` matches both
// "Version 2 · …" and "Версия 2 · …" without either spelling entering the spec.
// The message's own text is escaped first, so a translation containing "(", "."
// or "?" cannot quietly turn into a wildcard.
export function tRe(key, patterns = {}) {
  const marked = lookup(key).replace(/\{(\w+)\}/g, (m, name) =>
    name in patterns ? `${MARK}${name}${MARK}` : m,
  )
  const escaped = marked.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(
    escaped.replace(new RegExp(`${MARK}(\\w+)${MARK}`, 'g'), (m, name) => patterns[name]),
  )
}
