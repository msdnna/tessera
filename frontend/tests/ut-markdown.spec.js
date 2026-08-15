import { describe, it, expect } from 'vitest'
import {
  renderMarkdown,
  renderRich,
  sanitizeSvgFragment,
  toggleTaskMarker,
  toEditorHtml,
} from '@/utils/markdown'

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
    expect(html).toContain('>@Ann Lee</span>')
    expect(html).toContain('>@v.sokolov</span>')
    expect(html).toContain('class="mention" data-type="mention"')
  })

  it('stamps the member id on a known mention so hover cards resolve it', () => {
    const html = renderRich('hi @Ann Lee', [{ id: 'u1', label: 'Ann Lee' }])
    expect(html).toContain('data-id="u1"')
    expect(html).toContain('data-label="Ann Lee"')
  })

  it('gives a generic handle a label but no id — it names no known member', () => {
    const html = renderRich('hi @v.sokolov', [{ id: 'u1', label: 'Ann Lee' }])
    expect(html).toContain('data-label="v.sokolov"')
    expect(html).not.toContain('data-id=')
  })

  // A name carrying HTML-special characters is already escaped by the Markdown
  // renderer by the time mentions are matched, so the raw label never matches it
  // and the generic handle token takes over. That predates the data-attributes;
  // what matters here is that the chip stays well-formed and the label read back
  // off the chip is exactly the text it highlighted — no double escaping.
  it('keeps the chip well-formed for a name the renderer escaped', () => {
    const html = renderRich('hi @A&B', [{ id: 'u1', label: 'A&B' }])
    const chip = new DOMParser()
      .parseFromString(html, 'text/html')
      .querySelector('[data-type="mention"]')
    expect(chip.dataset.label).toBe(chip.textContent.replace(/^@/, ''))
    expect(html).not.toContain('&amp;amp;')
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

describe('sanitizeSvgFragment', () => {
  const svg = (inner) => `<svg xmlns="http://www.w3.org/2000/svg">${inner}</svg>`
  // The fragment is what RichContent mounts; serialise it the same way to assert on it.
  const clean = (src) => {
    const host = document.createElement('div')
    const frag = sanitizeSvgFragment(src)
    if (frag) host.append(frag)
    return host.innerHTML
  }

  it('keeps the diagram markup itself', () => {
    const out = clean(svg('<g class="node"><rect width="10" height="10"/></g>'))
    expect(out).toContain('<rect')
    expect(out).toContain('class="node"')
  })

  it('keeps <foreignObject> labels (mermaid draws node text as HTML)', () => {
    const out = clean(svg('<foreignObject><div>Node label</div></foreignObject>'))
    expect(out).toContain('Node label')
    expect(out.toLowerCase()).toContain('foreignobject')
  })

  it('keeps the inline <style> mermaid ships its classes in', () => {
    const out = clean(svg('<style>.node rect{fill:#fff}</style><g class="node"></g>'))
    expect(out).toContain('.node rect')
  })

  it('strips <script> inside the svg', () => {
    const out = clean(svg('<script>alert(1)</script><rect></rect>'))
    expect(out.toLowerCase()).not.toContain('<script')
    expect(out).toContain('<rect')
  })

  it('still sanitises the HTML inside <foreignObject>', () => {
    const out = clean(
      svg('<foreignObject><div><img src="x" onerror="alert(1)">ok</div></foreignObject>'),
    )
    expect(out).not.toContain('onerror')
    expect(out).toContain('ok')
  })

  it('strips <script> smuggled inside <foreignObject>', () => {
    const out = clean(svg('<foreignObject><div><script>alert(1)</script>ok</div></foreignObject>'))
    expect(out.toLowerCase()).not.toContain('<script')
    expect(out).toContain('ok')
  })

  it('strips event handlers on svg elements', () => {
    const out = clean('<svg onload="alert(1)"><rect onclick="alert(2)"></rect></svg>')
    expect(out).not.toContain('onload')
    expect(out).not.toContain('onclick')
  })

  it('strips javascript: hrefs', () => {
    const out = clean(svg('<a href="javascript:alert(1)"><rect></rect></a>'))
    expect(out.toLowerCase()).not.toContain('javascript:')
  })

  it('returns null for empty / falsy input', () => {
    expect(sanitizeSvgFragment('')).toBeNull()
    expect(sanitizeSvgFragment(null)).toBeNull()
    expect(sanitizeSvgFragment(undefined)).toBeNull()
  })
})
