import { describe, it, expect } from 'vitest'
import { helpAssetUrl, resolveHelpImages } from '@/utils/helpAssets'

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

  it('оставляет ссылку как есть, если файла нет', () => {
    // Better a visibly broken image than a silently blank one — the missing file
    // is what cx-help-index.spec.js fails on.
    const html = '<img src="../assets/такого-нет.png">'
    expect(resolveHelpImages(html)).toBe(html)
  })
})
