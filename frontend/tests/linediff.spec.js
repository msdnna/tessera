import { describe, it, expect } from 'vitest'
import { diffSegments } from '@/utils/linediff'

// Only the changed text (not the whole block) should be flagged — the resolver
// shows a focused inline highlight, e.g. a digit appended to a paragraph.
describe('diffSegments', () => {
  const changed = (segs) => segs.filter((s) => s.changed).map((s) => s.text)
  const joined = (segs) => segs.map((s) => s.text).join('')

  it('flags only an appended suffix', () => {
    const segs = diffSegments('менеджере).', 'менеджере).1')
    expect(changed(segs)).toEqual(['1'])
    expect(joined(segs)).toBe('менеджере).1')
  })

  it('flags only a changed middle, keeping shared prefix/suffix', () => {
    const segs = diffSegments('value false here', 'value TRUE here')
    expect(changed(segs)).toEqual(['TRUE'])
  })

  it('returns one unchanged segment for identical text', () => {
    const segs = diffSegments('same', 'same')
    expect(segs).toEqual([{ text: 'same', changed: false }])
  })

  it('treats an empty base as all-changed', () => {
    const segs = diffSegments('', 'new body')
    expect(changed(segs)).toEqual(['new body'])
  })

  // Emoji are surrogate pairs, and neighbours in one block share a half — the
  // shared half must not end up on the other side of the boundary, or the view
  // renders replacement glyphs instead of the character.
  it('keeps a swapped emoji whole when the prefix shares its high half', () => {
    const segs = diffSegments('Готово 😀', 'Готово 😃')
    expect(changed(segs)).toEqual(['😃'])
    expect(joined(segs)).toBe('Готово 😃')
  })

  it('keeps a swapped emoji whole when the suffix shares its low half', () => {
    const segs = diffSegments('𝄞 такт', '𝔞 такт')
    expect(changed(segs)).toEqual(['𝔞'])
    expect(joined(segs)).toBe('𝔞 такт')
  })

  it('never splits a segment mid-pair', () => {
    for (const [base, cur] of [
      ['👍 ок', '👎 ок'],
      ['😀', '😃'],
      ['a😀b', 'a😃b'],
    ]) {
      const segs = diffSegments(base, cur)
      expect(joined(segs)).toBe(cur)
      for (const seg of segs) expect(seg.text).not.toMatch(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/)
    }
  })

  it('preserves newlines in segment text (no per-line split)', () => {
    const segs = diffSegments('a\n\nb.', 'a\n\nb.2')
    expect(changed(segs)).toEqual(['2'])
    expect(joined(segs)).toBe('a\n\nb.2')
  })
})
