import { describe, it, expect } from 'vitest'
import {
  FILTER_FACETS,
  GROUP_MODES,
  NO_MILESTONE,
  emptyFilters,
  cloneFilters,
  countActiveFilters,
  encodeFacet,
  decodeFacet,
  applyFilterFacet,
  removeFilterFacet,
  encodeGroup,
  decodeGroup,
  encodeSort,
  decodeSort,
} from '@/utils/facetKeys'

// The whole point of this module is that the menu side and the parse side can't
// drift apart, so the round-trips below are the load-bearing tests: they fail if
// anyone changes a prefix on one end only.
describe('facetKeys — round trip', () => {
  it('every menu-visible filter facet survives encode → decode', () => {
    for (const f of FILTER_FACETS.filter((x) => x.prefix)) {
      const raw = f.parse === Number ? 3 : 'some-value'
      const decoded = decodeFacet(encodeFacet(f.kind, raw))
      expect(decoded, `facet ${f.kind}`).toBeTruthy()
      expect(decoded.kind).toBe(f.kind)
      expect(decoded.field).toBe(f.field)
      expect(decoded.value).toBe(raw)
    }
  })

  it('every grouping mode survives encode → decode', () => {
    for (const mode of GROUP_MODES) {
      expect(decodeGroup(encodeGroup(mode))).toEqual({ mode, prefix: '' })
    }
  })

  it('keeps a tag prefix intact, including characters that need escaping', () => {
    for (const prefix of ['area', 'команда', 'a.b', 'a/b', 'a b']) {
      expect(decodeGroup(encodeGroup('tag', prefix))).toEqual({ mode: 'tag', prefix })
    }
  })

  it('round-trips sort fields', () => {
    expect(decodeSort(encodeSort('due'))).toBe('due')
    expect(decodeSort(encodeSort('milestone'))).toBe('milestone')
  })

  it('values containing a dot are not truncated (only the first dot splits)', () => {
    expect(decodeFacet(encodeFacet('assignee', 'gl:some.user')).value).toBe('gl:some.user')
  })
})

describe('facetKeys — rejecting what is not a facet', () => {
  // These keys all travel through the same n-dropdown select handler, so decoding
  // has to leave them alone rather than half-matching a prefix.
  it.each([
    ['nav.back', 'mobile drill'],
    ['nav.group', 'mobile drill'],
    ['fag', 'Naive group header (assignee)'],
    ['fcg', 'Naive group header (author)'],
    ['ftg.area', 'Naive group header (tags)'],
    ['s.due', 'sort key'],
    ['g.status', 'group key'],
    ['', 'empty'],
    ['nodot', 'no separator'],
  ])('decodeFacet(%s) is null — %s', (key) => {
    expect(decodeFacet(key)).toBeNull()
  })

  it('does not confuse the status filter (fs.) with a sort key (s.)', () => {
    expect(decodeSort('fs.col-1')).toBeNull()
    expect(decodeFacet('fs.col-1')).toMatchObject({ kind: 'status', value: 'col-1' })
  })

  it('rejects an unknown grouping mode instead of assigning it', () => {
    expect(decodeGroup('g.bogus')).toBeNull()
    expect(decodeGroup('fp.1')).toBeNull()
  })

  it('rejects a non-numeric priority', () => {
    expect(decodeFacet('fp.high')).toBeNull()
    expect(decodeFacet('fp.2')).toMatchObject({ kind: 'priority', value: 2 })
  })

  it('throws when the menu asks for a kind that has no key', () => {
    expect(() => encodeFacet('group', 'status')).toThrow(/facet kind/)
    expect(() => encodeFacet('search', 'x')).toThrow(/facet kind/)
  })
})

describe('facetKeys — filter set', () => {
  it('a blank set carries every declared field', () => {
    const f = emptyFilters()
    for (const facet of FILTER_FACETS) {
      expect(f, `field ${facet.field}`).toHaveProperty(facet.field)
      expect(Array.isArray(f[facet.field])).toBe(facet.multi)
    }
  })

  it('clone drops unknown keys and never shares arrays', () => {
    const src = { tags: ['t1'], bogus: 'x', due: 'today' }
    const out = cloneFilters(src)
    expect(out.bogus).toBeUndefined()
    expect(out.due).toBe('today')
    out.tags.push('t2')
    expect(src.tags).toEqual(['t1'])
  })

  it('clone repairs a non-array where an array is expected', () => {
    // A saved view written by an older build (or a hand-edited localStorage blob)
    // must not put a string where the board iterates an array.
    expect(cloneFilters({ tags: 'oops' }).tags).toEqual([])
    expect(cloneFilters(null)).toEqual(emptyFilters())
  })

  it('counts every facet, and only counts a blank search once it has content', () => {
    const f = emptyFilters()
    expect(countActiveFilters(f)).toBe(0)
    f.tags.push('t1', 't2')
    f.priorities.push(1)
    f.due = 'today'
    expect(countActiveFilters(f)).toBe(4)
    f.q = '   '
    expect(countActiveFilters(f)).toBe(4)
    f.q = 'bug'
    expect(countActiveFilters(f)).toBe(5)
  })
})

describe('facetKeys — apply and remove', () => {
  it('picking the same value twice does not add a second chip', () => {
    const f = emptyFilters()
    expect(applyFilterFacet(f, decodeFacet('ft.tag-1'))).toBe(true)
    expect(applyFilterFacet(f, decodeFacet('ft.tag-1'))).toBe(false)
    expect(f.tags).toEqual(['tag-1'])
  })

  it('a single-valued facet replaces rather than accumulates', () => {
    const f = emptyFilters()
    applyFilterFacet(f, decodeFacet('fd.today'))
    applyFilterFacet(f, decodeFacet('fd.overdue'))
    expect(f.due).toBe('overdue')
  })

  it('applying a null decode is a no-op', () => {
    const f = emptyFilters()
    expect(applyFilterFacet(f, decodeFacet('nav.back'))).toBe(false)
    expect(f).toEqual(emptyFilters())
  })

  it('removes one value and clears a single-valued facet', () => {
    const f = emptyFilters()
    f.milestones.push('m1', NO_MILESTONE)
    f.due = 'week'
    removeFilterFacet(f, 'milestone', 'm1')
    expect(f.milestones).toEqual([NO_MILESTONE])
    removeFilterFacet(f, 'due')
    expect(f.due).toBe('')
  })

  it('ignores a chip kind that owns no filter field (the grouping chip)', () => {
    const f = emptyFilters()
    expect(removeFilterFacet(f, 'group', 'status')).toBe(false)
    expect(f).toEqual(emptyFilters())
  })
})
