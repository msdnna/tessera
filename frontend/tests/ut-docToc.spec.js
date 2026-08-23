import { describe, it, expect } from 'vitest'
import {
  untitledHeading,
  docHeadings,
  docOutline,
  headingForBlock,
  headingLabel,
  internalHref,
  internalTargetId,
  tocRows,
  tocTree,
} from '@/utils/docToc'

function h(id, level, text) {
  return {
    type: 'heading',
    attrs: { id, level },
    content: text === undefined ? [] : [{ type: 'text', text }],
  }
}

function p(id, text) {
  return { type: 'paragraph', attrs: { id }, content: [{ type: 'text', text: text || 'текст' }] }
}

describe('docHeadings', () => {
  it('collects headings in reading order with their level and text', () => {
    const doc = {
      type: 'doc',
      content: [p('p1'), h('h1', 1, 'Введение'), p('p2'), h('h2', 2, 'Область применения')],
    }
    expect(docHeadings(doc)).toEqual([
      { id: 'h1', level: 1, text: 'Введение' },
      { id: 'h2', level: 2, text: 'Область применения' },
    ])
  })

  it('finds a heading nested inside another block', () => {
    // A heading inside a blockquote or a table cell is visibly a section on the
    // page; leaving it out would make the outline skip something the reader can
    // see.
    const doc = {
      type: 'doc',
      content: [{ type: 'blockquote', attrs: { id: 'q' }, content: [h('h1', 2, 'Из цитаты')] }],
    }
    expect(docHeadings(doc).map((x) => x.id)).toEqual(['h1'])
  })

  it('keeps a heading that has no text yet', () => {
    // A heading is created empty and typed into afterwards. Dropping it until
    // the first character would make the outline jump under the cursor.
    const rows = docHeadings({ type: 'doc', content: [h('h1', 1)] })
    expect(rows).toHaveLength(1)
    expect(headingLabel(rows[0])).toBe(untitledHeading())
  })

  it('ignores a heading with no block id, since nothing could link to it', () => {
    const doc = { type: 'doc', content: [{ type: 'heading', attrs: { level: 1 } }] }
    expect(docHeadings(doc)).toEqual([])
  })

  it('clamps a level outside 1..6 instead of trusting it', () => {
    const doc = { type: 'doc', content: [h('a', 0, 'x'), h('b', 99, 'y'), h('c', null, 'z')] }
    expect(docHeadings(doc).map((x) => x.level)).toEqual([1, 6, 1])
  })

  it('survives an empty or malformed document', () => {
    expect(docHeadings(null)).toEqual([])
    expect(docHeadings({ type: 'doc' })).toEqual([])
  })
})

describe('tocTree', () => {
  it('nests each heading under the nearest smaller level above it', () => {
    const tree = tocTree(docHeadings({ type: 'doc', content: [h('a', 1, 'A'), h('b', 2, 'B')] }))
    expect(tree).toHaveLength(1)
    expect(tree[0].children.map((c) => c.id)).toEqual(['b'])
    expect(tree[0].children[0].depth).toBe(1)
  })

  it('indents by one step per nesting level, not by the level number', () => {
    // A document that starts at h2 and drops to h4 must not render with two
    // empty indent steps in front of every entry: what the outline shows is the
    // structure the author wrote, not the numbers they happened to use.
    const rows = docOutline({
      type: 'doc',
      content: [h('a', 2, 'A'), h('b', 4, 'B'), h('c', 4, 'C')],
    })
    expect(rows.map((r) => [r.id, r.depth])).toEqual([
      ['a', 0],
      ['b', 1],
      ['c', 1],
    ])
  })

  it('starts a new root when the level goes back up', () => {
    const rows = docOutline({
      type: 'doc',
      content: [h('a', 2, 'A'), h('b', 3, 'B'), h('c', 1, 'C')],
    })
    expect(rows.map((r) => [r.id, r.depth])).toEqual([
      ['a', 0],
      ['b', 1],
      ['c', 0],
    ])
  })

  it('flattens back into reading order', () => {
    const headings = docHeadings({
      type: 'doc',
      content: [h('a', 1, 'A'), h('b', 2, 'B'), h('c', 3, 'C'), h('d', 1, 'D')],
    })
    expect(tocRows(tocTree(headings)).map((r) => r.id)).toEqual(['a', 'b', 'c', 'd'])
  })
})

describe('internal links', () => {
  it('round-trips a block id through an href', () => {
    expect(internalHref('abc123')).toBe('#abc123')
    expect(internalTargetId(internalHref('abc123'))).toBe('abc123')
  })

  it('treats anything that is not a bare fragment as external', () => {
    // "/documents/x#block" is a route change and has to stay the router's
    // business: following it inside the editor would leave the URL pointing at
    // one document while another is on screen.
    expect(internalTargetId('/documents/x#block')).toBe('')
    expect(internalTargetId('https://example.com/#a')).toBe('')
    expect(internalTargetId('#')).toBe('')
    expect(internalTargetId('')).toBe('')
    expect(internalTargetId(null)).toBe('')
  })

  it('has no href for a missing id', () => {
    expect(internalHref('')).toBe('')
  })
})

describe('headingForBlock', () => {
  const doc = {
    type: 'doc',
    content: [p('intro'), h('h1', 1, 'Первый'), p('a'), h('h2', 2, 'Второй'), p('b')],
  }

  it('reports the nearest heading above the block', () => {
    expect(headingForBlock(doc, 'a')).toBe('h1')
    expect(headingForBlock(doc, 'b')).toBe('h2')
  })

  it('lets a heading belong to itself', () => {
    // The caret sitting on «Второй» should highlight «Второй» in the outline,
    // not the section above it.
    expect(headingForBlock(doc, 'h2')).toBe('h2')
  })

  it('reports no section for text above the first heading', () => {
    expect(headingForBlock(doc, 'intro')).toBe('')
  })

  it('reports no section for a block that is not in the document', () => {
    expect(headingForBlock(doc, 'gone')).toBe('')
    expect(headingForBlock(doc, '')).toBe('')
  })
})
