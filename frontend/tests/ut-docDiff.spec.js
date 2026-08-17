import { describe, it, expect } from 'vitest'
import {
  DIFF_ADDED,
  DIFF_CHANGED,
  DIFF_MOVED,
  DIFF_REMOVED,
  DIFF_SAME,
  diffDocs,
  diffSummary,
  docBlocks,
} from '@/utils/docDiff'

// A paragraph with a stable block id — the anchor D2 stamps on every top-level
// node, and the thing this diff is built on.
const p = (id, text) => ({
  type: 'paragraph',
  attrs: { id },
  content: [{ type: 'text', text }],
})
const doc = (...blocks) => ({ type: 'doc', content: blocks })

const statuses = (rows) => rows.map((r) => r.status)
const texts = (rows) => rows.map((r) => r.text)

describe('docDiff', () => {
  it('reports an untouched document as unchanged', () => {
    const d = doc(p('a', 'первый'), p('b', 'второй'))
    const rows = diffDocs(d, d)
    expect(statuses(rows)).toEqual([DIFF_SAME, DIFF_SAME])
    expect(diffSummary(rows).identical).toBe(true)
  })

  it('marks an edited block changed and keeps its previous wording', () => {
    const rows = diffDocs(
      doc(p('a', 'было'), p('b', 'общий')),
      doc(p('a', 'стало'), p('b', 'общий')),
    )
    expect(statuses(rows)).toEqual([DIFF_CHANGED, DIFF_SAME])
    // Without the old text, "изменено" asks the reader to remember what they
    // opened the comparison to look up.
    expect(rows[0].prevText).toBe('было')
    expect(rows[0].text).toBe('стало')
  })

  // The payoff of anchoring on block ids: a dragged paragraph (D3) is one moved
  // block, not a deletion plus an unrelated addition somewhere else.
  it('reports a dragged paragraph as moved, not as deleted and added', () => {
    const rows = diffDocs(
      doc(p('a', 'первый'), p('b', 'второй'), p('c', 'третий')),
      doc(p('b', 'второй'), p('a', 'первый'), p('c', 'третий')),
    )
    expect(statuses(rows)).toEqual([DIFF_MOVED, DIFF_MOVED, DIFF_SAME])
    const s = diffSummary(rows)
    expect(s.added).toBe(0)
    expect(s.removed).toBe(0)
  })

  it('does not call blocks moved just because one above them was deleted', () => {
    const rows = diffDocs(
      doc(p('a', 'первый'), p('b', 'второй'), p('c', 'третий')),
      doc(p('b', 'второй'), p('c', 'третий')),
    )
    // The deleted block was the first one, so its row heads the list; the two
    // that survived shifted up in the document but did not move relative to each
    // other, and reporting them as moved is the noise this guards against.
    expect(statuses(rows)).toEqual([DIFF_REMOVED, DIFF_SAME, DIFF_SAME])
    expect(diffSummary(rows)).toMatchObject({ removed: 1, moved: 0, added: 0 })
  })

  it('places a deletion where it happened, not at the end', () => {
    const rows = diffDocs(
      doc(p('a', 'первый'), p('b', 'второй'), p('c', 'третий')),
      doc(p('a', 'первый'), p('c', 'третий')),
    )
    expect(statuses(rows)).toEqual([DIFF_SAME, DIFF_REMOVED, DIFF_SAME])
    expect(texts(rows)).toEqual(['первый', 'второй', 'третий'])
  })

  it('keeps two deletions after the same block in their original order', () => {
    const rows = diffDocs(
      doc(p('a', 'первый'), p('b', 'второй'), p('c', 'третий'), p('d', 'четвёртый')),
      doc(p('a', 'первый'), p('d', 'четвёртый')),
    )
    expect(texts(rows)).toEqual(['первый', 'второй', 'третий', 'четвёртый'])
    expect(statuses(rows)).toEqual([DIFF_SAME, DIFF_REMOVED, DIFF_REMOVED, DIFF_SAME])
  })

  it('puts a deletion above every surviving block back at the top', () => {
    const rows = diffDocs(doc(p('a', 'вступление'), p('b', 'основное')), doc(p('b', 'основное')))
    expect(statuses(rows)).toEqual([DIFF_REMOVED, DIFF_SAME])
    expect(texts(rows)).toEqual(['вступление', 'основное'])
  })

  it('counts a new block as added', () => {
    const rows = diffDocs(doc(p('a', 'первый')), doc(p('a', 'первый'), p('b', 'новый абзац')))
    expect(statuses(rows)).toEqual([DIFF_SAME, DIFF_ADDED])
    expect(diffSummary(rows)).toMatchObject({ added: 1, removed: 0, identical: false })
  })

  // Formatting is content: a heading that became level 3, a link that now points
  // elsewhere. The flattened text is identical in both cases.
  it('sees a change that the plain text does not show', () => {
    const before = {
      type: 'doc',
      content: [
        {
          type: 'heading',
          attrs: { id: 'h', level: 2 },
          content: [{ type: 'text', text: 'Раздел' }],
        },
      ],
    }
    const after = {
      type: 'doc',
      content: [
        {
          type: 'heading',
          attrs: { id: 'h', level: 3 },
          content: [{ type: 'text', text: 'Раздел' }],
        },
      ],
    }
    expect(statuses(diffDocs(before, after))).toEqual([DIFF_CHANGED])
  })

  // Imported documents and anything written before D2 have no ids; positional
  // pairing is the best available answer, and it must not degrade into "the
  // whole document was replaced".
  it('falls back to position for blocks without ids', () => {
    const anon = (text) => ({ type: 'paragraph', content: [{ type: 'text', text }] })
    const rows = diffDocs(doc(anon('первый'), anon('второй')), doc(anon('первый'), anon('другой')))
    expect(statuses(rows)).toEqual([DIFF_SAME, DIFF_CHANGED])
  })

  it('describes an empty document against a filled one', () => {
    const rows = diffDocs({ type: 'doc', content: [] }, doc(p('a', 'первая строка')))
    expect(statuses(rows)).toEqual([DIFF_ADDED])
    expect(diffDocs(doc(p('a', 'первая строка')), { type: 'doc', content: [] })).toHaveLength(1)
  })

  it('survives a missing or malformed version body', () => {
    expect(diffDocs(null, null)).toEqual([])
    expect(diffDocs(undefined, doc(p('a', 'текст')))).toHaveLength(1)
  })

  it('flattens nested and media blocks into readable text', () => {
    const blocks = docBlocks({
      type: 'doc',
      content: [
        {
          type: 'bulletList',
          attrs: { id: 'l' },
          content: [
            {
              type: 'listItem',
              content: [{ type: 'paragraph', content: [{ type: 'text', text: 'раз' }] }],
            },
            {
              type: 'listItem',
              content: [{ type: 'paragraph', content: [{ type: 'text', text: 'два' }] }],
            },
          ],
        },
        { type: 'image', attrs: { id: 'i', src: '/api/documents/asset?x=1', alt: 'схема' } },
      ],
    })
    expect(blocks[0].text).toBe('раз два')
    // An image has no text of its own; its alt is what a reader can act on.
    expect(blocks[1].text).toBe('схема')
  })
})
