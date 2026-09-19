// renderRich decorates already-rendered HTML with a regex pass: «#123» becomes a
// task link and «@handle» a mention chip. The pass must see the DISPLAYED TEXT
// only — an attribute is not text, and marked writes user prose into two of them
// (an image's alt, a link's title). Splicing an element inside `alt="…"` ends the
// attribute at the chip's first quote, which destroyed the attribute, grew junk
// attributes on the element and spilled the tail of the chip into the page as
// visible text (#2932).
//
// The controls below are the other half: skipping tags must not cost the
// decorations a single case they used to handle — including the one where a
// mention or a ref opens a text run directly after a tag, whose left boundary
// used to be the `>` and is now the run's own start.

import { describe, it, expect } from 'vitest'
import { renderRich, replaceOutsideMarkup } from '@/utils/markdown'

const MEMBERS = [{ id: 'u1', label: 'ivan', display: 'Иван Петров' }]
const rich = (src) => renderRich(src, MEMBERS, { taskRefs: true })

describe('decorations stay out of tag markup', () => {
  it('leaves a task ref in an image alt as plain text', () => {
    const html = rich('![скрин #123](/api/files/a.png)')
    expect(html).toContain('alt="скрин #123"')
    expect(html).not.toContain('task-ref')
  })

  it('does not spill the chip tail into the page next to the image', () => {
    const html = rich('![скрин #123](/api/files/a.png)')
    // The broken render ended the alt early and left `#123"&gt;` standing in the
    // paragraph, after the <img>, as text the reader could see and select.
    expect(html).not.toContain('&gt;')
    expect(html).toContain('<img')
  })

  it('leaves a mention in an image alt as plain text', () => {
    const html = rich('![привет @ivan тут](/api/files/a.png)')
    expect(html).toContain('alt="привет @ivan тут"')
    expect(html).not.toContain('class="mention"')
  })

  it('keeps a link title carrying a task ref', () => {
    const html = rich('[тык](/x "см #123 тут")')
    expect(html).toContain('title="см #123 тут"')
    expect(html).not.toContain('task-ref')
  })

  it('keeps the anchor text when its title carries a mention', () => {
    const html = rich('[тык](/x "спроси @ivan")')
    expect(html).toContain('title="спроси @ivan"')
    // The anchor's own text was swallowed by the wrecked attribute: the
    // paragraph rendered `тут"&gt;тык` instead of a link reading «тык».
    expect(html).toContain('>тык</a>')
  })

  it('does not invent attributes on the decorated element', () => {
    const html = rich('![скрин #123](/a.png)')
    expect(html).not.toContain('data-task-ref')
  })
})

describe('decorations still reach the text', () => {
  it('links a bare task ref', () => {
    expect(rich('см #123 дальше')).toContain('data-task-ref="123"')
  })

  it('chips a known handle', () => {
    const html = rich('привет @ivan')
    expect(html).toContain('class="mention"')
    expect(html).toContain('data-id="u1"')
  })

  it('chips a handle that opens a paragraph (no lead character at all)', () => {
    expect(rich('@ivan, глянь')).toContain('class="mention"')
  })

  it('links a ref that opens a paragraph', () => {
    expect(rich('#123 — вот это')).toContain('data-task-ref="123"')
  })

  it('decorates the text of a link, not its markup', () => {
    const html = rich('[@ivan](/u/ivan)')
    expect(html).toContain('class="mention"')
    expect(html).toContain('href="/u/ivan"')
  })

  it('decorates a run that starts right after an inline tag', () => {
    const html = rich('**важно** #123')
    expect(html).toContain('<strong>важно</strong>')
    expect(html).toContain('data-task-ref="123"')
  })

  it('leaves a ref inside inline code alone', () => {
    const html = rich('см `#123`')
    expect(html).toContain('<code>#123</code>')
    expect(html).not.toContain('task-ref')
  })

  it('leaves a handle inside a fenced block alone', () => {
    const html = rich('```\nsudo -u @ivan\n```')
    expect(html).not.toContain('class="mention"')
  })

  it('still chips a multi-word display name', () => {
    expect(rich('@Иван Петров, глянь')).toContain('data-id="u1"')
  })

  it('still refuses to chip a longer word that merely opens with a handle', () => {
    expect(rich('@Иван Петрович')).not.toContain('class="mention"')
    expect(rich('@ivanov')).not.toContain('data-id="u1"')
  })
})

describe('replaceOutsideMarkup', () => {
  it('skips tag markup as well as code', () => {
    const out = replaceOutsideMarkup('a<b class="x">c</b><code>d</code>e', (p) => p.toUpperCase())
    expect(out).toBe('A<b class="x">C</b><code>d</code>E')
  })

  it('hands the callback an attribute value never, not even a quoted one', () => {
    const seen = []
    replaceOutsideMarkup('<img alt="жил #1" src="/a.png">хвост', (p) => {
      seen.push(p)
      return p
    })
    expect(seen.join('')).toBe('хвост')
  })
})
