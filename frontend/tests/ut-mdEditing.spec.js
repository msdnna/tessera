import { describe, it, expect } from 'vitest'
import {
  WRAP_PAIRS,
  INDENT,
  AUTOCLOSE_PAIRS,
  wrapSelection,
  indentLines,
  outdentLines,
  orderedListPrefix,
  autoClose,
  deletePair,
  handleEnter,
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

describe('autoClose', () => {
  it('inserts the closing half and parks the caret between', () => {
    const r = autoClose('foo', 3, '(')
    expect(r).toEqual({ type: 'insert', value: 'foo()', start: 4, end: 4 })
  })

  it('steps over a closer the caret already sits before', () => {
    // "foo()" with the caret between the brackets, user types ")"
    expect(autoClose('foo()', 4, ')')).toEqual({ type: 'over', caret: 5 })
  })

  it('closes every configured pair', () => {
    for (const [open, close] of Object.entries(AUTOCLOSE_PAIRS)) {
      expect(autoClose('', 0, open).value).toBe(open + close)
    }
  })

  it('leaves emphasis marks to the browser (no auto-pair)', () => {
    expect(autoClose('a', 1, '*')).toBeNull()
    expect(autoClose('a', 1, '~')).toBeNull()
  })

  it('does not trap following text inside a bracket', () => {
    // caret before "bar" — auto-closing "(" here would swallow the word
    expect(autoClose('bar', 0, '(')).toBeNull()
    // but a bracket is fine before whitespace / a closer / end of line
    expect(autoClose('a b', 1, '(')).toBeTruthy()
    expect(autoClose('()', 1, '[')).toBeTruthy()
  })

  it('keeps an apostrophe literal inside a word', () => {
    // "don" + "'" — preceding char is a letter, so no auto-pair
    expect(autoClose('don', 3, "'")).toBeNull()
    // at a boundary it does pair
    expect(autoClose('', 0, "'")).toEqual({ type: 'insert', value: "''", start: 1, end: 1 })
    expect(autoClose('say ', 4, '"')).toBeTruthy()
  })

  it('builds a fenced block from three backticks then a boundary quote', () => {
    // First backtick pairs at the boundary, the second steps over, the third is
    // literal (preceding char is a backtick, not a boundary).
    expect(autoClose('', 0, '`')).toEqual({ type: 'insert', value: '``', start: 1, end: 1 })
    expect(autoClose('``', 1, '`')).toEqual({ type: 'over', caret: 2 })
    expect(autoClose('``', 2, '`')).toBeNull()
  })
})

describe('deletePair', () => {
  it('removes both halves of an empty pair', () => {
    expect(deletePair('a()b', 2)).toEqual({ value: 'ab', start: 1, end: 1 })
    expect(deletePair('""', 1)).toEqual({ value: '', start: 0, end: 0 })
  })

  it('leaves a non-pair alone', () => {
    expect(deletePair('ab', 1)).toBeNull()
    expect(deletePair('(x)', 1)).toBeNull() // not empty between the brackets
  })
})

describe('handleEnter — lists', () => {
  it('continues a bullet with the same marker', () => {
    const r = handleEnter('- one', 5)
    expect(r.value).toBe('- one\n- ')
    expect(r.start).toBe(8)
  })

  it('increments an ordered list', () => {
    expect(handleEnter('1. one', 6).value).toBe('1. one\n2. ')
    expect(handleEnter('3) x', 4).value).toBe('3) x\n4) ')
  })

  it('resets a checkbox to unchecked', () => {
    expect(handleEnter('- [x] done', 10).value).toBe('- [x] done\n- [ ] ')
    expect(handleEnter('  - [ ] a', 9).value).toBe('  - [ ] a\n  - [ ] ')
  })

  it('preserves indentation', () => {
    expect(handleEnter('  - a', 5).value).toBe('  - a\n  - ')
  })

  it('ends the list when the item is empty', () => {
    // Enter on an empty "- " clears the marker rather than adding another bullet.
    expect(handleEnter('- one\n- ', 8)).toEqual({ value: '- one\n', start: 6, end: 6 })
    expect(handleEnter('1. ', 3)).toEqual({ value: '', start: 0, end: 0 })
  })

  it('returns null on a non-list line', () => {
    expect(handleEnter('plain text', 10)).toBeNull()
  })
})

describe('handleEnter — code fences', () => {
  it('drops a closing fence under an opening one', () => {
    const r = handleEnter('```', 3)
    expect(r.value).toBe('```\n\n```')
    expect(r.start).toBe(4) // caret on the empty middle line
  })

  it('keeps the language and indentation on the opening fence', () => {
    expect(handleEnter('```js', 5).value).toBe('```js\n\n```')
  })

  it('does not double-close a fence that already has a closer below', () => {
    // caret at the end of the opening line; a closing ``` exists further down
    expect(handleEnter('```\ncode\n```', 3)).toBeNull()
  })

  it('ignores a fence when the caret is mid-line', () => {
    expect(handleEnter('```js', 3)).toBeNull()
  })
})
