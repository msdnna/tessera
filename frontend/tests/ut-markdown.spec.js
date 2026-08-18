import { describe, it, expect } from 'vitest'
import {
  renderMarkdown,
  renderRich,
  replaceOutsideCode,
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

  it('highlights a mention written as the login', () => {
    const html = renderRich('hi @e.polyansky', [{ label: 'e.polyansky', display: 'Ann Lee' }])
    expect(html).toContain('class="mention" data-type="mention"')
    expect(html).toContain('>@e.polyansky</span>')
  })

  it('still highlights the full name in comments written before the login switch', () => {
    // label is the login now, but the stored comment holds the old display name.
    const html = renderRich('hi @Ann Lee', [{ label: 'e.polyansky', display: 'Ann Lee' }])
    expect(html).toContain('>@Ann Lee</span>')
    expect(html).toContain('data-label="Ann Lee"')
  })

  it('prefers the longer name when one is a prefix of another', () => {
    const html = renderRich('hi @Ann Lee Smith', [
      { label: 'a.lee', display: 'Ann Lee' },
      { label: 'a.lee.smith', display: 'Ann Lee Smith' },
    ])
    expect(html).toContain('>@Ann Lee Smith</span>')
  })

  it('returns "" for empty input', () => {
    expect(renderRich('')).toBe('')
    expect(renderRich(null)).toBe('')
  })

  // Behaviour change (#2717): mentions used to be highlighted inside code too,
  // because the regexp knew nothing about <code>/<pre>. Task refs made that
  // untenable — "#2550" shows up in diffs and snippets constantly — so both
  // decorations now run only outside code.
  it('leaves @handles inside code spans and fences alone', () => {
    const inline = renderRich('run `sudo -u @root` now', [])
    expect(inline).toContain('<code>sudo -u @root</code>')
    expect(inline).not.toContain('<code>sudo -u <span class="mention">')

    const fenced = renderRich('```\ncurl @host\n```', [])
    expect(fenced).not.toContain('class="mention"')
  })

  it('links #N task references only when asked', () => {
    const off = renderRich('see #2550', [])
    expect(off).not.toContain('task-ref')

    const on = renderRich('see #2550', [], { taskRefs: true })
    expect(on).toContain('data-task-ref="2550"')
    expect(on).toContain('>#2550</a>')
  })

  it('does not link #N inside code', () => {
    const html = renderRich('fix `#2550` and\n\n```\n-#123\n```', [], { taskRefs: true })
    expect(html).toContain('<code>#2550</code>')
    expect(html).not.toContain('data-task-ref')
  })

  it('ignores over-long digit runs and hex-looking colours', () => {
    const html = renderRich('#12345678 and #1a2b3c', [], { taskRefs: true })
    expect(html).not.toContain('data-task-ref')
  })

  it('keeps <details>/<summary> spoilers through the sanitiser', () => {
    const html = renderRich('<details><summary>Подробнее</summary>\n\n**тайна**\n\n</details>', [])
    expect(html).toContain('<details>')
    expect(html).toContain('<summary>Подробнее</summary>')
    expect(html).toContain('<strong>тайна</strong>')
  })
})

describe('replaceOutsideCode', () => {
  it('applies the callback only outside code elements', () => {
    const out = replaceOutsideCode('a<code>b</code>c', (part) => part.toUpperCase())
    expect(out).toBe('A<code>b</code>C')
  })

  it('treats a <pre><code> fence as a single untouched block', () => {
    const out = replaceOutsideCode('x<pre><code>y</code></pre>z', (part) => part.toUpperCase())
    expect(out).toBe('X<pre><code>y</code></pre>Z')
  })

  it('passes content through untouched when there is no code at all', () => {
    expect(replaceOutsideCode('plain', (p) => `[${p}]`)).toBe('[plain]')
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
