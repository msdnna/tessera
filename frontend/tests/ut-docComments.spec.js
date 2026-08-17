import { describe, it, expect } from 'vitest'
import {
  authorLabel,
  blockIdsIn,
  blockIdsInOrder,
  buildThreads,
  openCountByBlock,
  quoteFromBlock,
  sortThreads,
  splitThreads,
} from '@/utils/docComments'

// Comment rows as the API returns them (flat, roots and replies together).
function cm(id, over = {}) {
  return {
    id,
    block_id: '',
    parent_id: null,
    body: `тело ${id}`,
    quote: '',
    resolved_at: null,
    created_at: '2026-08-15T10:00:00Z',
    ...over,
  }
}

describe('blockIdsIn', () => {
  it('collects ids from the whole tree, not just the top level', () => {
    const doc = {
      type: 'doc',
      content: [
        { type: 'paragraph', attrs: { id: 'a' } },
        {
          type: 'bulletList',
          attrs: { id: 'b' },
          content: [{ type: 'listItem', content: [{ type: 'paragraph', attrs: { id: 'c' } }] }],
        },
      ],
    }
    expect(blockIdsIn(doc)).toEqual(new Set(['a', 'b', 'c']))
  })

  it('survives an empty or malformed document', () => {
    expect(blockIdsIn(null).size).toBe(0)
    expect(blockIdsIn({ type: 'doc' }).size).toBe(0)
  })
})

describe('blockIdsInOrder', () => {
  it('returns the ids in reading order', () => {
    const doc = {
      type: 'doc',
      content: [
        { type: 'paragraph', attrs: { id: 'a' } },
        {
          type: 'bulletList',
          attrs: { id: 'b' },
          content: [{ type: 'listItem', content: [{ type: 'paragraph', attrs: { id: 'c' } }] }],
        },
        { type: 'paragraph', attrs: { id: 'd' } },
      ],
    }
    expect(blockIdsInOrder(doc)).toEqual(['a', 'b', 'c', 'd'])
  })

  it('holds the same ids as blockIdsIn', () => {
    const doc = { type: 'doc', content: [{ type: 'paragraph', attrs: { id: 'a' } }] }
    expect(new Set(blockIdsInOrder(doc))).toEqual(blockIdsIn(doc))
  })

  it('survives an empty or malformed document', () => {
    expect(blockIdsInOrder(null)).toEqual([])
    expect(blockIdsInOrder({ type: 'doc' })).toEqual([])
  })
})

describe('buildThreads', () => {
  it('hangs replies under their root, oldest first', () => {
    const threads = buildThreads([
      cm('r1', { block_id: 'a' }),
      cm('x1', { parent_id: 'r1', created_at: '2026-08-15T10:01:00Z' }),
      cm('x2', { parent_id: 'r1', created_at: '2026-08-15T10:02:00Z' }),
    ])
    expect(threads).toHaveLength(1)
    expect(threads[0].replies.map((r) => r.id)).toEqual(['x1', 'x2'])
  })

  it('drops a reply whose root is missing rather than promoting it', () => {
    // A promoted reply renders as an answer to a question nobody can see, and —
    // worse — as a thread that can be resolved on its own.
    const threads = buildThreads([cm('x1', { parent_id: 'gone' })])
    expect(threads).toEqual([])
  })
})

describe('sortThreads', () => {
  it('puts unresolved first and the newest of each group on top', () => {
    const threads = sortThreads([
      { id: 'old', created_at: '2026-08-14T10:00:00Z', resolved_at: null },
      { id: 'done', created_at: '2026-08-15T12:00:00Z', resolved_at: '2026-08-15T13:00:00Z' },
      { id: 'new', created_at: '2026-08-15T11:00:00Z', resolved_at: null },
    ])
    expect(threads.map((t) => t.id)).toEqual(['new', 'old', 'done'])
  })

  it('does not mutate its input', () => {
    const input = [{ id: 'a', resolved_at: '2026-08-15T13:00:00Z' }, { id: 'b' }]
    sortThreads(input)
    expect(input.map((t) => t.id)).toEqual(['a', 'b'])
  })
})

describe('splitThreads', () => {
  it('separates anchored, document-level and detached threads', () => {
    const threads = [
      { id: '1', block_id: 'here' },
      { id: '2', block_id: '' },
      { id: '3', block_id: 'deleted' },
    ]
    const out = splitThreads(threads, new Set(['here']))
    expect(out.anchored.map((t) => t.id)).toEqual(['1'])
    expect(out.document.map((t) => t.id)).toEqual(['2'])
    expect(out.detached.map((t) => t.id)).toEqual(['3'])
  })

  it('orders anchored threads by document position when given an ordered list', () => {
    // The panel's card order is what keeps the annotation lines from crossing:
    // cards and blocks then run the same way down the screen.
    const threads = [
      { id: 'late', block_id: 'c' },
      { id: 'early', block_id: 'a' },
      { id: 'mid', block_id: 'b' },
    ]
    const out = splitThreads(threads, ['a', 'b', 'c'])
    expect(out.anchored.map((t) => t.id)).toEqual(['early', 'mid', 'late'])
  })

  it('keeps the incoming order within one block', () => {
    // Inside a block the sortThreads order (open first, newest first) must
    // survive — the document sort only decides between blocks.
    const threads = [
      { id: 'open', block_id: 'a' },
      { id: 'done', block_id: 'a' },
    ]
    const out = splitThreads(threads, ['a'])
    expect(out.anchored.map((t) => t.id)).toEqual(['open', 'done'])
  })

  it('leaves the order alone when given a Set', () => {
    const threads = [
      { id: 'late', block_id: 'c' },
      { id: 'early', block_id: 'a' },
    ]
    const out = splitThreads(threads, new Set(['a', 'c']))
    expect(out.anchored.map((t) => t.id)).toEqual(['late', 'early'])
  })

  it('keeps a thread whose block was deleted instead of dropping it', () => {
    // The whole point: rewriting a paragraph must not delete the discussion that
    // asked for the rewrite.
    const out = splitThreads([{ id: '1', block_id: 'gone', body: 'нужен срок' }], new Set())
    expect(out.detached).toHaveLength(1)
    expect(out.anchored).toHaveLength(0)
  })
})

describe('openCountByBlock', () => {
  it('counts only unresolved threads, per block', () => {
    const counts = openCountByBlock([
      { block_id: 'a', resolved_at: null },
      { block_id: 'a', resolved_at: null },
      { block_id: 'a', resolved_at: '2026-08-15T13:00:00Z' },
      { block_id: '', resolved_at: null },
    ])
    expect(counts.get('a')).toBe(2)
    // A settled discussion stops marking up the text, and a document-level
    // thread has no block to mark.
    expect(counts.has('')).toBe(false)
  })
})

describe('quoteFromBlock', () => {
  it('takes the text of a node tree and collapses whitespace', () => {
    const node = {
      type: 'paragraph',
      content: [{ type: 'text', text: 'Исполнитель\n  обязан ' }],
    }
    expect(quoteFromBlock(node)).toBe('Исполнитель обязан')
  })

  it('truncates with an ellipsis so a thread cannot carry a page', () => {
    const node = { type: 'paragraph', content: [{ type: 'text', text: 'я'.repeat(300) }] }
    const quote = quoteFromBlock(node, 20)
    expect(quote).toHaveLength(21) // 20 characters plus the ellipsis
    expect(quote.endsWith('…')).toBe(true)
  })

  it('accepts a live ProseMirror node via textContent', () => {
    expect(quoteFromBlock({ textContent: '  Пункт  4  ' })).toBe('Пункт 4')
  })
})

describe('authorLabel', () => {
  it('falls back the same way presence badges do', () => {
    expect(authorLabel({ author_name: 'Иван Петров' })).toBe('Иван Петров')
    expect(authorLabel({ author_name: '  ', author_email: 'ivan@test.local' })).toBe('ivan')
    expect(authorLabel({})).toBe('Участник')
  })
})
