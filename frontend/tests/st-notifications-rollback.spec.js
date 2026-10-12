// Отметка «прочитано» — оптимистичная правка, и до #2932 у неё не было откатa.
//
// markRead/markAllRead ставят read_at локально ДО запроса (так колокольчик
// отвечает на клик мгновенно) и глотали отказ пустым `catch`. Счётчик уезжал в
// нуль, сервер оставался при своём, и интерфейс врал про его состояние до
// следующего load() — после которого все «прочитанные» возвращались непрочитанными
// без единого слова. Доска этот класс закрыла давно (onColChange, onQuadChange,
// moveSidebar*: тост + пересинхронизация), стор был последним местом без отката.
//
// Тост берётся параметром, как в useSidebarDnd: useMessage() живёт только в
// setup-контексте, а стор создаётся откуда угодно.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'

const markRead = vi.fn(() => Promise.resolve())
const markAllRead = vi.fn(() => Promise.resolve())
vi.mock('@/api', () => ({
  notifications: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    unreadCount: vi.fn(() => Promise.resolve({ data: { count: 0 } })),
    markRead: (...a) => markRead(...a),
    markAllRead: (...a) => markAllRead(...a),
  },
}))

import { useNotificationsStore } from '@/stores/notifications'

const notif = (over = {}) => ({ id: 'n1', text: 'x', read_at: null, ...over })
const bus = () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() })
const REFUSED = new Error('forbidden')

describe('стор уведомлений: откат оптимистичной отметки', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    markRead.mockResolvedValue()
    markAllRead.mockResolvedValue()
  })

  it('возвращает строку в непрочитанные, когда отметка одной отклонена', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a' })]
    markRead.mockRejectedValue(REFUSED)
    const m = bus()
    await s.markRead('a', m)
    expect(s.items[0].read_at).toBe(null)
    expect(s.unread).toBe(1)
    expect(m.error).toHaveBeenCalledWith('forbidden')
  })

  it('оставляет отметку одной строки, когда сервер её принял', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a' })]
    const m = bus()
    await s.markRead('a', m)
    expect(s.items[0].read_at).toBeTruthy()
    expect(s.unread).toBe(0)
    expect(m.error).not.toHaveBeenCalled()
  })

  it('возвращает все строки в непрочитанные, когда «отметить все» отклонено', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a' }), notif({ id: 'b' }), notif({ id: 'c' })]
    markAllRead.mockRejectedValue(REFUSED)
    const m = bus()
    await s.markAllRead(m)
    expect(s.items.map((n) => n.read_at)).toEqual([null, null, null])
    expect(s.unread).toBe(3)
    expect(m.error).toHaveBeenCalledWith('forbidden')
  })

  // Откат возвращает ровно то, что переписал: строку, прочитанную раньше (на
  // другом устройстве или по клику), он не имеет права поднять обратно наверх
  // непрочитанной.
  it('не трогает откатом то, что было прочитано до вызова', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a', read_at: '2026-09-01T10:00:00Z' }), notif({ id: 'b' })]
    markAllRead.mockRejectedValue(REFUSED)
    await s.markAllRead(bus())
    expect(s.items[0].read_at).toBe('2026-09-01T10:00:00Z')
    expect(s.items[1].read_at).toBe(null)
    expect(s.unread).toBe(1)
  })

  // Уведомление, приехавшее по сокету, пока запрос в полёте: оптимистичная
  // правка его не видела, значит и откат его не касается — он остаётся
  // непрочитанным, а не «прочитанным заодно».
  it('не отмечает прочитанным то, что приехало в полёте', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a' })]
    let refuse
    markAllRead.mockReturnValue(new Promise((_, rej) => (refuse = rej)))
    const done = s.markAllRead(bus())
    expect(s.items[0].read_at).toBeTruthy()
    s.onEvent(
      { type: 'notification', data: { user_id: 'me', notification: notif({ id: 'z' }) } },
      'me',
    )
    refuse(REFUSED)
    await done
    expect(s.items.find((n) => n.id === 'z').read_at).toBe(null)
    expect(s.unread).toBe(2)
  })

  // Лента могла перечитаться (load) пока запрос летел: откат ищет строку по id и
  // молчит, если её больше нет, вместо того чтобы воскрешать её в списке.
  it('не воскрешает строку, исчезнувшую из ленты в полёте', async () => {
    const s = useNotificationsStore()
    s.items = [notif({ id: 'a' })]
    let refuse
    markRead.mockReturnValue(new Promise((_, rej) => (refuse = rej)))
    const done = s.markRead('a', bus())
    s.items = []
    refuse(REFUSED)
    await done
    expect(s.items).toEqual([])
  })
})
