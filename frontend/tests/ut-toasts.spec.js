// The toasts of v2 (#3117, PackageI): the rules (utils/toastModel) and the
// store that applies them — who gives way in the deck, what «Не беспокоить» and
// the quiet hours let through, a burst of board activity folding into one
// toast, the error toast picking up the request it is about.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import {
  ACTIVITY_DELAY,
  DURATION,
  insertTop,
  insertBottom,
  normalizePrefs,
  inQuietHours,
  passesQuiet,
  phoneOrder,
  notificationKind,
  notificationCategory,
  awayLine,
  dndPreset,
} from '@/utils/toastModel'
import { useToastsStore } from '@/stores/toasts'
import { noteFailure } from '@/utils/lastFailure'
import { i18n } from '@/i18n'

vi.mock('@/api', () => ({
  notificationPrefs: { get: vi.fn(), update: vi.fn(), dnd: vi.fn() },
}))

const t = i18n.global.t

describe('toastModel', () => {
  it('pushes the weakest, oldest toast out of a full deck', () => {
    let list = []
    for (const [id, rank] of [
      [1, 1],
      [2, 3],
      [3, 4],
    ])
      list = insertTop(list, { id, rank }).list
    const { list: next, evicted } = insertTop(list, { id: 4, rank: 3 })
    expect(next.map((x) => x.id)).toEqual([4, 3, 2])
    expect(evicted.map((x) => x.id)).toEqual([1])
    // A newcomer weaker than everything shown is the one to go.
    const r = insertTop(next, { id: 5, rank: 1 })
    expect(r.evicted.map((x) => x.id)).toEqual([5])
  })

  it('keeps errors in the bottom lane over everything else', () => {
    let list = [{ id: 1, tone: 'red' }, { id: 2 }, { id: 3 }]
    list = insertBottom(list, { id: 4 }).list
    expect(list.map((x) => x.id)).toEqual([1, 3, 4])
  })

  it('reads the stored settings with their defaults', () => {
    const p = normalizePrefs({ activity: 'all', batchMin: 99, me: { comments: false }, junk: 1 })
    expect(p.activity).toBe('all')
    expect(p.batchMin).toBe(9)
    expect(p.me.comments).toBe(false)
    expect(p.me.assign).toBe(true)
    expect(p.activityKinds.created).toBe(false)
    expect(normalizePrefs(null).activity).toBe('others')
  })

  it('evaluates the quiet window in its zone, across midnight', () => {
    const prefs = {
      quiet_enabled: true,
      quiet_toasts: true,
      quiet_start_minutes: 22 * 60,
      quiet_end_minutes: 8 * 60,
      quiet_tz: 'UTC',
    }
    expect(inQuietHours(prefs, new Date('2026-10-09T23:30:00Z'))).toBe(true)
    expect(inQuietHours(prefs, new Date('2026-10-09T07:59:00Z'))).toBe(true)
    expect(inQuietHours(prefs, new Date('2026-10-09T12:00:00Z'))).toBe(false)
    // Without «И всплывающие в приложении» the window is the channels' only.
    expect(inQuietHours({ ...prefs, quiet_toasts: false }, new Date('2026-10-09T23:30:00Z'))).toBe(
      false,
    )
  })

  it('lets mentions, reminders and calls through the quiet', () => {
    expect(passesQuiet('mention')).toBe(true)
    expect(passesQuiet('reminder')).toBe(true)
    expect(passesQuiet('call')).toBe(true)
    expect(passesQuiet('me')).toBe(false)
    expect(passesQuiet('activity')).toBe(false)
  })

  it('names the kind and the switch of a notification', () => {
    const n = (event, kind) => ({ kind, payload: { event } })
    expect(notificationKind(n('task_mention'))).toBe('mention')
    // A shared «Входящие» note naming the reader (#3040).
    expect(notificationKind(n('note_mention'))).toBe('mention')
    expect(notificationCategory(n('note_mention'))).toBe('mentions')
    expect(notificationKind(n('conference_invited'))).toBe('call')
    expect(notificationKind(n('', 'reminder'))).toBe('reminder')
    expect(notificationCategory(n('task_deleted'))).toBe('removal')
    expect(notificationCategory(n('integration_sync_ok'))).toBe('system')
  })

  it('queues the phone: errors, undo, then by rank', () => {
    const out = phoneOrder([
      { id: 'a', rank: 1, at: 3 },
      { id: 'u', lane: 'undo', at: 1 },
      { id: 'e', tone: 'red', at: 0 },
      { id: 'c', rank: 6, at: 2 },
    ])
    expect(out.map((x) => x.id)).toEqual(['e', 'u', 'c', 'a'])
  })

  it('words the digest with plurals', () => {
    expect(awayLine({ assign: 2, comments: 5, mentions: 1 }, t)).toBe(
      '2 назначения, 5 комментариев, 1 упоминание',
    )
  })

  it('sets «До завтра, 9:00» to nine tomorrow', () => {
    const d = dndPreset('tomorrow', new Date(2026, 9, 9, 15, 0))
    expect([d.getDate(), d.getHours(), d.getMinutes()]).toEqual([10, 9, 0])
  })
})

describe('toasts store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  const me = (id, over = {}) => ({
    type: 'notification',
    data: {
      user_id: 'u1',
      notification: {
        id,
        kind: 'assigned',
        task_id: `t${id}`,
        task_number: 7,
        task_title: 'Задача',
        payload: { event: 'task_assigned', actor: 'Дмитрий Орлов' },
        created_at: new Date().toISOString(),
        ...over,
      },
    },
  })

  it('shows a notification about me in the top lane, once', () => {
    const s = useToastsStore()
    s.onSocket(me('n1'), 'u1')
    s.onSocket(me('n1'), 'u1')
    s.onSocket(me('n2'), 'someone-else')
    expect(s.top).toHaveLength(1)
    expect(s.top[0].bold()).toBe('Дмитрий Орлов')
    expect(s.top[0].duration).toBe(DURATION.me)
  })

  it('silences it under «Не беспокоить», but not a mention', () => {
    const s = useToastsStore()
    s.server.dnd_until = new Date(Date.now() + 3600e3).toISOString()
    s.now = Date.now()
    s.onSocket(me('n1'), 'u1')
    expect(s.top).toHaveLength(0)
    s.onSocket(me('n2', { kind: 'mention', payload: { event: 'task_mention', actor: 'А' } }), 'u1')
    expect(s.top).toHaveLength(1)
  })

  it('counts what came while the tab was hidden into one digest', () => {
    const s = useToastsStore()
    s.setHiddenTab(true)
    s.onSocket(me('n1'), 'u1')
    s.onSocket(me('n2', { kind: 'comment', payload: { event: 'task_comment', actor: 'А' } }), 'u1')
    expect(s.top).toHaveLength(0)
    s.setHiddenTab(false)
    expect(s.top).toHaveLength(1)
    expect(s.top[0].awayCounts).toEqual({ assign: 1, comments: 1 })
  })

  it('folds a burst of one person moving tasks into one toast with a count', () => {
    const s = useToastsStore()
    const a = (taskId) => ({
      taskId,
      number: 1,
      title: 'x',
      verb: 'moved',
      column: 'Готово',
      actorId: 'p',
      actorName: 'Илья Петров',
      self: false,
    })
    for (const id of ['a', 'b', 'c', 'd']) {
      s.activity(a(id))
      vi.advanceTimersByTime(ACTIVITY_DELAY + 1)
    }
    expect(s.top).toHaveLength(1)
    expect(s.top[0].batch).toBe(true)
    expect(s.top[0].count).toBe(4)
    expect(s.top[0].tail(4)).toContain('задачи')
  })

  it('shows no activity toast for a task that just came as «про меня»', () => {
    const s = useToastsStore()
    s.onSocket(me('n1'), 'u1')
    s.activity({ taskId: 'tn1', verb: 'moved', actorId: 'p', actorName: 'П', self: false })
    vi.advanceTimersByTime(ACTIVITY_DELAY + 1)
    expect(s.top.filter((x) => x.kind === 'activity')).toHaveLength(0)
  })

  it('gives an error toast the request it is about, and waits with a retry', () => {
    const s = useToastsStore()
    noteFailure({ method: 'PATCH', url: '/tasks/1', status: 500, raw: 'boom', requestId: 'r1' })
    const retry = vi.fn()
    s.respond('error', 'Не удалось', { retry })
    const x = s.bottom[0]
    expect(x.duration).toBe(0)
    expect(x.live).toBe('assertive')
    expect(x.line).toBe(t('toasts.err.server', { status: 500 }))
    expect(x.details[0]).toBe('PATCH /tasks/1 → 500')
    expect(x.actions.map((a) => a.id)).toEqual(['retry', 'details'])
    s.run(x.id, 'retry')
    expect(retry).toHaveBeenCalled()
    expect(s.bottom).toHaveLength(0)
  })

  it('times answers out and pauses a hovered lane', () => {
    const s = useToastsStore()
    s.respond('success', 'Готово')
    s.hover.bottom = true
    s.tick(DURATION.success + 100)
    expect(s.bottom).toHaveLength(1)
    s.hover.bottom = false
    s.tick(DURATION.success + 100)
    expect(s.bottom).toHaveLength(0)
  })

  it('moves «Позже» of the update toast into the top-bar chip', () => {
    const s = useToastsStore()
    s.update()
    const x = s.top[0]
    s.run(x.id, 'later')
    expect(s.top).toHaveLength(0)
    expect(s.updateChip).toBe(true)
  })
})
