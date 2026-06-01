import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect } from 'vitest'
import { useActivityStore } from '@/stores/activity'

describe('activity store', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('maps a known event and increments unread', () => {
    const a = useActivityStore()
    a.push({ type: 'task.created', scope: 'w' })
    expect(a.items.length).toBe(1)
    expect(a.items[0].text).toBe('Создана задача')
    expect(a.unread).toBe(1)
  })

  it('highlights assigned-to-you vs others', () => {
    const a = useActivityStore()
    a.push({ type: 'task.assigned', data: { user_id: 'me' } }, 'me')
    expect(a.items[0].text).toBe('Вам назначили задачу')
    a.push({ type: 'task.assigned', data: { user_id: 'other' } }, 'me')
    expect(a.items[0].text).toBe('Назначен исполнитель')
  })

  it('ignores noisy/unknown events', () => {
    const a = useActivityStore()
    a.push({ type: 'task.tagged' })
    a.push({ type: 'whatever' })
    expect(a.items.length).toBe(0)
  })

  it('caps the feed at 30 and markRead resets unread', () => {
    const a = useActivityStore()
    for (let i = 0; i < 35; i++) a.push({ type: 'task.created' })
    expect(a.items.length).toBe(30)
    expect(a.unread).toBe(35)
    a.markRead()
    expect(a.unread).toBe(0)
  })
})
