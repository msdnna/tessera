import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mountTesseraLoader } from '@/utils/tesseraLoader'

describe('mountTesseraLoader', () => {
  let host
  let rafSpy

  beforeEach(() => {
    host = document.createElement('div')
    document.body.appendChild(host)
    // Deterministic: run one frame then stop, so animation code is exercised once.
    let called = false
    rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => {
      if (!called) {
        called = true
        cb(16)
      }
      return 1
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    rafSpy.mockRestore()
    vi.restoreAllMocks()
    host.remove()
  })

  it('mounts an svg with 7 rects + a glyph path and default currentColor paint', () => {
    mountTesseraLoader(host)
    const svg = host.querySelector('svg')
    expect(svg).toBeTruthy()
    expect(svg.getAttribute('fill')).toBe('currentColor')
    expect(svg.getAttribute('width')).toBe('140')
    expect(svg.querySelectorAll('rect')).toHaveLength(7)
    expect(svg.querySelector('path')).toBeTruthy()
  })

  it('honours size and paint options', () => {
    mountTesseraLoader(host, { size: 64, paint: 'url(#t-accent-grad-svg)' })
    const svg = host.querySelector('svg')
    expect(svg.getAttribute('width')).toBe('64')
    expect(svg.getAttribute('fill')).toBe('url(#t-accent-grad-svg)')
  })

  it('destroy() removes the svg from the host', () => {
    const handle = mountTesseraLoader(host)
    expect(host.querySelector('svg')).toBeTruthy()
    handle.destroy()
    expect(host.querySelector('svg')).toBeNull()
  })

  it('draws a static frame (no rAF) when reduced motion is preferred', () => {
    // jsdom has no matchMedia — stub one that reports the reduce preference.
    const prev = window.matchMedia
    window.matchMedia = vi.fn().mockReturnValue({ matches: true })
    rafSpy.mockClear()
    const handle = mountTesseraLoader(host)
    expect(host.querySelector('svg')).toBeTruthy()
    expect(rafSpy).not.toHaveBeenCalled()
    handle.destroy()
    expect(host.querySelector('svg')).toBeNull()
    window.matchMedia = prev
  })

  it('renders numeric geometry attributes on the rects', () => {
    mountTesseraLoader(host)
    const rect = host.querySelector('rect')
    expect(Number.isNaN(Number(rect.getAttribute('width')))).toBe(false)
    expect(Number(rect.getAttribute('width'))).toBeGreaterThanOrEqual(0)
  })
})
