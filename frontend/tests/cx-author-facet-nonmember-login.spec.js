// The «Автор» facet and the person who is on the board but not in this workspace.
//
// GitLab-synced tasks carry their author as a bare login (`gitlab_author`, with
// `created_by IS NULL`), so the facet offers two kinds of value: a Tessera user id
// for a workspace member, and `gl:<login>` for everyone else. A login that a linked
// Tessera row already covers is deliberately left out of the GitLab bucket, or one
// person would be listed twice — their own entry matches the login through
// `glLoginByUserId` (see matchesAuthor).
//
// The skip set for that rule was the WHOLE of `glLoginByUserId` (#2932), and that map
// is the GitLab *project* roster keyed by Tessera user id: the backend resolves those
// ids by joining oauth_identities / gitlab_credentials on the login alone
// (ListGitlabProjectMembersByWorkspace), with no membership check. So it also covers
// people with a Tessera account who are not members HERE — someone who linked GitLab
// in another workspace, or who has since left this one. Those had no Tessera entry in
// the menu (they are not in the member list) and no `gl:` entry either, so their
// issues answered to no value of the facet at all, exactly the hole that was closed
// one facet over for «Исполнитель».
//
// Pinned here over `useBoardFacets` — the menu builder — plus the pure rule both
// person facets now share.
import { describe, it, expect, vi } from 'vitest'
import { ref, reactive } from 'vue'

import { useBoardFacets } from '@/composables/useBoardFacets'
import { linkedMemberLogins, matchesAuthor } from '@/utils/boardFilters'

function facets(over = {}) {
  const ctx = {
    groupMode: ref('status'),
    tagPrefix: ref(''),
    timelineLike: ref(false),
    columns: ref([{ id: 'c1', name: 'К работе', position: 1 }]),
    allTasks: ref([]),
    tagsList: ref([]),
    membersList: ref([]),
    milestonesList: ref([]),
    gitlabMembersList: ref([]),
    tagsMap: reactive({}),
    membersMap: reactive({}),
    milestonesMap: reactive({}),
    tagPrefixNames: reactive({}),
    glLoginByUserId: ref({}),
    milestoneScope: ref(''),
    clearMilestoneScope: vi.fn(),
    ...over,
  }
  const api = useBoardFacets(ctx)
  return { api, filters: api.filters, ...ctx }
}

// Flatten one facet menu to its keys, GitLab group included.
const menuKeys = (api, key) => {
  const children = api.addOptions.value.find((o) => o.key === key).children
  return children.flatMap((c) => (c.children ? c.children.map((x) => x.key) : [c.key]))
}

// A GitLab-synced issue: no Tessera author, the login in `gitlab_author`.
const glTask = (login) => ({ id: `t-${login}`, created_by: null, gitlab_author: login })

describe('author facet — a GitLab person outside this workspace', () => {
  it('offers the roster login of an account that is not a member here', () => {
    // `u9` has a Tessera account (so the roster resolved tessera_user_id for
    // `petya`) but no membership in this workspace: no Tessera entry to fall back
    // to, so the `gl:` entry is the only way to select them.
    const { api, membersList, gitlabMembersList, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    gitlabMembersList.value = [{ gl_username: 'petya', gl_name: 'Петя' }]
    glLoginByUserId.value = { u9: 'petya' }

    expect(menuKeys(api, 'fc')).toEqual(['fc.u1', 'fc.gl:petya'])
  })

  it('offers a board-only author whose account is not a member here', () => {
    // The same person, absent from the roster too (it is an hourly cache) and known
    // only from the cards — boardGitlabAuthors' half of the menu.
    const { api, membersList, allTasks, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    allTasks.value = [glTask('petya')]
    glLoginByUserId.value = { u9: 'petya' }

    expect(menuKeys(api, 'fc')).toEqual(['fc.u1', 'fc.gl:petya'])
  })

  it('still lists a linked member once, not twice', () => {
    // The control the rule exists for: `anya` IS a member here, so her own entry
    // covers the login and the GitLab bucket must stay empty.
    const { api, membersList, gitlabMembersList, allTasks, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    gitlabMembersList.value = [{ gl_username: 'anya', gl_name: 'Аня' }]
    allTasks.value = [glTask('anya')]
    glLoginByUserId.value = { u1: 'anya' }

    expect(menuKeys(api, 'fc')).toEqual(['fc.u1'])
  })

  it('does not repeat a login that the roster and the board both carry', () => {
    const { api, membersList, gitlabMembersList, allTasks, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    gitlabMembersList.value = [{ gl_username: 'petya', gl_name: 'Петя' }]
    allTasks.value = [glTask('petya'), glTask('petya')]
    glLoginByUserId.value = { u9: 'petya' }

    expect(menuKeys(api, 'fc')).toEqual(['fc.u1', 'fc.gl:petya'])
  })

  it('the value the menu now offers is the one that matches the card', () => {
    // The other half of the facet: what is selectable has to be what filters. The
    // predicate was never the broken part — the entry was missing.
    const task = glTask('petya')
    expect(matchesAuthor(task, ['gl:petya'])).toBe(true)
    // ...and a non-member's user id is not an alias for it: the map is the roster,
    // not a membership list, so matching through it would draw the card for a
    // person this board cannot even name.
    expect(matchesAuthor(task, ['u1'], { u9: 'petya' })).toBe(false)
  })

  it('the assignee facet keeps following the same rule', () => {
    // Both person facets share `linkedMemberLogins` now; the assignee menu was the
    // one that had it right, so it must not have moved.
    const { api, membersList, gitlabMembersList, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    gitlabMembersList.value = [
      { gl_username: 'anya', gl_name: 'Аня' },
      { gl_username: 'petya', gl_name: 'Петя' },
    ]
    glLoginByUserId.value = { u1: 'anya', u9: 'petya' }

    expect(menuKeys(api, 'fa')).toEqual(['fa.u1', 'fa.gl:petya'])
  })
})

describe('linkedMemberLogins', () => {
  it('collects only the logins of members it was given', () => {
    const map = { u1: 'anya', u9: 'petya' }
    expect([...linkedMemberLogins([{ user_id: 'u1' }], map)]).toEqual(['anya'])
    expect([...linkedMemberLogins([{ user_id: 'u1' }, { user_id: 'u9' }], map)]).toEqual([
      'anya',
      'petya',
    ])
  })

  it('shrugs off a member with no link, and missing arguments', () => {
    expect([...linkedMemberLogins([{ user_id: 'u2' }], { u1: 'anya' })]).toEqual([])
    expect([...linkedMemberLogins(undefined, undefined)]).toEqual([])
    expect([...linkedMemberLogins([null, {}], { u1: 'anya' })]).toEqual([])
  })
})
