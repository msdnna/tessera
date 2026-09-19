import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

// Пагинация действий в журнале синка (#2932) — страница, приехавшая «не для этого
// состояния».
//
// Журнал грузит действия прогона keyset-страницами и кэширует их в actionsByRun.
// Кэш — ещё и признак «уже загружено»: toggleRun перезапрашивает страницу только
// если записи нет. Значит любая страница, легшая в кэш не вовремя, залипает там
// насовсем — раскрыть/свернуть её больше не обновит.
//
// Две двери, через которые такая страница приезжает:
//  * прогон ЕЩЁ ШЁЛ, когда пользователь его раскрыл: действия пишутся в конце, и
//    ответ — пустая страница. Пришедшее следом событие integration.sync чистит
//    кэш, но если запрос ещё в полёте — чистить нечего, и пустая страница ляжет
//    ПОСЛЕ чистки. Пользователь смотрит на «Действий нет» у прогона, который
//    только что записал сорок действий;
//  * пользователь свернул прогон и раскрыл соседний, пока страница летела.
//
// Обе проверяются здесь, а не в e2e: журнал за GitLab-привязкой, которую заводит
// только глобальный админ с живым GitLab, — e2e-аккаунт этой панели не видит
// вовсе (см. cx-panel-master-detail.spec.js).

const api = {
  gitlab: {
    syncRuns: vi.fn(),
    syncRunActions: vi.fn(),
    syncActionDetail: vi.fn(),
    retryWriteback: vi.fn(),
  },
}
vi.mock('@/api', () => api)

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// Перехватываем оба колбэка useRealtime, чтобы гнать события руками.
let emitEvent = () => {}
vi.mock('@/composables/useRealtime', () => ({
  useRealtime: (onEvent) => {
    emitEvent = onEvent
    return { on: () => () => {} }
  },
}))

const stubs = {
  Icon: { template: '<i class="n-icon-stub"><slot /></i>' },
  Button: { template: '<button class="n-button-stub"><slot /></button>' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /></div>' },
  Modal: { template: '<div class="n-modal-stub"><slot name="header" /><slot /></div>' },
}

function run(id, { running = false } = {}) {
  return {
    id,
    kind: 'push',
    trigger: 'manual',
    status: running ? 'running' : 'ok',
    started_at: '2026-09-07T10:00:00Z',
    finished_at: running ? null : '2026-09-07T10:00:20Z',
    created_count: 0,
    updated_count: running ? 0 : 3,
    failed_count: 0,
  }
}

function action(id, summary) {
  return {
    id,
    op: 'update',
    status: 'ok',
    direction: 'push',
    summary,
    has_detail: false,
    detail: {},
  }
}

const page = (items) => ({ data: { items, has_more: false, next_after_seq: null } })

// deferred отдаёт промис, который резолвится извне — так ответ страницы можно
// задержать ровно до нужного момента.
function deferred() {
  let resolve
  const promise = new Promise((r) => (resolve = r))
  return { promise, resolve }
}

const wrappers = []
afterEach(() => {
  while (wrappers.length) wrappers.pop().unmount()
})

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

async function mountJournal() {
  const C = (await import('@/components/GitLabJournalPanel.vue')).default
  const w = mount(C, { props: { wsId: 'ws1' }, global: { stubs } })
  wrappers.push(w)
  await flushPromises()
  return w
}

const summaries = (w) => w.findAll('.j-action-sum').map((el) => el.text())

describe('GitLabJournalPanel.vue — страницы действий и живое обновление', () => {
  it('показывает действия прогона, который финишировал прямо во время загрузки', async () => {
    api.gitlab.syncRuns.mockResolvedValue({ data: [run('r1', { running: true })] })
    const inFlight = deferred()
    api.gitlab.syncRunActions.mockReturnValueOnce(inFlight.promise)
    const w = await mountJournal()

    // Пользователь раскрывает идущий прогон — запрос уходит и зависает.
    await w.find('.j-run-head').trigger('click')
    await flushPromises()

    // Прогон финиширует: приходит событие, список перечитывается.
    api.gitlab.syncRuns.mockResolvedValue({ data: [run('r1')] })
    api.gitlab.syncRunActions.mockResolvedValue(page([action('a1', 'Задача #12 → GitLab')]))
    emitEvent({ type: 'integration.sync', scope: 'ws1', data: { run_id: 'r1' } })
    await flushPromises()

    // И только теперь доезжает ответ, снятый с ещё идущего прогона: пусто.
    inFlight.resolve(page([]))
    await flushPromises()

    // Он не должен вытеснить актуальную страницу и не должен считаться «уже
    // загружено»: прогон раскрыт, его действия обязаны быть на экране.
    expect(summaries(w)).toEqual(['Задача #12 → GitLab'])
  })

  it('не подсовывает страницу покинутого прогона в раскрытый соседний', async () => {
    api.gitlab.syncRuns.mockResolvedValue({ data: [run('r1'), run('r2')] })
    const stale = deferred()
    api.gitlab.syncRunActions.mockReturnValueOnce(stale.promise)
    const w = await mountJournal()

    const heads = () => w.findAll('.j-run-head')
    // Раскрыли первый прогон — страница в полёте.
    await heads()[0].trigger('click')
    await flushPromises()

    // Передумали: свернули его и раскрыли второй.
    api.gitlab.syncRunActions.mockResolvedValue(page([action('b1', 'Задача #77 → GitLab')]))
    await heads()[0].trigger('click')
    await heads()[1].trigger('click')
    await flushPromises()

    // Ответ первого доезжает последним.
    stale.resolve(page([action('a1', 'Задача #12 → GitLab')]))
    await flushPromises()

    // На экране раскрыт только второй прогон — и в нём только его действия.
    expect(summaries(w)).toEqual(['Задача #77 → GitLab'])
  })

  it('не сообщает «действий нет», пока страница раскрытого прогона ещё летит', async () => {
    api.gitlab.syncRuns.mockResolvedValue({ data: [run('r1'), run('r2')] })
    const first = deferred()
    const second = deferred()
    api.gitlab.syncRunActions.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const w = await mountJournal()

    const heads = () => w.findAll('.j-run-head')
    // Оба запроса в полёте: раскрыли первый, передумали, раскрыли второй.
    await heads()[0].trigger('click')
    await flushPromises()
    await heads()[0].trigger('click')
    await heads()[1].trigger('click')
    await flushPromises()

    // Первый отвечает раньше — но раскрыт второй, и он ещё грузится.
    first.resolve(page([action('a1', 'Задача #12 → GitLab')]))
    await flushPromises()
    expect(w.find('.j-muted').exists()).toBe(true)
    // Именно в раскрытом прогоне — в правой панели «выберите действие» законно.
    expect(w.find('.j-actions .empty-state').exists()).toBe(false)

    second.resolve(page([action('b1', 'Задача #77 → GitLab')]))
    await flushPromises()
    expect(summaries(w)).toEqual(['Задача #77 → GitLab'])
  })
})
