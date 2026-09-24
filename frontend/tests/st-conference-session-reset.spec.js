// Живой созвон переживал выход из аккаунта (#2932).
//
// Сессия созвона поднята НАД роутером (#2888): транспорт и сокет комнаты живут
// в сторе, а ConferenceRoom и плавающее мини-окно — только два её представления.
// Это и есть дыра: выход из аккаунта в SPA — смена маршрута, а не перезагрузка,
// размонтирование оболочки сессию не трогает, а clearSession() про неё не знал.
// Токен SFU к этому моменту уже выдан, и LiveKit проверяет только подпись — так
// что камера и микрофон продолжали вещать в комнату поверх экрана входа.
//
// Вторая половина — порядок внутри hangup(). Сначала гасим медиа и сокет, и
// только потом отпускаем место на сервере: `confApi.leave` — запрос без
// собственного дедлайна, и на офлайне «положить трубку» оставляло микрофон
// включённым, пока он не отвалится сам.
//
// Заодно чистится стор конфликтов: его `clear()` был написан ровно под выход из
// аккаунта (о чём прямо говорит комментарий про logout в conflicts.js) и не был
// подключён никуда.
import { setActivePinia, createPinia } from 'pinia'
import { ref } from 'vue'
import { beforeEach, describe, it, expect, vi } from 'vitest'

const confLeave = vi.fn(() => Promise.resolve())
const glConflicts = vi.fn(() => Promise.resolve({ data: [] }))

vi.mock('@/api', () => ({
  auth: {
    logout: vi.fn(() => Promise.resolve()),
    me: vi.fn(() => Promise.resolve({ data: {} })),
  },
  notifications: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    markRead: vi.fn(() => Promise.resolve()),
    markAllRead: vi.fn(() => Promise.resolve()),
  },
  workspaces: {
    list: vi.fn(() => Promise.resolve({ data: [] })),
    groups: vi.fn(() => Promise.resolve({ data: [] })),
    projects: vi.fn(() => Promise.resolve({ data: [] })),
    commands: vi.fn(() => Promise.resolve({ data: {} })),
    remove: vi.fn(() => Promise.resolve()),
  },
  projects: { boards: vi.fn(() => Promise.resolve({ data: [] })) },
  gitlab: {
    getConnection: vi.fn(() => Promise.resolve({ data: {} })),
    connect: vi.fn(() => Promise.resolve({ data: {} })),
    disconnect: vi.fn(() => Promise.resolve()),
    conflicts: (...a) => glConflicts(...a),
  },
  conferences: { leave: (...a) => confLeave(...a) },
  setAccessToken: vi.fn(),
  restoreSession: vi.fn(() => Promise.resolve(null)),
}))

// Транспорт и комната подменены целиком: живой livekit-клиент и сокет сюда не
// нужны, проверяется распоряжение сессией, а не сам созвон.
const transport = {
  micOn: ref(false),
  camOn: ref(false),
  join: vi.fn(),
  leave: vi.fn(() => Promise.resolve()),
}
const room = {
  forceMuted: ref(false),
  ended: ref(false),
  open: vi.fn(),
  close: vi.fn(),
  setMedia: vi.fn(),
}
vi.mock('@/composables/useConfTransport', () => ({ useConfTransport: () => transport }))
vi.mock('@/composables/useConfRoom', () => ({ useConfRoom: () => room }))

import { useConferenceSession } from '@/stores/conference'
import { useConflictsStore } from '@/stores/conflicts'
import { useAuthStore } from '@/stores/auth'

function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  sessionStorage.clear()
  vi.clearAllMocks()
  transport.micOn.value = false
  transport.camOn.value = false
  room.forceMuted.value = false
  room.ended.value = false
  confLeave.mockImplementation(() => Promise.resolve())
})

describe('выход из аккаунта гасит живой созвон', () => {
  it('роняет медиа и сокет комнаты, а не только прячет мини-окно', async () => {
    const session = useConferenceSession()
    session.start('c1', 'Планёрка')
    expect(session.active).toBe(true)

    useAuthStore().clearSession()
    await vi.waitFor(() => expect(transport.leave).toHaveBeenCalled())

    expect(room.close).toHaveBeenCalled()
    expect(session.activeId).toBe('')
    expect(session.title).toBe('')
  })

  it('отпускает место на сервере — токен ещё живой, пока clearSession не дошёл до него', async () => {
    const session = useConferenceSession()
    session.start('c1', 'Планёрка')

    useAuthStore().clearSession()
    await vi.waitFor(() => expect(confLeave).toHaveBeenCalledWith('c1'))
  })

  it('не оставляет созвон, в который вернётся перезагрузка вкладки', async () => {
    const session = useConferenceSession()
    session.start('c1', 'Планёрка')
    expect(sessionStorage.getItem('tessera_conf_active')).toBe('c1')

    useAuthStore().clearSession()
    await vi.waitFor(() => expect(transport.leave).toHaveBeenCalled())

    expect(sessionStorage.getItem('tessera_conf_active')).toBeNull()
  })

  it('без созвона никого не трогает и лишнего запроса не шлёт', async () => {
    useAuthStore().clearSession()
    await Promise.resolve()

    expect(transport.leave).not.toHaveBeenCalled()
    expect(confLeave).not.toHaveBeenCalled()
  })
})

describe('трубка кладётся до похода на сервер, а не после', () => {
  it('микрофон гаснет, пока запрос «выйти» ещё висит', async () => {
    const session = useConferenceSession()
    session.start('c1', 'Планёрка')

    const d = deferred()
    confLeave.mockImplementation(() => d.promise)
    const inflight = session.hangup()

    // Висящий (офлайн) запрос не держит медиа: комната уже закрыта, транспорт
    // отпущен, мини-окно снято.
    await vi.waitFor(() => expect(transport.leave).toHaveBeenCalled())
    expect(room.close).toHaveBeenCalled()
    expect(session.active).toBe(false)

    d.resolve()
    await inflight
    expect(confLeave).toHaveBeenCalledWith('c1')
  })

  it('отказ сервера не оставляет сессию поднятой', async () => {
    const session = useConferenceSession()
    session.start('c1', 'Планёрка')
    confLeave.mockImplementation(() => Promise.reject(new Error('409 ended')))

    await session.hangup()

    expect(session.active).toBe(false)
    expect(transport.leave).toHaveBeenCalled()
  })
})

describe('выход из аккаунта уносит конфликты GitLab', () => {
  it('снимает счётчик бейджа и отметки на карточках', async () => {
    const conflicts = useConflictsStore()
    glConflicts.mockImplementation(() =>
      Promise.resolve({
        data: [
          { id: 'x1', task_id: 't1' },
          { id: 'x2', task_id: 't2' },
        ],
      }),
    )
    await conflicts.load('w1')
    expect(conflicts.count).toBe(2)
    expect(conflicts.has('t1')).toBe(true)

    useAuthStore().clearSession()

    expect(conflicts.count).toBe(0)
    expect(conflicts.has('t1')).toBe(false)
  })

  it('забывает и сам воркспейс: обновление счётчика не ходит за чужим', async () => {
    const conflicts = useConflictsStore()
    await conflicts.load('w1')
    useAuthStore().clearSession()
    glConflicts.mockClear()

    // Резолвер зовёт load() без аргумента; без сброса wsId это был бы запрос
    // конфликтов воркспейса ушедшего читателя.
    await conflicts.load()

    expect(glConflicts).not.toHaveBeenCalled()
    expect(conflicts.list).toEqual([])
  })

  it('ответ, поднятый уходящей сессией, не доливает чужие конфликты назад', async () => {
    const conflicts = useConflictsStore()
    const d = deferred()
    glConflicts.mockImplementation(() => d.promise)
    const inflight = conflicts.load('w1')

    useAuthStore().clearSession()
    d.resolve({ data: [{ id: 'x1', task_id: 't1' }] })
    await inflight

    expect(conflicts.list).toEqual([])
  })
})
