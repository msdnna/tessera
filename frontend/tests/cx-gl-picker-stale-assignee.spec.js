// #2932 — un-assigning a GitLab person who left the project.
//
// The card's assignee picker built its GitLab half from the project roster alone.
// The roster is pruned to the project's CURRENT membership on every refresh, while a
// hand-pinned assignee (source='user') survives every sync rebuild — so a person who
// left the GitLab project stayed on the task forever: their avatar was drawn on the
// card (glAssigneesOf reads gitlab_assignee_logins) and the picker had no row for
// them, i.e. no checkmark and no way to take them off.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import * as naive from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'

vi.mock('@/api', () => ({
  tasks: {
    addTag: vi.fn(),
    removeTag: vi.fn(),
    addAssignee: vi.fn(),
    removeAssignee: vi.fn(),
    pinGitlabAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    removeGitlabAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    update: vi.fn(),
    dueNotify: vi.fn(),
  },
  projects: { createTag: vi.fn() },
  users: { updatePrefs: vi.fn() },
}))

const { tasks: tasksApi } = await import('@/api')
const TaskCardPills = (await import('@/components/task/TaskCardPills.vue')).default

// Only «anna» is still a member of the GitLab project.
const ROSTER = [{ gl_user_id: 1, gl_username: 'anna', gl_name: 'Анна Петрова', gl_avatar_url: '' }]
// The card carries one more: «quit», pinned by hand before they left.
const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Синхронизированная',
  number: 7,
  gitlab_assignee_logins: ['anna', 'quit'],
}

let wrapper = null

// The picker lives in an n-popover, which teleports its body — mount into the
// document so the menu is reachable, and open it by clicking the people pill.
async function openPicker(task = TASK, roster = ROSTER) {
  seedBoardStore({ columns: [{ id: 'c1', name: 'К работе', position: 1 }], gitlabMembers: roster })
  wrapper = mount(
    { render: () => h(naive.NMessageProvider, null, { default: () => h(TaskCardPills, { task }) }) },
    { attachTo: document.body },
  )
  await nextTick()
  await wrapper.find('.assignee-pill').trigger('click')
  await nextTick()
  await nextTick()
  return document.querySelector('.assignee-menu')
}

function rows(menu) {
  return [...menu.querySelectorAll('.assignee-item')]
}
// The row's own label: its text content also carries the avatar's initials.
function nameOf(row) {
  return row.querySelector('.aname').textContent.trim()
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

afterEach(() => {
  // A mount left standing keeps its watchers on the shared stores and poisons the
  // next spec's scheduler (the pills watch the board-view store deeply).
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('assignee picker · GitLab half', () => {
  it('offers a task assignee the roster no longer holds', async () => {
    const menu = await openPicker()
    expect(menu).toBeTruthy()
    const names = rows(menu).map(nameOf)
    expect(names).toContain('Анна Петрова')
    expect(names).toContain('quit')
  })

  it('marks both of them as assigned and un-assigns on click', async () => {
    const menu = await openPicker()
    const stale = rows(menu).find((r) => nameOf(r) === 'quit')
    expect(stale.querySelector('.chk')).toBeTruthy()
    stale.click()
    await nextTick()
    expect(tasksApi.removeGitlabAssignee).toHaveBeenCalledWith('t1', 'quit')
  })

  it('leaves the ordinary case (everyone assignable is a member) unchanged', async () => {
    const menu = await openPicker({ ...TASK, gitlab_assignee_logins: ['anna'] })
    const names = rows(menu).map(nameOf)
    expect(names.filter((n) => n === 'Анна Петрова')).toHaveLength(1)
    expect(names).not.toContain('quit')
  })
})
