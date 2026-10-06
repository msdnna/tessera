// Block ids when the randomness source is missing or degraded (#2932).
//
// The ids stamped here are the anchors D4 locks, D5 annotates and #2733 links
// to. A document whose blocks share one id does not fail anywhere: it renders,
// saves and syncs — while one caret marks every paragraph as held, an
// annotation shows on all of them, and every block link resolves to the first.
// So the property under test is not "an id is produced" but "the ids differ",
// and it has to hold on the path where the preferred source is unavailable.
import { describe, it, expect, afterEach, vi } from 'vitest'
import { ensureBlockIds, newBlockId } from '@/utils/docExtensions/blockId'

const paragraph = (text) => ({ type: 'paragraph', content: [{ type: 'text', text }] })

const stampedIds = (count) =>
  ensureBlockIds({
    type: 'doc',
    content: Array.from({ length: count }, (_, i) => paragraph(`абзац ${i}`)),
  }).content.map((n) => n.attrs.id)

describe('block ids without crypto.getRandomValues', () => {
  afterEach(() => vi.unstubAllGlobals())

  // A context with no Web Crypto at all — the case the fallback was written for.
  it('still draws distinct ids when the global is absent', () => {
    vi.stubGlobal('crypto', undefined)
    const ids = Array.from({ length: 200 }, () => newBlockId())
    expect(new Set(ids).size).toBe(200)
    expect(ids.every((id) => /^[0-9a-f]{16}$/.test(id))).toBe(true)
  })

  it('gives every block of a loaded document its own id', () => {
    vi.stubGlobal('crypto', {})
    const ids = stampedIds(5)
    expect(new Set(ids).size).toBe(5)
  })

  // A source that exists but writes nothing is the shape the old code could not
  // tell from a working one: the array keeps its zero fill and every id comes
  // out as sixteen zeroes. The stamping pass is what has to notice.
  it('re-issues around a source that leaves the bytes untouched', () => {
    vi.stubGlobal('crypto', { getRandomValues: (a) => a })
    const ids = stampedIds(5)
    expect(new Set(ids).size).toBe(5)
  })
})
