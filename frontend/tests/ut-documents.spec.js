import { describe, it, expect } from 'vitest'
import { buildDocTree, docTreeOptions } from '@/utils/documents'

const doc = (id, parent_id = null, title = id) => ({ id, parent_id, title })

describe('buildDocTree', () => {
  it('returns an empty array for empty or non-array input', () => {
    expect(buildDocTree([])).toEqual([])
    expect(buildDocTree(null)).toEqual([])
    expect(buildDocTree(undefined)).toEqual([])
  })

  it('keeps flat documents at the root', () => {
    const tree = buildDocTree([doc('a'), doc('b')])
    expect(tree.map((n) => n.id)).toEqual(['a', 'b'])
    expect(tree.every((n) => n.children.length === 0)).toBe(true)
  })

  it('nests children under their parent', () => {
    const tree = buildDocTree([doc('root'), doc('child', 'root'), doc('leaf', 'child')])
    expect(tree).toHaveLength(1)
    expect(tree[0].id).toBe('root')
    expect(tree[0].children[0].id).toBe('child')
    expect(tree[0].children[0].children[0].id).toBe('leaf')
  })

  it('preserves list order among siblings', () => {
    const tree = buildDocTree([doc('root'), doc('b', 'root'), doc('a', 'root')])
    expect(tree[0].children.map((n) => n.id)).toEqual(['b', 'a'])
  })

  it('surfaces orphans at the root instead of dropping them', () => {
    // The parent was filtered out (e.g. a project-scoped list) — the child must
    // still be reachable, otherwise a document openable by link is invisible.
    const tree = buildDocTree([doc('orphan', 'missing-parent')])
    expect(tree.map((n) => n.id)).toEqual(['orphan'])
  })

  it('does not lose a subtree to a cycle in stale data', () => {
    // The API rejects cycles, but a client holding a stale list could still see
    // one; every node must stay reachable from some root.
    const tree = buildDocTree([doc('a', 'b'), doc('b', 'a')])
    const ids = []
    const walk = (nodes) => nodes.forEach((n) => (ids.push(n.id), walk(n.children)))
    walk(tree)
    expect(ids.sort()).toEqual(['a', 'b'])
  })

  it('does not mutate the input documents', () => {
    const input = [doc('root'), doc('child', 'root')]
    buildDocTree(input)
    expect(input[0].children).toBeUndefined()
  })
})

describe('docTreeOptions', () => {
  it('maps a tree to n-tree options and omits empty children', () => {
    const opts = docTreeOptions(buildDocTree([doc('root'), doc('child', 'root')]))
    expect(opts[0].key).toBe('root')
    expect(opts[0].children[0].key).toBe('child')
    expect(opts[0].children[0].children).toBeUndefined()
  })

  it('falls back to a placeholder label for an untitled document', () => {
    const opts = docTreeOptions(buildDocTree([{ id: 'a', parent_id: null, title: '' }]))
    expect(opts[0].label).toBe('Без названия')
  })

  it('tolerates empty input', () => {
    expect(docTreeOptions([])).toEqual([])
    expect(docTreeOptions(undefined)).toEqual([])
  })
})
