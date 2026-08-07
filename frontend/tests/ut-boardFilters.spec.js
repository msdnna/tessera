import { describe, it, expect } from 'vitest'
import {
  matchesAssignee,
  matchesAuthor,
  boardGitlabAuthors,
  isGitlabValue,
  gitlabLogin,
} from '@/utils/boardFilters'

const U1 = '11111111-1111-1111-1111-111111111111'
const U2 = '22222222-2222-2222-2222-222222222222'

describe('gl value helpers', () => {
  it('recognises and unwraps gl: values', () => {
    expect(isGitlabValue('gl:john')).toBe(true)
    expect(isGitlabValue(U1)).toBe(false)
    expect(gitlabLogin('gl:john')).toBe('john')
    expect(gitlabLogin(U1)).toBe('')
  })
})

describe('matchesAssignee', () => {
  it('passes everything when nothing is selected', () => {
    expect(matchesAssignee({ assignee_ids: [] }, [])).toBe(true)
    expect(matchesAssignee({}, undefined)).toBe(true)
  })

  it('matches a Tessera assignee by uuid', () => {
    const t = { assignee_ids: [U1] }
    expect(matchesAssignee(t, [U1])).toBe(true)
    expect(matchesAssignee(t, [U2])).toBe(false)
  })

  it('matches a GitLab assignee by gl: login', () => {
    const t = { assignee_ids: [], gitlab_assignee_logins: ['john'] }
    expect(matchesAssignee(t, ['gl:john'])).toBe(true)
    expect(matchesAssignee(t, ['gl:jane'])).toBe(false)
  })

  it('ORs several selected people', () => {
    expect(matchesAssignee({ assignee_ids: [U2] }, [U1, U2])).toBe(true)
  })
})

describe('matchesAuthor', () => {
  it('passes everything when nothing is selected', () => {
    expect(matchesAuthor({ created_by: null }, [])).toBe(true)
    expect(matchesAuthor({}, undefined)).toBe(true)
  })

  it('matches a Tessera author by created_by', () => {
    const t = { created_by: U1 }
    expect(matchesAuthor(t, [U1])).toBe(true)
    expect(matchesAuthor(t, [U2])).toBe(false)
  })

  it('matches a GitLab author by gl: login', () => {
    const t = { created_by: null, gitlab_author: 'john' }
    expect(matchesAuthor(t, ['gl:john'])).toBe(true)
    expect(matchesAuthor(t, ['gl:jane'])).toBe(false)
  })

  it('matches a synced task against the linked Tessera user', () => {
    // created_by IS NULL on GitLab-synced tasks — the map bridges uuid → login.
    const t = { created_by: null, gitlab_author: 'john' }
    expect(matchesAuthor(t, [U1], { [U1]: 'john' })).toBe(true)
    expect(matchesAuthor(t, [U1], { [U1]: 'jane' })).toBe(false)
    expect(matchesAuthor(t, [U1], {})).toBe(false)
  })

  it('filters out a task with no author at all', () => {
    expect(matchesAuthor({ created_by: null, gitlab_author: null }, [U1])).toBe(false)
    expect(matchesAuthor({}, ['gl:john'])).toBe(false)
  })

  it('ORs several selected authors', () => {
    const t = { created_by: null, gitlab_author: 'john' }
    expect(matchesAuthor(t, [U1, 'gl:john'])).toBe(true)
  })
})

describe('boardGitlabAuthors', () => {
  it('collects distinct authors with names and avatars, sorted', () => {
    const out = boardGitlabAuthors([
      { gitlab_author: 'zoe', gitlab_author_name: 'Zoe', gitlab_author_avatar_url: '/z.png' },
      { gitlab_author: 'ann', gitlab_author_name: 'Ann' },
      { gitlab_author: 'zoe', gitlab_author_name: 'Zoe' }, // dup
      { created_by: U1 }, // no GitLab author
    ])
    expect(out.map((a) => a.gl_username)).toEqual(['ann', 'zoe'])
    expect(out[1]).toEqual({ gl_username: 'zoe', gl_name: 'Zoe', gl_avatar_url: '/z.png' })
    expect(out[0].gl_avatar_url).toBe('')
  })

  it('falls back to the login when no display name is synced', () => {
    expect(boardGitlabAuthors([{ gitlab_author: 'john' }])[0].gl_name).toBe('john')
  })

  it('tolerates an empty or missing task list', () => {
    expect(boardGitlabAuthors([])).toEqual([])
    expect(boardGitlabAuthors(undefined)).toEqual([])
  })
})
