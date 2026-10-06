import { describe, it, expect, vi } from 'vitest'
import { ref, reactive } from 'vue'

import { boardGitlabAssignees, matchesAssignee } from '@/utils/boardFilters'
import { useBoardFacets } from '@/composables/useBoardFacets'

// The «Исполнитель» facet used to offer only the GitLab member ROSTER, which the
// backend refreshes at most once an hour (memberRosterTTL) and best-effort at that,
// while issues arrive on every pull. A person the cards already draw from
// `gitlab_assignee_logins` therefore had no entry to select them with, even though
// `matchesAssignee` has always matched any `gl:<login>`. The author facet had the
// same hole plugged long ago (boardGitlabAuthors); this is its mirror.

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
  return { api: useBoardFacets(ctx), ...ctx }
}
// The GitLab bucket of the assignee menu, flattened to its facet keys.
const glKeys = (api) => {
  const children = api.addOptions.value.find((o) => o.key === 'fa').children
  const group = children.find((c) => c.type === 'group' && c.key === 'fag')
  return (group?.children || []).map((c) => c.key)
}

describe('boardGitlabAssignees', () => {
  it('collects distinct logins across the board, sorted', () => {
    expect(
      boardGitlabAssignees([
        { gitlab_assignee_logins: ['ivan', 'anya'] },
        { gitlab_assignee_logins: ['ivan'] },
        { gitlab_assignee_logins: [] },
        {},
      ]),
    ).toEqual([
      { gl_username: 'anya', gl_name: 'anya', gl_avatar_url: '' },
      { gl_username: 'ivan', gl_name: 'ivan', gl_avatar_url: '' },
    ])
  })

  it('drops empty logins and survives a missing list', () => {
    expect(boardGitlabAssignees([{ gitlab_assignee_logins: ['', null] }, null])).toEqual([])
    expect(boardGitlabAssignees(undefined)).toEqual([])
  })

  it('ignores the legacy name-only field, which no filter value could match', () => {
    // `gitlab_assignees` carries display names, not logins — an entry built from one
    // would select nothing, so it must not become a menu option.
    expect(boardGitlabAssignees([{ gitlab_assignees: ['Иван П.'] }])).toEqual([])
  })
})

describe('assignee facet — a login the roster does not have', () => {
  it('offers the board-seen login, and it selects the card', () => {
    const { api, allTasks } = facets()
    const task = { id: 't1', gitlab_assignee_logins: ['newcomer'] }
    allTasks.value = [task]
    // The roster is empty (never synced / stale), but the card shows this person.
    expect(glKeys(api)).toEqual(['fa.gl:newcomer'])
    // And the value the menu emits is one the predicate actually matches.
    expect(matchesAssignee(task, ['gl:newcomer'])).toBe(true)
  })

  it('does not duplicate a login the roster already carries', () => {
    const { api, allTasks, gitlabMembersList } = facets()
    gitlabMembersList.value = [{ gl_username: 'ivan', gl_name: 'Иван П.' }]
    allTasks.value = [{ gitlab_assignee_logins: ['ivan', 'newcomer'] }]
    expect(glKeys(api)).toEqual(['fa.gl:ivan', 'fa.gl:newcomer'])
    // The roster row keeps its display name; the board-seen one has only a login.
    const gl = api.addOptions.value
      .find((o) => o.key === 'fa')
      .children.find((c) => c.key === 'fag')
    expect(gl.children.map((c) => c.label)).toEqual(['Иван П.', 'newcomer'])
  })

  it('skips a linked workspace member — their Tessera entry already matches', () => {
    // A hand-pinned assignee (source='user') survives every sync, so a login can
    // sit on a card long after that person linked a Tessera account. The roster
    // drops such a row to avoid listing one person twice, and matchesAssignee
    // resolves their uuid through glLoginByUserId — adding a `gl:` entry here would
    // put them back on the menu twice.
    const { api, allTasks, membersList, glLoginByUserId } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    glLoginByUserId.value = { u1: 'anya' }
    const task = { id: 't1', gitlab_assignee_logins: ['anya'] }
    allTasks.value = [task]
    expect(glKeys(api)).toEqual([])
    expect(api.addOptions.value.find((o) => o.key === 'fa').children.map((c) => c.key)).toEqual([
      'fa.u1',
    ])
    expect(matchesAssignee(task, ['u1'], glLoginByUserId.value)).toBe(true)
  })

  it('keeps a roster member linked to a non-member of this workspace', () => {
    // glLoginByUserId is built from the whole roster, not from the workspace's
    // members, so pre-seeding the skip set from it wholesale would silently drop a
    // GitLab person who has no Tessera entry on this board to fall back to.
    const { api, gitlabMembersList, glLoginByUserId } = facets()
    gitlabMembersList.value = [{ gl_username: 'ivan', gl_name: 'Иван П.' }]
    glLoginByUserId.value = { uOther: 'ivan' }
    expect(glKeys(api)).toEqual(['fa.gl:ivan'])
  })

  it('leaves the menu flat when the board has no GitLab people at all', () => {
    const { api, membersList, allTasks } = facets()
    membersList.value = [{ user_id: 'u1', name: 'Аня' }]
    allTasks.value = [{ id: 't1', assignee_ids: ['u1'] }]
    expect(api.addOptions.value.find((o) => o.key === 'fa').children.map((c) => c.key)).toEqual([
      'fa.u1',
    ])
  })
})
