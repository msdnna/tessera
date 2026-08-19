import { describe, expect, it } from 'vitest'
import { contrastRatio, darkSheetInk, parseCssColor, readableInk } from '@/utils/docColor'
import { DARK } from '@/styles/tokens'

describe('parseCssColor', () => {
  it('reads the notations an imported document and the picker produce', () => {
    expect(parseCssColor('#1f4e79')).toEqual([31, 78, 121])
    expect(parseCssColor('#FFF')).toEqual([255, 255, 255])
    expect(parseCssColor('rgb(31, 78, 121)')).toEqual([31, 78, 121])
    expect(parseCssColor('rgba(31, 78, 121, 0.5)')).toEqual([31, 78, 121])
    expect(parseCssColor('black')).toEqual([0, 0, 0])
  })

  it('refuses to guess at anything else', () => {
    // A colour we cannot read is not a colour we should rewrite.
    expect(parseCssColor('var(--t-text1)')).toBe(null)
    expect(parseCssColor('color-mix(in srgb, red, blue)')).toBe(null)
    expect(parseCssColor('')).toBe(null)
    expect(parseCssColor(null)).toBe(null)
  })
})

describe('readableInk', () => {
  it('lifts a Word colour until it reads on the dark sheet', () => {
    // #1f4e79 is the heading colour of the document from задача 2755: 1.9:1 on
    // the dark sheet, which is text you cannot read.
    const lifted = readableInk('#1f4e79', DARK.surface)
    expect(lifted).not.toBe('#1f4e79')
    expect(
      contrastRatio(parseCssColor(lifted), parseCssColor(DARK.surface)),
    ).toBeGreaterThanOrEqual(4.5)
  })

  it('keeps the hue, so a blue heading stays blue', () => {
    const [r, g, b] = parseCssColor(readableInk('#1f4e79', DARK.surface))
    expect(b).toBeGreaterThan(g)
    expect(g).toBeGreaterThan(r)
  })

  it('leaves a colour that already reads exactly as it was', () => {
    // The common case — a palette chosen for this theme — must cost nothing and
    // change nothing.
    expect(readableInk('#f0f0f3', DARK.surface)).toBe('#f0f0f3')
    expect(readableInk('#9aa0aa', DARK.surface)).toBe('#9aa0aa')
  })

  it('nudges a colour that only just misses, rather than jumping', () => {
    // The urgent-priority red sits at 4.4:1 on the dark sheet. Lifting it to
    // the 4.5 bar has to stay a nudge — a colour that lands far from the one
    // the author picked is the failure this whole path is trying to avoid.
    const ink = parseCssColor(readableInk('#e0533d', DARK.surface))
    const before = parseCssColor('#e0533d')
    expect(ink).not.toEqual(before)
    for (let i = 0; i < 3; i += 1) expect(Math.abs(ink[i] - before[i])).toBeLessThan(20)
  })

  it('darkens instead of lifting when the surface is light', () => {
    const ink = readableInk('#ffff00', '#ffffff')
    const [r, g, b] = parseCssColor(ink)
    expect(r).toBeLessThan(255)
    expect(b).toBeLessThan(r)
    expect(g).toBeGreaterThan(b)
    expect(contrastRatio([r, g, b], [255, 255, 255])).toBeGreaterThanOrEqual(4.5)
  })

  it('stops short of white, so the hue survives even from near-black', () => {
    const ink = readableInk('#000000', DARK.surface)
    expect(ink).not.toBe('#ffffff')
    expect(contrastRatio(parseCssColor(ink), parseCssColor(DARK.surface))).toBeGreaterThanOrEqual(
      4.5,
    )
  })

  it('passes an unreadable value through untouched', () => {
    expect(readableInk('var(--t-text1)', DARK.surface)).toBe('var(--t-text1)')
    expect(readableInk('#1f4e79', 'var(--t-surface)')).toBe('#1f4e79')
  })
})

describe('darkSheetInk', () => {
  it('measures against the dark sheet the document is drawn on', () => {
    expect(darkSheetInk('#1f4e79')).toBe(readableInk('#1f4e79', DARK.surface))
  })
})
