// Ответ ленты уведомлений, пришедший не по порядку (#2932).
//
// load() всегда ПРИСВАИВАЛ: `items.value = res.data`. Одного заказчика у этого
// запроса нет — лента читается на монтировании и заново на КАЖДОМ resync'е
// (utils/shellResync.js), то есть ровно тогда, когда связь рвётся и встаёт
// обратно. Два таких запроса в полёте — и на экране остаётся тот, что ответил
// последним, а не тот, что спросили последним.
//
// Громче всего это звучит на самом сокете: resync поднял загрузку, по только что
// восстановленному сокету прилетело новое уведомление (onEvent положил его в
// ленту, счётчик стал 1), а следом пришёл ответ со снимком, снятым ДО него, —
// и упоминание пропадает вместе с бейджем. Ради этих же пропущенных уведомлений
// resync и заводили.
//
// Отметка прочитанным — та же гонка с другой стороны: клик по строке ставит
// read_at локально, а ответ загрузки, снятый до запроса, возвращает строку в
// непрочитанные. Прочитанность монотонна (сделать строку непрочитанной нечем),
// поэтому локальная отметка ответ переживает.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'

const list = vi.fn(() => Promise.resolve({ data: [] }))
const markRead = vi.fn(() => Promise.resolve())
const markAllRead = vi.fn(() => Promise.resolve())
vi.mock('@/api', () => ({
  notifications: {
    list: (...a) => list(...a),
    unreadCount: vi.fn(() => Promise.resolve({ data: { count: 0 } })),
    markRead: (...a) => markRead(...a),
    markAllRead: (...a) => markAllRead(...a),
  },
}))

import { useNotificationsStore } from '@/stores/notifications'

const notif = (id, over = {}) => ({ id, text: id, read_at: null, ...over })
const bus = () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() })
// Событие сокета в том виде, в каком его разбирает onEvent.
const event = (n, userId = 'me') => ({
  type: 'notification',
  data: { user_id: userId, notification: n },
})
// Отложенный ответ, которым в тесте управляют вручную.
function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

describe('лента уведомлений: устаревший ответ', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    list.mockResolvedValue({ data: [] })
    markRead.mockResolvedValue()
    markAllRead.mockResolvedValue()
  })

  it('не отдаёт ленту более раннему ответу, когда загрузок две', async () => {
    const s = useNotificationsStore()
    const slow = deferred()
    const fast = deferred()
    list.mockReturnValueOnce(slow.promise).mockReturnValueOnce(fast.promise)

    const first = s.load()
    const second = s.load()
    fast.resolve({ data: [notif('new')] })
    await second
    slow.resolve({ data: [notif('old')] })
    await first

    expect(s.items.map((n) => n.id)).toEqual(['new'])
  })

  it('сохраняет уведомление, пришедшее по сокету, пока загрузка была в полёте', async () => {
    const s = useNotificationsStore()
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    const loading = s.load()
    s.onEvent(event(notif('live')), 'me')
    answer.resolve({ data: [notif('snapshot')] })
    await loading

    expect(s.items.map((n) => n.id)).toEqual(['live', 'snapshot'])
    expect(s.unread).toBe(2)
  })

  it('не задваивает уведомление, которое ответ уже принёс', async () => {
    const s = useNotificationsStore()
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    const loading = s.load()
    s.onEvent(event(notif('live')), 'me')
    answer.resolve({ data: [notif('live'), notif('older')] })
    await loading

    expect(s.items.map((n) => n.id)).toEqual(['live', 'older'])
  })

  it('не подмешивает живые строки в ответ следующей загрузки', async () => {
    const s = useNotificationsStore()
    const first = deferred()
    list.mockReturnValueOnce(first.promise)

    const loading = s.load()
    s.onEvent(event(notif('live')), 'me')
    first.resolve({ data: [] })
    await loading
    expect(s.items.map((n) => n.id)).toEqual(['live'])

    // Сервер к этому моменту знает про строку и сам решает, показывать ли её:
    // выпала из ленты — значит выпала, тащить её из прошлого полёта нечем.
    list.mockResolvedValueOnce({ data: [notif('other')] })
    await s.load()
    expect(s.items.map((n) => n.id)).toEqual(['other'])
  })

  it('оставляет строку прочитанной, когда ответ загрузки снят до отметки', async () => {
    const s = useNotificationsStore()
    s.items = [notif('a')]
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    const loading = s.load()
    const marking = s.markRead('a', bus())
    answer.resolve({ data: [notif('a')] })
    await loading
    await marking

    expect(s.items[0].read_at).toBeTruthy()
    expect(s.unread).toBe(0)
  })

  it('возвращает строку в непрочитанные, когда отказ пришёл после ответа загрузки', async () => {
    const s = useNotificationsStore()
    s.items = [notif('a')]
    markRead.mockRejectedValue(new Error('forbidden'))
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    const loading = s.load()
    const m = bus()
    const marking = s.markRead('a', m)
    answer.resolve({ data: [notif('a')] })
    await loading
    await marking

    expect(s.items[0].read_at).toBe(null)
    expect(s.unread).toBe(1)
    expect(m.error).toHaveBeenCalled()
  })

  it('не поднимает флаг loaded на устаревшем ответе', async () => {
    const s = useNotificationsStore()
    const slow = deferred()
    list.mockReturnValueOnce(slow.promise).mockRejectedValueOnce(new Error('offline'))

    const first = s.load()
    await s.load()
    expect(s.loaded).toBe(false)
    slow.resolve({ data: [notif('old')] })
    await first

    // Последней спросили ту загрузку, что упала, — лента остаётся при своём,
    // и «лента прочитана» тоже не объявляется.
    expect(s.items).toEqual([])
    expect(s.loaded).toBe(false)
  })
})
