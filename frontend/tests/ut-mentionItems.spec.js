import { describe, it, expect } from 'vitest'
import { buildMentionItems, glLoginsByUserId } from '@/utils/mentions'

const ann = { user_id: 'u1', name: 'Ann Lee', gl_username: 'a.lee' }
const bob = { user_id: 'u2', name: 'Bob Fox' } // no GitLab account at all

describe('buildMentionItems', () => {
  it('inserts the OAuth login and shows the name', () => {
    const [it0] = buildMentionItems([ann], [], {})
    expect(it0.label).toBe('a.lee') // what lands in the comment
    expect(it0.display).toBe('Ann Lee') // what the popup row says
    expect(it0.hint).toBe('@a.lee')
    expect(it0.id).toBe('u1')
  })

  it('falls back to the GitLab roster when the member is linked by PAT', () => {
    const map = { 7: { gl_user_id: 7, gl_username: 'b.fox', tessera_user_id_pat: 'u2' } }
    const [it0] = buildMentionItems([bob], [], map)
    expect(it0.label).toBe('b.fox')
  })

  it('prefers the OAuth login over the roster', () => {
    const map = { 7: { gl_user_id: 7, gl_username: 'stale.login', tessera_user_id: 'u1' } }
    expect(buildMentionItems([ann], [], map)[0].label).toBe('a.lee')
  })

  it('falls back to the name, with no hint, when no login is known', () => {
    const [it0] = buildMentionItems([bob], [], {})
    expect(it0.label).toBe('Bob Fox')
    expect(it0.hint).toBe('')
  })

  it('appends GitLab-only users without duplicating Tessera members', () => {
    const glOnly = { gl_user_id: 9, gl_username: 'k.zhmurko', gl_name: 'K Zhmurko' }
    const items = buildMentionItems([ann], [glOnly], {})
    expect(items).toHaveLength(2)
    expect(items.map((i) => i.label)).toEqual(['a.lee', 'k.zhmurko'])
    expect(items[1].id).toBeNull() // no Tessera notification for these
    expect(items[1].gitlab).toBe(true)
  })

  it('tolerates missing arguments', () => {
    expect(buildMentionItems(null, null, null)).toEqual([])
  })
})

describe('glLoginsByUserId', () => {
  it('maps both OAuth- and PAT-linked roster entries', () => {
    const map = {
      1: { gl_username: 'a.lee', tessera_user_id: 'u1' },
      2: { gl_username: 'b.fox', tessera_user_id_pat: 'u2' },
      3: { gl_username: 'nobody' }, // unlinked — not in the result
    }
    expect(glLoginsByUserId(map)).toEqual({ u1: 'a.lee', u2: 'b.fox' })
  })
})
