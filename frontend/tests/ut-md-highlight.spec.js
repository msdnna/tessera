import { describe, it, expect } from 'vitest'
import { highlightMarkdown } from '@/utils/mdHighlight'
import { formatAction, modeAction } from '@/utils/edKeys'

// The tint layer is laid over the textarea: whatever it marks, the text it
// carries must be the source, character for character.
const plain = (html) =>
  html
    .replace(/<[^>]+>/g, '')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&amp;/g, '&')

describe('highlightMarkdown (#3118)', () => {
  const SRC = [
    '## Цель',
    'Убрать 1px-разделители и перейти на **лестницу поверхностей**: фон.',
    '',
    '- [x] Токены поверхностей `--v-*`',
    '- [ ] Проверка в тёмной теме — @Анна Ким',
    'Связано с #2936, a < b & c',
  ].join('\n')

  it('keeps every character of the source, in order', () => {
    expect(plain(highlightMarkdown(SRC, { mentions: ['Анна Ким'] }))).toBe(SRC)
  })

  it('fades the markers and bolds a heading', () => {
    const html = highlightMarkdown(SRC)
    expect(html).toContain('<span class="mh-mk">## </span><span class="mh-h">Цель</span>')
    expect(html).toContain('<span class="mh-mk">**</span>лестницу поверхностей')
    expect(html).toContain('<span class="mh-mk">- [x] </span>')
    expect(html).toContain('<span class="mh-mk">`</span>--v-*<span class="mh-mk">`</span>')
  })

  it('takes a known multi-word name whole, and a #N reference', () => {
    const html = highlightMarkdown(SRC, { mentions: ['Анна Ким'] })
    expect(html).toContain('<span class="mh-ref">@Анна Ким</span>')
    expect(html).toContain('<span class="mh-ref">#2936</span>')
  })

  it('leaves an address and C#1 alone', () => {
    const html = highlightMarkdown('mail user@host.io and C#1')
    expect(html).not.toContain('mh-ref')
  })

  it('escapes HTML', () => {
    expect(highlightMarkdown('<b>x</b> & "q"')).toBe('&lt;b&gt;x&lt;/b&gt; &amp; &quot;q&quot;')
  })

  it('marks a /command at the start of a line only where asked', () => {
    expect(highlightMarkdown('/due завтра', { commands: true })).toContain(
      '<span class="mh-cmd">/due</span>',
    )
    expect(highlightMarkdown('/due завтра')).not.toContain('mh-cmd')
    expect(highlightMarkdown('cd /home', { commands: true })).not.toContain('mh-cmd')
  })

  it('draws the open query as a chip on top of other marks', () => {
    const src = 'Посмотри @ан'
    const html = highlightMarkdown(src, { active: { start: 9, end: 12 } })
    expect(html).toContain('<span class="mh-ref mh-q">@ан</span>')
    expect(plain(html)).toBe(src)
  })

  it('leaves the inside of a code fence and of a code span alone', () => {
    const html = highlightMarkdown('```\n## not a heading @x\n```\n`**no**`')
    expect(html).not.toContain('mh-h')
    expect(html).not.toContain('mh-ref')
    expect(html).toContain('<span class="mh-mk">`</span>**no**<span class="mh-mk">`</span>')
  })

  it('no bold where the field is proportional', () => {
    expect(highlightMarkdown('## Цель', { bold: false })).not.toContain('mh-h')
  })

  it('pads a trailing line break so the caret line exists', () => {
    expect(highlightMarkdown('a\n')).toBe('a\n ')
  })
})

describe('edKeys (#3118)', () => {
  const key = (code, o = {}) => new KeyboardEvent('keydown', { code, key: '', ...o })

  it('reads the formatting keys by physical key (jsdom: Ctrl is the mod)', () => {
    expect(formatAction(key('KeyB', { ctrlKey: true, key: 'и' }))).toBe('bold')
    expect(formatAction(key('KeyI', { ctrlKey: true }))).toBe('italic')
    expect(formatAction(key('KeyX', { ctrlKey: true, shiftKey: true }))).toBe('strike')
    expect(formatAction(key('KeyE', { ctrlKey: true }))).toBe('code')
    expect(formatAction(key('KeyK', { ctrlKey: true, shiftKey: true }))).toBe('link')
    expect(formatAction(key('Digit7', { ctrlKey: true, shiftKey: true, key: '&' }))).toBe(
      'orderedList',
    )
    expect(formatAction(key('Digit8', { ctrlKey: true, shiftKey: true, key: '*' }))).toBe(
      'bulletList',
    )
  })

  it('leaves ⌘K to the palette and plain letters to the text', () => {
    expect(formatAction(key('KeyK', { ctrlKey: true }))).toBeNull()
    expect(formatAction(key('KeyB'))).toBeNull()
    expect(formatAction(key('KeyB', { ctrlKey: true, isComposing: true }))).toBeNull()
  })

  it('reads the fullscreen modes', () => {
    expect(modeAction(key('Digit1', { ctrlKey: true }))).toBe('write')
    expect(modeAction(key('Digit2', { ctrlKey: true }))).toBe('split')
    expect(modeAction(key('Digit3', { ctrlKey: true }))).toBe('preview')
    expect(modeAction(key('Digit3'))).toBeNull()
  })
})
