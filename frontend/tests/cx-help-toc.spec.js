import { describe, it, expect } from 'vitest'
import { pickActive, TOC_BAND } from '@/utils/helpToc'

// Geometry of a typical article in the help centre's reading column: 600px of
// viewport, 2000px of content, headings spread through it.
const HEADINGS = [
  { id: 'intro', top: 0 },
  { id: 'board', top: 700 },
  { id: 'card', top: 1400 },
  { id: 'next', top: 1900 },
]
const COLUMN = { clientHeight: 600, scrollHeight: 2000 }

const at = (scrollTop, geom = COLUMN) => pickActive(HEADINGS, { scrollTop, ...geom })

describe('pickActive', () => {
  it('takes the last heading above the top band', () => {
    // Band line sits at scrollTop + 180: «board» (700) is above it, «card» is not.
    expect(at(600)).toBe('board')
    expect(at(1300)).toBe('card')
  })

  it('reports nothing above the first heading', () => {
    expect(pickActive([{ id: 'intro', top: 400 }], { scrollTop: 0, ...COLUMN })).toBe('')
  })

  it('highlights the last visible heading once scrolled to the end', () => {
    // The bug (#2811): at the bottom neither «card» nor «next» can reach the
    // band, and the old observer left «board» highlighted.
    expect(at(1400)).toBe('next')
  })

  it('keeps stepping through sections until the end is actually reached', () => {
    expect(at(1350)).toBe('card')
  })

  it('tolerates a sub-pixel scroll offset at the end', () => {
    expect(at(1399.5)).toBe('next')
  })

  it('picks the last heading of a short article that does not scroll', () => {
    // scrollHeight === clientHeight: the reader is at the end from the start.
    expect(at(0, { clientHeight: 2000, scrollHeight: 2000 })).toBe('next')
  })

  it('ignores headings below the fold of an unscrolled short article', () => {
    expect(
      pickActive([{ id: 'intro', top: 0 }, ...HEADINGS.slice(1)], {
        scrollTop: 0,
        clientHeight: 1000,
        scrollHeight: 1000,
      }),
    ).toBe('board')
  })

  it('returns nothing without headings', () => {
    expect(pickActive([], { scrollTop: 0, ...COLUMN })).toBe('')
    expect(pickActive(null, { scrollTop: 0, ...COLUMN })).toBe('')
  })

  it('honours a custom band', () => {
    // A narrower band puts the line at 660 — «board» (700) has not crossed it yet.
    expect(pickActive(HEADINGS, { scrollTop: 600, ...COLUMN, band: 0.1 })).toBe('intro')
    expect(TOC_BAND).toBe(0.3)
  })
})
