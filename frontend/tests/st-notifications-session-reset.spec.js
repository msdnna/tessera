// Лента уведомлений переживала выход из аккаунта (#2932).
//
// Колокольчик — штука персональная, а выход из аккаунта в SPA это смена
// маршрута, а не перезагрузка страницы (перезагружается приложение только при
// смене сервера, stores/servers.js). Pinia-стор при этом остаётся в памяти как
// был, и следующий вход в той же вкладке начинался с чужой ленты: бейдж и
// тексты упоминаний прошлого читателя — вместе с заголовками задач внутри —
// на экране следующего.
//
// Видно это до ответа load(), а если load() не ответил — навсегда: его catch
// намеренно «оставляет что было», и для рвущейся связи это верно, а для ленты,
// которая больше никому не принадлежит, — нет.
//
// reset() для этого и был написан, экспортирован и ни разу не вызван. Второй
// половиной починки идёт сам reset(): загрузка, поднятая уходящей сессией,
// может быть ещё в полёте, и её ответ от свежего ничем не отличается.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, it, expect, vi } from 'vitest'

const list = vi.fn(() => Promise.resolve({ data: [] }))
const logout = vi.fn(() => Promise.resolve())
vi.mock('@/api', () => ({
  auth: {
    logout: (...a) => logout(...a),
    me: vi.fn(() => Promise.resolve({ data: {} })),
  },
  notifications: {
    list: (...a) => list(...a),
    markRead: vi.fn(() => Promise.resolve()),
    markAllRead: vi.fn(() => Promise.resolve()),
  },
  setAccessToken: vi.fn(),
  restoreSession: vi.fn(() => Promise.resolve(null)),
}))

import { useNotificationsStore } from '@/stores/notifications'
import { useAuthStore } from '@/stores/auth'

const notif = (id, over = {}) => ({ id, text: id, read_at: null, ...over })
const event = (n, userId = 'me') => ({
  type: 'notification',
  data: { user_id: userId, notification: n },
})
function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

describe('лента уведомлений: конец сессии', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    list.mockResolvedValue({ data: [] })
    logout.mockResolvedValue()
    localStorage.clear()
  })

  it('выход из аккаунта уносит и строки, и бейдж', async () => {
    const s = useNotificationsStore()
    list.mockResolvedValueOnce({ data: [notif('a'), notif('b')] })
    await s.load()
    expect(s.unread).toBe(2)

    useAuthStore().logout()

    expect(s.items).toEqual([])
    expect(s.unread).toBe(0)
    // loaded снят: иначе следующий читатель получит пустую ленту, объявленную
    // прочитанной, вместо «ещё не загружали».
    expect(s.loaded).toBe(false)
  })

  it('чистит ленту и на истёкшей сессии, не только на кнопке «выйти»', async () => {
    const s = useNotificationsStore()
    list.mockResolvedValueOnce({ data: [notif('a')] })
    await s.load()

    // Путь auth:expired — сервер отказал в обновлении токена, кнопку никто не
    // нажимал, а лента прошлого читателя на экране та же.
    useAuthStore().clearSession()

    expect(s.items).toEqual([])
  })

  it('не даёт загрузке уходящей сессии вернуть ленту после выхода', async () => {
    const s = useNotificationsStore()
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    const loading = s.load()
    useAuthStore().logout()
    answer.resolve({ data: [notif('a'), notif('b')] })
    await loading

    expect(s.items).toEqual([])
    expect(s.loaded).toBe(false)
  })

  it('не подмешивает в следующую ленту строки, пришедшие по сокету до выхода', async () => {
    const s = useNotificationsStore()
    const answer = deferred()
    list.mockReturnValueOnce(answer.promise)

    // Загрузка в полёте, по сокету прилетело уведомление — его собирает
    // arrivals, чтобы ответ, снятый раньше, его не стёр.
    const loading = s.load()
    s.onEvent(event(notif('live')), 'me')
    useAuthStore().logout()
    answer.resolve({ data: [] })
    await loading

    // Следующий читатель входит в той же вкладке.
    list.mockResolvedValueOnce({ data: [notif('own')] })
    await s.load()
    expect(s.items.map((n) => n.id)).toEqual(['own'])
  })

  it('после выхода лента снова загружается — сдвиг счётчика не глушит будущие запросы', async () => {
    const s = useNotificationsStore()
    useAuthStore().logout()

    list.mockResolvedValueOnce({ data: [notif('fresh')] })
    await s.load()

    expect(s.items.map((n) => n.id)).toEqual(['fresh'])
    expect(s.loaded).toBe(true)
  })
})
