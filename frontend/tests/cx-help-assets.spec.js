import { describe, it, expect } from 'vitest'
import { readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { helpAssetUrl, helpAssetCandidates, resolveHelpImages } from '@/utils/helpAssets'

// The Russian set — the names articles actually link. The English twins are
// resolved *to*, never linked, so they are filtered out here.
const ASSETS = join(dirname(fileURLToPath(import.meta.url)), '../../docs/help/assets')
const BASE_NAMES = readdirSync(ASSETS).filter((n) => !n.includes('.en.'))

// The help articles are `?raw` strings, so nothing in the build pipeline checks
// their image links (#2793). This guards the one piece that makes them work:
// the rewrite from "../assets/board-light.png" to the URL of the built asset,
// and the light → dark swap a reader in the dark theme gets.

describe('helpAssetUrl', () => {
  it('находит скриншот по имени файла независимо от глубины статьи', () => {
    const fromBoards = helpAssetUrl('../assets/board-light.png')
    expect(fromBoards).toBeTruthy()
    expect(helpAssetUrl('../../assets/board-light.png')).toBe(fromBoards)
  })

  it('в тёмной теме отдаёт тёмный дубль', () => {
    const light = helpAssetUrl('../assets/board-light.png')
    const dark = helpAssetUrl('../assets/board-light.png', true)
    expect(dark).toBeTruthy()
    expect(dark).not.toBe(light)
    expect(dark).toBe(helpAssetUrl('../assets/board-dark.png'))
  })

  it('без тёмного дубля остаётся исходный файл', () => {
    // A name that has no "-light" suffix has no twin to swap to; asking for the
    // dark variant must not blank the picture.
    expect(helpAssetUrl('../assets/board-dark.png', true)).toBe(
      helpAssetUrl('../assets/board-dark.png'),
    )
  })

  it('неизвестный файл даёт пустую строку', () => {
    expect(helpAssetUrl('../assets/такого-нет.png')).toBe('')
  })

  it('снятый английский кадр действительно подставляется', () => {
    // Both waves landed (#2816) — desktop and mobile — so for these names the
    // resolver must pick the twin, not merely be capable of it.
    expect(helpAssetUrl('../assets/board-light.png', false, 'en')).toBe(
      helpAssetUrl('../assets/board-light.en.png'),
    )
    expect(helpAssetUrl('../assets/board-light.png', true, 'en')).toBe(
      helpAssetUrl('../assets/board-dark.en.png'),
    )
    expect(helpAssetUrl('../assets/board-mobile-light.png', false, 'en')).toBe(
      helpAssetUrl('../assets/board-mobile-light.en.png'),
    )
    expect(helpAssetUrl('../assets/board-mobile-light.png', true, 'en')).toBe(
      helpAssetUrl('../assets/board-mobile-dark.en.png'),
    )
  })

  it('без английского кадра берётся русский того же тона', () => {
    // Checked on a language nothing has been shot in rather than on a name whose
    // twin merely hasn't been taken yet: the previous version stood on
    // `board-mobile-*`, and the moment that wave landed it began asserting the
    // opposite of its own title. Only ru and en are shot, so any third language
    // exercises the fallback for good.
    expect(helpAssetUrl('../assets/board-light.png', false, 'de')).toBe(
      helpAssetUrl('../assets/board-light.png'),
    )
    expect(helpAssetUrl('../assets/board-light.png', true, 'de')).toBe(
      helpAssetUrl('../assets/board-dark.png'),
    )
  })

  it('английскому читателю ни одно имя не даёт пустую картинку', () => {
    // Holds regardless of which wave has landed: every name an article can link
    // resolves to *something* in English. Guards the case a per-name assertion
    // cannot — a twin added under a typo'd name leaves the original unreachable.
    expect(BASE_NAMES.length).toBeGreaterThan(0)
    for (const name of BASE_NAMES) {
      expect(helpAssetUrl(`../assets/${name}`, false, 'en')).toBeTruthy()
      expect(helpAssetUrl(`../assets/${name}`, true, 'en')).toBeTruthy()
    }
  })
})

describe('helpAssetCandidates', () => {
  // The order is the contract, and it has to be checkable before the `.en.png`
  // files exist — the shots are added in a later wave of the same task.
  it('в русском ищет ровно то, что искал раньше', () => {
    expect(helpAssetCandidates('../assets/board-light.png')).toEqual(['board-light.png'])
    expect(helpAssetCandidates('../assets/board-light.png', true)).toEqual([
      'board-dark.png',
      'board-light.png',
    ])
  })

  it('английский кадр берётся первым, но тема важнее языка', () => {
    expect(helpAssetCandidates('../assets/board-light.png', false, 'en')).toEqual([
      'board-light.en.png',
      'board-light.png',
    ])
    // A white shot on a dark page hurts to look at; a Russian shot in an English
    // article merely reads as untranslated — so the dark Russian twin outranks
    // the light English one.
    expect(helpAssetCandidates('../assets/board-light.png', true, 'en')).toEqual([
      'board-dark.en.png',
      'board-dark.png',
      'board-light.en.png',
      'board-light.png',
    ])
  })
})

describe('resolveHelpImages', () => {
  it('подменяет относительный src на собранный URL', () => {
    const html = '<p><img src="../assets/board-light.png" alt="Доска"></p>'
    const out = resolveHelpImages(html)
    expect(out).not.toContain('../assets/board-light.png')
    expect(out).toContain(helpAssetUrl('../assets/board-light.png'))
    expect(out).toContain('alt="Доска"')
  })

  it('в тёмной теме подставляет тёмный кадр', () => {
    const html = '<img src="../assets/board-light.png">'
    expect(resolveHelpImages(html, true)).toContain(helpAssetUrl('../assets/board-dark.png'))
  })

  it('не трогает внешние и data-картинки', () => {
    const external = '<img src="https://example.com/x.png">'
    const data = '<img src="data:image/png;base64,AAAA">'
    expect(resolveHelpImages(external)).toBe(external)
    expect(resolveHelpImages(data)).toBe(data)
  })

  it('язык статьи не ломает русский рендер', () => {
    // Regression guard: an untranslated article renders with lang='ru' and must
    // resolve byte-for-byte as it did before the language axis existed.
    const html = '<img src="../assets/board-light.png">'
    expect(resolveHelpImages(html, false, 'ru')).toBe(resolveHelpImages(html))
    expect(resolveHelpImages(html, true, 'ru')).toBe(resolveHelpImages(html, true))
  })

  it('оставляет ссылку как есть, если файла нет', () => {
    // Better a visibly broken image than a silently blank one — the missing file
    // is what cx-help-index.spec.js fails on.
    const html = '<img src="../assets/такого-нет.png">'
    expect(resolveHelpImages(html)).toBe(html)
  })
})
