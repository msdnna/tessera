import { describe, it, expect } from 'vitest'
import { metaPrefixesFromRules, buildTagGroups } from '@/utils/tagGroups'

describe('metaPrefixesFromRules', () => {
  it('collects canonical prefixes of non-tag prefix rules only', () => {
    const rules = {
      rules: [
        { match: 'S: ', match_type: 'prefix', action: 'status' },
        { match: 'P: ', match_type: 'prefix', action: 'priority' },
        { match: 'M: ', match_type: 'prefix', action: 'group' },
        { match: 'T: ', match_type: 'prefix', action: 'tag' }, // stays visible
        { match: '^bug', match_type: 'regex', action: 'status' }, // regex → skipped
      ],
    }
    const set = metaPrefixesFromRules(rules)
    expect([...set].sort()).toEqual(['m:', 'p:', 's:'])
    expect(set.has('t:')).toBe(false)
  })

  it('handles missing / empty rules', () => {
    expect(metaPrefixesFromRules(null).size).toBe(0)
    expect(metaPrefixesFromRules({}).size).toBe(0)
  })

  it('canonicalises scoped (::) prefixes', () => {
    const set = metaPrefixesFromRules({ rules: [{ match: 'P::', action: 'priority' }] })
    expect(set.has('p::')).toBe(true)
  })
})

describe('buildTagGroups with hidePrefixes', () => {
  const tags = [
    { id: '1', name: 'S: In Progress' },
    { id: '2', name: 'P: High' },
    { id: '3', name: 'T: Bug' },
    { id: '4', name: 'plain' },
  ]

  it('drops tags whose prefix is governed by a meta rule', () => {
    const hide = new Set(['s:', 'p:'])
    const groups = buildTagGroups(tags, {}, hide)
    const names = groups.flatMap((g) => g.tags.map((t) => t.name))
    expect(names).toContain('T: Bug')
    expect(names).toContain('plain')
    expect(names).not.toContain('S: In Progress')
    expect(names).not.toContain('P: High')
  })

  it('keeps everything when no hide set is given', () => {
    const names = buildTagGroups(tags).flatMap((g) => g.tags.map((t) => t.name))
    expect(names).toHaveLength(4)
  })
})
