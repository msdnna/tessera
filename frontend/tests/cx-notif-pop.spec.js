import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// The bell's feed on the v2 language (#3107, PackageE «NotifPop»): filters,
// «Новые / Ранее», the sentence with the actor in bold and the content under
// it, and the feed's own keys (↑↓ ↵ E ⇧E) while it is open.

const api = vi.hoisted(() => ({
  list: vi.fn(() => Promise.resolve({ data: [] })),
  markRead: vi.fn(() => Promise.resolve()),
  markAllRead: vi.fn(() => Promise.resolve()),
}))
vi.mock('@/api', () => ({ notifications: api, apiBaseURL: () => '/api' }))
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn() }),
    // The popover renders its content inline so the feed can be queried.
    NPopover: {
      props: ['show'],
      emits: ['update:show'],
      template:
        '<div><span @click="$emit(\'update:show\', !show)"><slot name="trigger" /></span>' +
        '<div v-if="show"><slot /></div></div>',
    },
  }
})

import NotificationsBell from '@/components/NotificationsBell.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { notificationGroup, notificationAge } from '@/utils/notificationFeed'

const ago = (m) => new Date(Date.now() - m * 60000).toISOString()
const rows = () => [
  {
    id: 'm1',
    kind: 'mention',
    task_id: 't3',
    task_number: 3,
    task_board_id: 'b1',
    created_at: ago(5),
    read_at: null,
    payload: { event: 'task_mention', actor: 'Марина', task_number: 3, excerpt: 'глянь контраст' },
  },
  {
    id: 'g1',
    kind: 'integration_sync',
    created_at: ago(30),
    read_at: null,
    payload: { event: 'integration_sync_failed', label: 'GitLab', reason: 'timeout', seconds: 5 },
  },
  {
    id: 'a1',
    kind: 'assigned',
    task_number: 4,
    task_board_id: 'b1',
    created_at: ago(60 * 30),
    read_at: ago(60),
    payload: { event: 'task_assigned', actor: 'Павел', task_number: 4, title: 'Push-уведомления' },
  },
]

let wrapper = null
async function open() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }],
  })
  router.push('/')
  await router.isReady()
  const store = useNotificationsStore()
  store.items = rows()
  wrapper = mount(NotificationsBell, { attachTo: document.body, global: { plugins: [router] } })
  await wrapper.get('[data-testid="notifications-bell"]').trigger('click')
  await flushPromises()
  return { store, router }
}
const press = async (key, init = {}) => {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init }))
  await flushPromises()
}

beforeEach(() => {
  setActivePinia(createPinia())
  for (const fn of Object.values(api)) fn.mockClear()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('notification feed — groups and rows', () => {
  it('puts unread rows under «Новые» and read ones under «Ранее»', async () => {
    await open()
    const text = wrapper.text()
    expect(text).toContain('Новые')
    expect(text).toContain('Ранее')
    const order = wrapper
      .findAll('[data-testid="notification-row"]')
      .map((r) => r.attributes('data-row'))
    expect(order).toEqual(['m1', 'g1', 'a1'])
  })

  it('draws the actor in bold and the content under the line, not twice', async () => {
    await open()
    const mention = wrapper.get('[data-row="m1"]')
    expect(mention.get('b').text()).toBe('Марина')
    expect(mention.text()).toContain('«глянь контраст»')
    // The sentence itself no longer inlines the excerpt.
    expect(mention.get('.np-text').text()).not.toContain('глянь')
    const assigned = wrapper.get('[data-row="a1"]')
    expect(assigned.get('.np-chip').text()).toContain('Push-уведомления')
  })

  it('narrows to a filter and counts what is unread in it', async () => {
    await open()
    expect(wrapper.get('[data-testid="notifications-filter-mentions"]').text()).toContain('1')
    await wrapper.get('[data-testid="notifications-filter-assigned"]').trigger('click')
    const ids = wrapper
      .findAll('[data-testid="notification-row"]')
      .map((r) => r.attributes('data-row'))
    expect(ids).toEqual(['a1'])
  })
})

describe('notification feed — keys while open', () => {
  it('E marks the row under the cursor read, ⇧E reads them all', async () => {
    const { store } = await open()
    await press('ArrowDown')
    await press('ArrowDown')
    await press('e')
    expect(api.markRead).toHaveBeenCalledWith('g1')
    expect(store.items.find((n) => n.id === 'm1').read_at).toBeNull()
    await press('E', { shiftKey: true })
    expect(api.markAllRead).toHaveBeenCalled()
    expect(store.unread).toBe(0)
  })

  it('↵ opens the row under the cursor', async () => {
    const { router } = await open()
    await press('ArrowDown')
    await press('Enter')
    expect(api.markRead).toHaveBeenCalledWith('m1')
    expect(router.currentRoute.value.fullPath).toContain('task=t3')
  })

  it('does not answer keys once closed', async () => {
    await open()
    await wrapper.get('[data-testid="notifications-bell"]').trigger('click')
    await flushPromises()
    await press('E', { shiftKey: true })
    expect(api.markAllRead).not.toHaveBeenCalled()
  })
})

describe('notificationFeed helpers', () => {
  it('sorts kinds into the feed filters', () => {
    expect(notificationGroup({ kind: 'mention' })).toBe('mentions')
    expect(notificationGroup({ kind: 'assigned' })).toBe('assigned')
    expect(notificationGroup({ kind: 'integration_sync' })).toBe('gitlab')
    expect(notificationGroup({ kind: 'comment' })).toBe('other')
  })

  it('says how old a row is in the feed’s terse words', () => {
    const t = (k, p) => `${k}:${p?.n ?? ''}`
    const formatters = {
      parts: () => ({ year: 1, month: 1, day: 1 }),
      today: () => ({ year: 1, month: 1, day: 2 }),
      formatTime: () => 'HH:MM',
      formatRelative: () => 'вчера',
      formatDate: () => 'DATE',
    }
    const now = new Date('2026-10-08T12:00:00Z')
    expect(notificationAge('2026-10-08T11:59:40Z', { t, formatters, now })).toBe(
      'notifications.pop.now:',
    )
    expect(notificationAge('2026-10-08T11:55:00Z', { t, formatters, now })).toBe(
      'notifications.pop.minutes:5',
    )
    expect(notificationAge('2026-10-08T09:00:00Z', { t, formatters, now })).toBe(
      'notifications.pop.hours:3',
    )
    expect(notificationAge('2026-10-07T09:00:00Z', { t, formatters, now })).toBe('вчера')
  })
})
