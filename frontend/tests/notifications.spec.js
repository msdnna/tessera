import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'

// The store imports the api module; stub it so no network is touched.
vi.mock('@/api', () => ({
  notifications: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    unreadCount: vi.fn(() => Promise.resolve({ data: { count: 0 } })),
    markRead: vi.fn(() => Promise.resolve()),
    markAllRead: vi.fn(() => Promise.resolve()),
  },
}))

import { useNotificationsStore } from '@/stores/notifications'

const notif = (over = {}) => ({
  id: over.id || crypto.randomUUID(),
  text: 'x',
  read_at: null,
  ...over,
})

describe('notifications store', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('only adds notification events addressed to me', () => {
    const s = useNotificationsStore()
    s.onEvent(
      { type: 'notification', data: { user_id: 'me', notification: notif({ id: '1' }) } },
      'me',
    )
    s.onEvent(
      { type: 'notification', data: { user_id: 'other', notification: notif({ id: '2' }) } },
      'me',
    )
    s.onEvent({ type: 'task.created', data: {} }, 'me')
    expect(s.items.length).toBe(1)
    expect(s.unread).toBe(1)
  })

  it('dedupes by id and caps at 50', () => {
    const s = useNotificationsStore()
    const dup = notif({ id: 'dup' })
    s.onEvent({ type: 'notification', data: { user_id: 'me', notification: dup } }, 'me')
    s.onEvent({ type: 'notification', data: { user_id: 'me', notification: dup } }, 'me')
    expect(s.items.length).toBe(1)
    for (let i = 0; i < 60; i++) {
      s.onEvent({ type: 'notification', data: { user_id: 'me', notification: notif() } }, 'me')
    }
    expect(s.items.length).toBe(50)
  })

  it('markAllRead clears unread', async () => {
    const s = useNotificationsStore()
    s.onEvent({ type: 'notification', data: { user_id: 'me', notification: notif() } }, 'me')
    expect(s.unread).toBe(1)
    await s.markAllRead()
    expect(s.unread).toBe(0)
  })
})
