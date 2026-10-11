import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { normalizePrefs, soundFor } from '@/utils/toastModel'

// «Звук» всплывающих уведомлений (#3121).

const chime = vi.hoisted(() => vi.fn(() => true))
vi.mock('@/utils/chime', () => ({ playChime: chime, resetChime: vi.fn() }))
vi.mock('@/api', () => ({
  notificationPrefs: { get: vi.fn(), update: vi.fn(), setDnd: vi.fn() },
}))
const { useToastsStore } = await import('@/stores/toasts')

const note = (id, kind, event) => ({
  type: 'notification',
  data: {
    user_id: 'u1',
    notification: {
      id,
      kind,
      task_id: `t${id}`,
      task_number: 7,
      task_title: 'Задача',
      payload: { event, actor: 'Дмитрий' },
      created_at: new Date().toISOString(),
    },
  },
})

describe('toast sound', () => {
  it('modes: off rings nothing, important — only what cannot wait, all — any', () => {
    expect(soundFor('mention', 'off')).toBe(false)
    expect(soundFor('mention', 'important')).toBe(true)
    expect(soundFor('call', 'important')).toBe(true)
    expect(soundFor('activity', 'important')).toBe(false)
    expect(soundFor('activity', 'all')).toBe(true)
  })

  it('is off by default and survives a stray value', () => {
    expect(normalizePrefs({}).sound).toBe('off')
    expect(normalizePrefs({ sound: 'loud' }).sound).toBe('off')
    expect(normalizePrefs({ sound: 'all' }).sound).toBe('all')
  })

  beforeEach(() => {
    setActivePinia(createPinia())
    chime.mockClear()
  })

  it('a mention rings under «Важное», silent under «Не беспокоить»', () => {
    const s = useToastsStore()
    s.prefs = { ...s.prefs, sound: 'important' }
    s.onSocket(note('n1', 'mention', 'task_mention'), 'u1')
    expect(s.top).toHaveLength(1)
    expect(chime).toHaveBeenCalledTimes(1)

    s.server.dnd_until = new Date(Date.now() + 3600e3).toISOString()
    s.now = Date.now()
    s.onSocket(note('n2', 'mention', 'task_mention'), 'u1')
    expect(s.top).toHaveLength(2) // a mention still shows…
    expect(chime).toHaveBeenCalledTimes(1) // …but does not ring
  })

  it('stays silent when the mode is off', () => {
    const s = useToastsStore()
    s.onSocket(note('n1', 'mention', 'task_mention'), 'u1')
    expect(s.top).toHaveLength(1)
    expect(chime).not.toHaveBeenCalled()
  })
})
