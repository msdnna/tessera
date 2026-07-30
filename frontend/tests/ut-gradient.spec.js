import { describe, it, expect } from 'vitest'
import {
  hueGrad,
  hueGradVert,
  softFill,
  tagPillBg,
  readableHue,
  onColor,
  swatchBg,
} from '@/utils/gradient'

describe('hueGrad / hueGradVert', () => {
  it('builds a diagonal same-hue gradient with a centre base stop', () => {
    const g = hueGrad('#7c5cff')
    expect(g).toContain('linear-gradient(to top right')
    expect(g).toContain('#7c5cff 50%')
    expect(g).toContain('color-mix(in srgb, #7c5cff 86%, #000)')
    expect(g).toContain('color-mix(in srgb, #7c5cff 86%, #fff)')
  })

  it('vertical variant runs bottom→top', () => {
    expect(hueGradVert('#f00')).toContain('linear-gradient(to top,')
  })

  it('falls back to a neutral grey for a missing colour', () => {
    expect(hueGrad()).toContain('#888')
    expect(hueGradVert(null)).toContain('#888')
  })
})

describe('softFill / tagPillBg', () => {
  it('softFill tints toward the surface var', () => {
    expect(softFill('#7c5cff')).toBe('color-mix(in srgb, #7c5cff 13%, var(--t-surface))')
  })

  it('tagPillBg layers a flat interior on padding-box + gradient on border-box', () => {
    const filled = tagPillBg('#7c5cff')
    expect(filled).toContain('padding-box')
    expect(filled).toContain('border-box')
    expect(filled).toContain(softFill('#7c5cff'))
    const bare = tagPillBg('#7c5cff', false)
    expect(bare).toContain('linear-gradient(transparent, transparent) padding-box')
  })
})

describe('readableHue', () => {
  it('returns the input (or grey) for invalid hex', () => {
    expect(readableHue('nothex', true)).toBe('nothex')
    expect(readableHue('', false)).toBe('#888')
    expect(readableHue(null, false)).toBe('#888')
  })

  it('expands 3-digit hex and clamps lightness per theme', () => {
    // near-black on dark theme → lightened to a legible band
    const onDark = readableHue('#000', true)
    expect(onDark).toMatch(/^#[0-9a-f]{6}$/)
    expect(onDark).not.toBe('#000000')
    // near-white on light theme → darkened
    const onLight = readableHue('#fff', false)
    expect(onLight).not.toBe('#ffffff')
    // 3-digit form is accepted
    expect(readableHue('#f00', true)).toMatch(/^#[0-9a-f]{6}$/)
  })
})

describe('onColor', () => {
  it('picks dark text on a bright fill and white on a dark fill', () => {
    expect(onColor('#ffff00')).toBe('#1f1f1f') // yellow → dark text
    expect(onColor('#000000')).toBe('#ffffff')
  })

  it('defaults to white for invalid input', () => {
    expect(onColor('zzz')).toBe('#fff') // non-hex chars → null → white
    expect(onColor('')).toBe('#fff')
  })
})

describe('swatchBg', () => {
  it('gradient for a real colour, flat neutral for default', () => {
    expect(swatchBg('#7c5cff')).toBe(hueGrad('#7c5cff'))
    expect(swatchBg('')).toBe('linear-gradient(var(--t-border), var(--t-border))')
  })
})
