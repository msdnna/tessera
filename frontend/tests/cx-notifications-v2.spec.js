import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

// The v2 notifications (#3033): the new kinds in the feed's words (#3048), the
// page of all notifications read from the server a page at a time (#3049), and
// «Принять / Отклонить» an invitation right in its row (#3051).

const api = vi.hoisted(() => ({
  list: vi.fn(() => Promise.resolve({ data: [] })),
  markRead: vi.fn(() => Promise.resolve()),
  markAllRead: vi.fn(() => Promise.resolve()),
  unreadCount: vi.fn(() =>
    Promise.resolve({ data: { count: 3, by_group: { mentions: 1, assigned: 0, gitlab: 1 } } }),
  ),
  page: vi.fn(),
}))
const confApi = vi.hoisted(() => ({ rsvp: vi.fn(() => Promise.resolve({ data: {} })) }))
const resolver = vi.hoisted(() => ({ openResolver: vi.fn() }))
vi.mock('@/api', () => ({
  notifications: api,
  conferences: confApi,
  apiBaseURL: () => '/api',
}))
vi.mock('@/stores/conflicts', () => ({ useConflictsStore: () => resolver }))
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return { ...actual, useMessage: () => ({ error: vi.fn(), success: vi.fn() }) }
})

import NotifFeed from '@/components/NotifFeed.vue'
import { useNotificationsStore } from '@/stores/notifications'
import { notificationText } from '@/utils/notificationText'
import { notificationRoute } from '@/utils/notificationRoute'
import { notificationGroup, inFilter, groupByDay } from '@/utils/notificationFeed'
import { notificationKind, notificationCategory } from '@/utils/toastModel'
import { buildNotificationToast } from '@/utils/notificationToast'
import { i18n, setI18nLocale } from '@/i18n'

const ago = (m) => new Date(Date.now() - m * 60000).toISOString()
const invite = (over = {}) => ({
  id: 'c1',
  kind: 'conference_invite',
  created_at: ago(20),
  read_at: null,
  rsvp: 'pending',
  payload: {
    event: 'conference_invited',
    actor: 'Илья Петров',
    title: 'Синк',
    conference_id: 'conf-1',
    scheduled_at: new Date(Date.now() + 3600e3).toISOString(),
  },
  ...over,
})
const conflict = () => ({
  id: 'x1',
  kind: 'integration_conflict',
  task_id: 'task-9',
  task_number: 418,
  task_board_id: 'b1',
  created_at: ago(60),
  read_at: null,
  payload: {
    event: 'integration_conflict',
    provider: 'gitlab',
    label: 'GitLab',
    task_number: 418,
    title: 'Экспорт',
    fields: ['description'],
  },
})
const stage = () => ({
  id: 's1',
  kind: 'milestone_closed',
  created_at: ago(60 * 30),
  read_at: ago(60),
  payload: {
    event: 'milestone_closed',
    actor: 'Илья Петров',
    title: 'Спринт 41',
    done: 19,
    total: 22,
    project: 'Tessera Web',
    project_slug: 'tessera-web',
    milestone_id: 'm-41',
  },
})

let wrapper = null
async function mountFeed(props = {}, items = []) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:rest(.*)*', component: { template: '<div />' } }],
  })
  router.push('/')
  await router.isReady()
  const store = useNotificationsStore()
  store.items = items
  wrapper = mount(NotifFeed, { props, attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { store, router }
}

beforeEach(async () => {
  setActivePinia(createPinia())
  for (const fn of [...Object.values(api), confApi.rsvp, resolver.openResolver]) fn.mockClear()
  await setI18nLocale('ru')
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('the new kinds in words (#3048)', () => {
  const t = i18n.global.t
  const te = i18n.global.te
  it('words each new event in both languages', async () => {
    const cases = [
      [
        stage().payload,
        'Илья Петров закрыл(а) этап «Спринт 41» — выполнено 19 из 22',
        'Илья Петров closed stage “Спринт 41” — 19 of 22 done',
      ],
      [
        { event: 'conference_starting', title: 'Синк', minutes: 5 },
        'Через 5 мин начнётся встреча «Синк»',
        'Meeting “Синк” starts in 5 min',
      ],
      [
        conflict().payload,
        'GitLab · конфликт синхронизации в #418 (описание): изменено в обоих местах',
        'GitLab · sync conflict on #418 (description): changed on both sides',
      ],
      [
        { event: 'document_mention', actor: 'Анна', name: 'План', excerpt: 'глянь' },
        'Анна упомянул(а) вас в документе «План» «глянь»',
        'Анна mentioned you in document “План” “глянь”',
      ],
      [
        { event: 'conference_mention', actor: 'Анна', name: 'Стендап', excerpt: 'ок' },
        'Анна упомянул(а) вас в чате встречи «Стендап» «ок»',
        'Анна mentioned you in the chat of “Стендап” “ок”',
      ],
    ]
    for (const [payload, ruText] of cases)
      expect(notificationText({ payload, text: 'x' }, { t, te })).toBe(ruText)
    await setI18nLocale('en')
    for (const [payload, , enText] of cases)
      expect(notificationText({ payload, text: 'x' }, { t, te })).toBe(enText)
  })

  it('opens what each is about', () => {
    expect(notificationRoute(stage())).toBe('/project/tessera-web/stage/m-41')
    expect(notificationRoute({ payload: { document_slug: 'plan' } })).toBe('/documents/plan')
    expect(
      notificationRoute({ payload: { event: 'conference_starting', conference_id: 'c' } }),
    ).toBe('/conferences/c')
    expect(notificationRoute(conflict())).toBe('/board/b1?task=task-9')
  })

  it('files them under the feed filters and the toast kinds', () => {
    expect(notificationGroup(conflict())).toBe('gitlab')
    expect(notificationGroup({ kind: 'mention', payload: { event: 'document_mention' } })).toBe(
      'mentions',
    )
    expect(notificationKind({ payload: { event: 'conference_mention' } })).toBe('mention')
    expect(notificationKind({ payload: { event: 'conference_starting' } })).toBe('call')
    expect(notificationCategory(conflict())).toBe('system')
    const resolve = vi.fn()
    const spec = buildNotificationToast(conflict(), { act: { resolveConflict: resolve } })
    spec.actions[0].run()
    expect(resolve).toHaveBeenCalled()
  })
})

describe('the page helpers (#3049)', () => {
  it('reads a row under a page filter the way the server does', () => {
    expect(inFilter({ read_at: null }, 'unread')).toBe(true)
    expect(inFilter({ read_at: 'x' }, 'unread')).toBe(false)
    expect(inFilter(conflict(), 'gitlab')).toBe(true)
    expect(inFilter(conflict(), 'mentions')).toBe(false)
  })

  it('groups rows by day, newest first', () => {
    const t = (k) => k
    const formatters = {
      parts: (d) => ({ year: 1, month: 1, day: d.getUTCDate() }),
      today: (now) => ({ year: 1, month: 1, day: now.getUTCDate() }),
      formatDate: () => 'DATE',
    }
    const now = new Date('2026-10-10T12:00:00Z')
    const rows = [
      { id: 'a', created_at: '2026-10-10T10:00:00Z' },
      { id: 'b', created_at: '2026-10-10T09:00:00Z' },
      { id: 'c', created_at: '2026-10-09T09:00:00Z' },
      { id: 'd', created_at: '2026-10-01T09:00:00Z' },
    ]
    const g = groupByDay(rows, { t, formatters, now })
    expect(g.map((x) => [x.label, x.rows.map((r) => r.id)])).toEqual([
      ['notifications.pop.today', ['a', 'b']],
      ['notifications.pop.yesterday', ['c']],
      ['DATE', ['d']],
    ])
  })
})

describe('«Принять / Отклонить» in the feed (#3051)', () => {
  it('answers an invitation and shows the answer', async () => {
    const { store } = await mountFeed({}, [invite()])
    const row = wrapper.get('[data-testid="notification-rsvp"]')
    expect(row.attributes('data-rsvp')).toBe('pending')
    await wrapper.get('[data-testid="notification-accept"]').trigger('click')
    await flushPromises()
    expect(confApi.rsvp).toHaveBeenCalledWith('conf-1', 'accepted')
    expect(store.items[0].rsvp).toBe('accepted')
    expect(store.items[0].read_at).not.toBeNull()
    expect(wrapper.get('[data-testid="notification-accept"]').text()).toContain('Вы идёте')
  })

  it('puts the row back when the server refuses', async () => {
    confApi.rsvp.mockImplementationOnce(() => Promise.reject(new Error('нет')))
    const { store } = await mountFeed({}, [invite()])
    await wrapper.get('[data-testid="notification-decline"]').trigger('click')
    await flushPromises()
    expect(store.items[0].rsvp).toBe('pending')
    expect(store.items[0].read_at).toBeNull()
  })

  it('draws nothing to answer when the call is gone', async () => {
    await mountFeed({}, [invite({ rsvp: null })])
    expect(wrapper.find('[data-testid="notification-rsvp"]').exists()).toBe(false)
  })

  it('opens the resolver from a conflict row', async () => {
    await mountFeed({}, [conflict()])
    await wrapper.get('[data-testid="notification-resolve"]').trigger('click')
    expect(resolver.openResolver).toHaveBeenCalledWith('task-9')
    expect(api.markRead).toHaveBeenCalledWith('x1')
  })

  it('«Все уведомления →» leads to the page', async () => {
    const { router } = await mountFeed({}, [stage()])
    await wrapper.get('[data-testid="notifications-all"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/notifications')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})

describe('the page of all notifications (#3049)', () => {
  it('reads pages from the server and filters there', async () => {
    api.page
      .mockResolvedValueOnce({ data: { items: [invite(), conflict()], next_cursor: 'cur-1' } })
      .mockResolvedValueOnce({ data: { items: [stage()], next_cursor: null } })
      .mockResolvedValueOnce({ data: { items: [conflict()], next_cursor: null } })
    await mountFeed({ all: true })
    expect(api.page).toHaveBeenLastCalledWith({ filter: 'all', limit: 30 })
    expect(wrapper.findAll('[data-testid="notification-row"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="notifications-filter-unread"]').text()).toContain('3')

    await wrapper.get('[data-testid="notifications-load-more"]').trigger('click')
    await flushPromises()
    expect(api.page).toHaveBeenLastCalledWith({ filter: 'all', limit: 30, cursor: 'cur-1' })
    expect(wrapper.findAll('[data-testid="notification-row"]')).toHaveLength(3)
    expect(wrapper.find('[data-testid="notifications-load-more"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Это все уведомления')

    await wrapper.get('[data-testid="notifications-filter-gitlab"]').trigger('click')
    await flushPromises()
    expect(api.page).toHaveBeenLastCalledWith({ filter: 'gitlab', limit: 30 })
    expect(wrapper.findAll('[data-testid="notification-row"]')).toHaveLength(1)
  })

  it('takes a live arrival that belongs under the filter', async () => {
    api.page.mockResolvedValueOnce({ data: { items: [stage()], next_cursor: null } })
    const { store } = await mountFeed({ all: true })
    store.items = [conflict(), ...store.items]
    await flushPromises()
    const ids = wrapper
      .findAll('[data-testid="notification-row"]')
      .map((r) => r.attributes('data-row'))
    expect(ids).toEqual(['x1', 's1'])
  })
})
