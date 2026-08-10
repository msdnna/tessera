// Card field visibility: the size preset composes the set, the customize-view
// toggles refine it. Both the card and its extracted pill row ask this, so it
// gets a test of its own — a drift between the two reads as a silently missing
// field, not as an error.
import { describe, it, expect } from 'vitest'
import { sizeAllows, fieldEnabled, cardFieldVisible } from '@/utils/cardFields'

describe('sizeAllows', () => {
  it('shows nothing but the title on compact', () => {
    for (const k of ['number', 'priority', 'due', 'tags', 'assignee', 'estimate', 'milestone'])
      expect(sizeAllows('compact', k)).toBe(false)
  })

  it('shows the curated subset on medium', () => {
    for (const k of ['number', 'priority', 'due', 'tags', 'assignee'])
      expect(sizeAllows('medium', k)).toBe(true)
    for (const k of ['estimate', 'milestone', 'description', 'gitlab'])
      expect(sizeAllows('medium', k)).toBe(false)
  })

  it('shows everything on large', () => {
    for (const k of ['number', 'estimate', 'milestone', 'description', 'gitlab'])
      expect(sizeAllows('large', k)).toBe(true)
  })

  // A saved view written by an older build must never blank a card.
  it('treats an unknown size as large', () => {
    expect(sizeAllows('gigantic', 'milestone')).toBe(true)
    expect(sizeAllows(undefined, 'milestone')).toBe(true)
  })
})

describe('fieldEnabled', () => {
  it('defaults a missing key to visible (back-compat with older saved views)', () => {
    expect(fieldEnabled({}, 'due')).toBe(true)
    expect(fieldEnabled(undefined, 'due')).toBe(true)
    expect(fieldEnabled({ due: undefined }, 'due')).toBe(true)
  })

  it('hides only on an explicit false', () => {
    expect(fieldEnabled({ due: false }, 'due')).toBe(false)
    expect(fieldEnabled({ due: true }, 'due')).toBe(true)
    expect(fieldEnabled({ due: 0 }, 'due')).toBe(true)
  })
})

describe('cardFieldVisible', () => {
  it('needs both the preset and the toggle', () => {
    expect(cardFieldVisible('large', {}, 'milestone')).toBe(true)
    // preset allows, toggle off
    expect(cardFieldVisible('large', { milestone: false }, 'milestone')).toBe(false)
    // toggle on, preset drops it
    expect(cardFieldVisible('medium', { milestone: true }, 'milestone')).toBe(false)
    expect(cardFieldVisible('compact', {}, 'priority')).toBe(false)
  })
})
