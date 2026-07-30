import { describe, it, expect } from 'vitest'
import { renderMarkdown, renderRich, toggleTaskMarker, toEditorHtml } from '@/utils/markdown'

describe('renderMarkdown', () => {
  it('renders markdown to sanitised html', () => {
    const html = renderMarkdown('# Title\n\n**bold**')
    expect(html).toContain('<h1')
    expect(html).toContain('<strong>bold</strong>')
  })

  it('returns "" for empty input', () => {
    expect(renderMarkdown('')).toBe('')
    expect(renderMarkdown(null)).toBe('')
  })

  it('strips scripts / event handlers', () => {
    const html = renderMarkdown('<img src=x onerror="alert(1)">')
    expect(html).not.toContain('onerror')
    expect(html.toLowerCase()).not.toContain('<script')
  })

  it('highlights a known fenced language and escapes an unknown one', () => {
    const js = renderMarkdown('```js\nconst a = 1\n```')
    expect(js).toContain('hljs')
    expect(js).toContain('language-js')
    const unknown = renderMarkdown('```notalang\n<x>&\n```')
    expect(unknown).toContain('&lt;x&gt;')
  })

  it('leaves mermaid fences as escaped plain text (arrows not hljs-tokenised)', () => {
    const html = renderMarkdown('```mermaid\ngraph TD; A-->B\n```')
    expect(html).toContain('graph TD')
    // escaped, not turned into highlight.js token spans
    expect(html).toContain('A--&gt;B')
    expect(html).not.toContain('<span class="hljs-')
  })
})

describe('renderRich', () => {
  it('renders markdown when content is not block-html', () => {
    expect(renderRich('*em*')).toContain('<em>em</em>')
  })

  it('passes through content starting with a block tag', () => {
    const html = renderRich('<p>hello **not-md**</p>')
    expect(html).toContain('hello **not-md**') // not re-parsed as markdown
  })

  it('highlights known member display names before generic handles', () => {
    const html = renderRich('hi @Ann Lee and @v.sokolov', [{ label: 'Ann Lee' }])
    expect(html).toContain('<span class="mention">@Ann Lee</span>')
    expect(html).toContain('<span class="mention">@v.sokolov</span>')
  })

  it('returns "" for empty input', () => {
    expect(renderRich('')).toBe('')
    expect(renderRich(null)).toBe('')
  })
})

describe('toggleTaskMarker', () => {
  const src = '- [ ] one\n- [x] two\n- [ ] three'

  it('checks an unchecked item by render index', () => {
    expect(toggleTaskMarker(src, 0)).toBe('- [x] one\n- [x] two\n- [ ] three')
  })

  it('unchecks a checked item (case-insensitive marker)', () => {
    expect(toggleTaskMarker('- [X] done', 0)).toBe('- [ ] done')
  })

  it('supports ordered lists and *,+ bullets', () => {
    expect(toggleTaskMarker('1. [ ] a', 0)).toBe('1. [x] a')
    expect(toggleTaskMarker('* [ ] a', 0)).toBe('* [x] a')
    expect(toggleTaskMarker('+ [ ] a', 0)).toBe('+ [x] a')
  })

  it('returns the source unchanged when the index is out of range', () => {
    expect(toggleTaskMarker(src, 9)).toBe(src)
  })

  it('passes through empty / falsy source', () => {
    expect(toggleTaskMarker('', 0)).toBe('')
    expect(toggleTaskMarker(null, 0)).toBeNull()
  })
})

describe('toEditorHtml', () => {
  it('converts markdown, passes html through, and empties on falsy', () => {
    expect(toEditorHtml('**b**')).toContain('<strong>b</strong>')
    expect(toEditorHtml('<p>raw</p>')).toBe('<p>raw</p>')
    expect(toEditorHtml('')).toBe('')
  })
})
