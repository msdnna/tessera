import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  scanRussianFiles,
  isAllowlisted,
  stripComments,
  FRONTEND_ROOT,
} from './helpers/ruLiterals.js'

// Ratchet for the string extraction (#2799). The baseline lists every source
// file that still holds Russian interface text; waves shorten it, nothing may
// lengthen it. Two directions matter equally:
//
//  * a file gaining Russian text that is not on the list means new hardcoded
//    UI slipped in behind the waves;
//  * a file on the list with no Russian left means a wave finished it but did
//    not tick it off — and until it is ticked off, eslint's bare-string rule
//    stays switched off for that file (eslint.config.js reads this same list).
const BASELINE_PATH = 'tests/i18n-baseline.json'
const baseline = JSON.parse(readFileSync(join(FRONTEND_ROOT, BASELINE_PATH), 'utf8'))

describe('Russian literals in src/', () => {
  const current = scanRussianFiles()

  it('has no file with Russian text outside the baseline', () => {
    const added = current.filter((f) => !baseline.includes(f))
    expect(
      added,
      `New hardcoded Russian text. Use $t()/i18n.global.t() instead, or — if this really is ` +
        `data and not interface — add the path to ALLOWLIST in tests/helpers/ruLiterals.js.`,
    ).toEqual([])
  })

  it('has no stale baseline entries', () => {
    const done = baseline.filter((f) => !current.includes(f))
    expect(
      done,
      `These files no longer contain Russian text — drop them from ${BASELINE_PATH} so the ` +
        `eslint bare-string rule starts guarding them.`,
    ).toEqual([])
  })

  it('keeps the baseline sorted, unique and free of allowlisted paths', () => {
    expect(baseline).toEqual([...new Set(baseline)])
    expect(baseline).toEqual([...baseline].sort())
    expect(baseline.filter(isAllowlisted)).toEqual([])
  })
})

describe('the scanner itself', () => {
  it('ignores Cyrillic inside comments', () => {
    expect(stripComments('<!-- Назад -->\n<p>x</p>')).not.toMatch(/Назад/)
    expect(stripComments('/* Назад */\nconst a = 1')).not.toMatch(/Назад/)
    expect(stripComments('// with a «Назад»\nconst a = 1')).not.toMatch(/Назад/)
  })

  it('keeps Cyrillic in code, including next to a URL', () => {
    expect(stripComments(`const t = 'Сохранить'`)).toMatch(/Сохранить/)
    expect(stripComments(`const u = 'https://x/y' // Сохранить`)).toMatch(/Сохранить/)
  })
})
