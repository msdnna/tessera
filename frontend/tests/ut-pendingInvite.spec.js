// Приглашение, ждущее возврата от провайдера. Токен нельзя провезти в адресе
// через редирект GitLab, поэтому он ждёт в самой вкладке — и правила у этого
// ожидания ровно два: одноразовость (иначе протухший токен встретит СЛЕДУЮЩИЙ
// вход в этой вкладке) и «пустое не паркуем» (иначе обычный вход уедет на
// страницу приёма) (#2932).
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { stashPendingInvite, takePendingInvite } from '@/utils/pendingInvite'

describe('припаркованное приглашение', () => {
  beforeEach(() => sessionStorage.clear())
  afterEach(() => vi.restoreAllMocks())

  it('возвращается один раз и больше не возвращается', () => {
    stashPendingInvite('inv-tok')
    expect(takePendingInvite()).toBe('inv-tok')
    expect(takePendingInvite()).toBe('')
  })

  it('на пустом месте отдаёт пустоту', () => {
    expect(takePendingInvite()).toBe('')
  })

  it('пустой и не-строковый токен не паркуются', () => {
    stashPendingInvite('')
    expect(takePendingInvite()).toBe('')
    stashPendingInvite(undefined)
    expect(takePendingInvite()).toBe('')
    stashPendingInvite({ token: 'inv-tok' })
    expect(takePendingInvite()).toBe('')
  })

  // Обычный вход после входа по приглашению: парковка пустым должна СТИРАТЬ
  // прошлый токен, а не просто ничего не делать.
  it('парковка пустым стирает прошлое приглашение', () => {
    stashPendingInvite('inv-tok')
    stashPendingInvite('')
    expect(takePendingInvite()).toBe('')
  })

  // Хранилище может быть запрещено (приватный режим, зажатый вебвью). Потеря
  // приглашения — неприятность; упавший вход — поломка.
  it('запрещённое хранилище не роняет вход', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied')
    })
    expect(() => stashPendingInvite('inv-tok')).not.toThrow()
    expect(takePendingInvite()).toBe('')
  })
})
