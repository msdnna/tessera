import { describe, it, expect } from 'vitest'
import {
  WRAP_PAIRS,
  INDENT,
  wrapSelection,
  indentLines,
  outdentLines,
  orderedListPrefix,
} from '@/utils/mdEdit'

describe('wrapSelection', () => {
  it('wraps the selection in the typed pair instead of replacing it', () => {
    // "world" selected in "hello world"
    const r = wrapSelection('hello world', 6, 11, '(')
    expect(r.value).toBe('hello (world)')
    expect([r.start, r.end]).toEqual([7, 12]) // selection still on the text, not the brackets
  })

  it('closes symmetric marks with the same character', () => {
    expect(wrapSelection('bold', 0, 4, '*').value).toBe('*bold*')
    expect(wrapSelection('code', 0, 4, '`').value).toBe('`code`')
  })

  it('does nothing without a selection or for a character that is not a pair', () => {
    expect(wrapSelection('abc', 2, 2, '(')).toBeNull()
    expect(wrapSelection('abc', 0, 3, 'x')).toBeNull()
  })

  it('covers every pair listed for the keyboard handler', () => {
    for (const [open, close] of Object.entries(WRAP_PAIRS)) {
      expect(wrapSelection('a', 0, 1, open).value).toBe(`${open}a${close}`)
    }
  })
})

describe('indentLines', () => {
  it('indents every line the selection touches', () => {
    const r = indentLines('one\ntwo\nthree', 0, 7) // spans "one" and "two"
    expect(r.value).toBe('  one\n  two\nthree')
  })

  it('shifts the two ends by different amounts (start is on the first line)', () => {
    const r = indentLines('one\ntwo', 1, 5)
    expect(r.start).toBe(1 + INDENT.length)
    expect(r.end).toBe(5 + INDENT.length * 2)
  })

  it('indents the current line from a collapsed caret inside it', () => {
    expect(indentLines('one\ntwo', 5, 5).value).toBe('one\n  two')
  })
})

describe('outdentLines', () => {
  it('removes one indent step per line', () => {
    expect(outdentLines('  one\n  two', 0, 11).value).toBe('one\ntwo')
  })

  it('removes a partial indent rather than overshooting into the text', () => {
    expect(outdentLines(' one', 0, 4).value).toBe('one')
  })

  it('returns null when every line is already at column 0', () => {
    expect(outdentLines('one\ntwo', 0, 7)).toBeNull()
  })

  it('never drags the selection past the start of the line', () => {
    // Caret sits inside the indent itself — the result can't be negative.
    const r = outdentLines('  one', 1, 1)
    expect(r.value).toBe('one')
    expect(r.start).toBe(0)
    expect(r.end).toBe(0)
  })

  it('leaves already-flush lines alone while outdenting the rest', () => {
    const r = outdentLines('one\n  two', 0, 9)
    expect(r.value).toBe('one\ntwo')
  })
})

describe('orderedListPrefix', () => {
  it('numbers from 1 in selection order', () => {
    expect(['a', 'b', 'c'].map(orderedListPrefix)).toEqual(['1. ', '2. ', '3. '])
  })
})
