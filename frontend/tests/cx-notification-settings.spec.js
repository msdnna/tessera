import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

// Deleting a channel or a routing rule used to go through window.confirm, then
// an n-popconfirm. Since v2 (#3106) it is the two-step item of the row's «⋯»
// ActionMenu — so what's worth locking down is that the API call sits behind
// the second press and that the armed item names what's about to disappear.

const chApi = { list: vi.fn(), remove: vi.fn(), update: vi.fn() }
const rtApi = { list: vi.fn(), remove: vi.fn(), update: vi.fn() }
const prefsApi = { get: vi.fn(), update: vi.fn() }

vi.mock('@/api', () => ({
  notificationChannels: chApi,
  notificationRoutes: rtApi,
  notificationPrefs: prefsApi,
}))

vi.mock('@/utils/device', () => ({
  getDeviceId: () => 'dev-1',
  notificationsSupported: () => false,
}))

// The component reads only `timezone` off the theme store; the real one drags in
// the API client and the whole preference hydration for nothing.
const themeState = { timezone: '' }
vi.mock('@/stores/theme', () => ({ useThemeStore: () => themeState }))

const CHANNELS = [
  { id: 'ch1', type: 'email', label: 'Почта', enabled: true, config: {} },
  { id: 'ch2', type: 'webhook', label: '', enabled: true, config: {} },
]
const ROUTES = [
  { id: 'rt1', matcher: {}, channel_ids: ['ch1'], options: {}, enabled: true, position: 1 },
]

async function mountSettings(routes = ROUTES) {
  chApi.list.mockResolvedValue({ data: CHANNELS })
  rtApi.list.mockResolvedValue({ data: routes })
  prefsApi.get.mockResolvedValue({ data: {} })
  const NotificationSettings = (await import('@/components/NotificationSettings.vue')).default
  const w = mount(NotificationSettings, { attachTo: document.body })
  live.push(w)
  await flushPromises()
  return w
}

// Every mounted settings screen, unmounted after each test: a screen left alive
// keeps its debounced autosave, and on a slow runner (CI) it fired into the next
// test's mock — with the previous test's quiet_tz.
const live = []
afterEach(() => {
  while (live.length) live.pop().unmount()
})

// The menu teleports to <body>; its rows are reached through the document.
const $ = (sel) => document.body.querySelector(sel)
async function click(el) {
  el.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await flushPromises()
}
async function openMenu(w, testid, index = 0) {
  await w.findAll(`[data-testid="${testid}"]`)[index].trigger('click')
  await flushPromises()
}

describe('NotificationSettings.vue — удаление двухшаговым пунктом меню', () => {
  let w
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })
  afterEach(() => {
    w?.unmount()
    document.body.innerHTML = ''
  })

  it('удаляет канал только со второго нажатия', async () => {
    w = await mountSettings()
    chApi.remove.mockResolvedValue({})
    await openMenu(w, 'notif-channel-menu', 0)
    await click($('[data-testid="notif-menu-remove"]'))
    expect(chApi.remove).not.toHaveBeenCalled()
    await click($('[data-testid="notif-menu-remove-armed"]'))
    expect(chApi.remove).toHaveBeenCalledWith('ch1')
  })

  it('называет канал во взведённом пункте и предупреждает о правилах', async () => {
    w = await mountSettings()
    await openMenu(w, 'notif-channel-menu', 0)
    await click($('[data-testid="notif-menu-remove"]'))
    // ch1 is referenced by one route → the armed item has to say so
    const armed = $('[data-testid="notif-menu-remove-armed"]').textContent
    expect(armed).toContain('Почта')
    expect(armed).toContain('в 1 правиле')
  })

  it('без правил подставляет имя типа и не пугает правилами', async () => {
    w = await mountSettings()
    await openMenu(w, 'notif-channel-menu', 1)
    await click($('[data-testid="notif-menu-remove"]'))
    const armed = $('[data-testid="notif-menu-remove-armed"]').textContent
    expect(armed).toContain('Webhook')
    expect(armed).not.toContain('правил')
  })

  it('удаляет правило маршрутизации тем же двухшаговым пунктом', async () => {
    w = await mountSettings()
    rtApi.remove.mockResolvedValue({})
    await openMenu(w, 'notif-route-menu', 0)
    await click($('[data-testid="notif-menu-remove"]'))
    expect($('[data-testid="notif-menu-remove-armed"]').textContent).toContain(
      'Удалить правило маршрутизации?',
    )
    expect(rtApi.remove).not.toHaveBeenCalled()
    await click($('[data-testid="notif-menu-remove-armed"]'))
    expect(rtApi.remove).toHaveBeenCalledWith('rt1')
  })
})

// The rule editor's event list is a mirror of what the server actually routes
// (backend internal/notify/locale.go `kinds`), and it had fallen one kind behind:
// a conference invitation is delivered like any other notification
// (handlers/conferences.go → notify(…, kindConferenceInvite, …)) and even has its
// own subject line in both languages, but it could not be picked here. So it was
// impossible to send invitations to a channel on their own — or to keep them out
// of one — and a rule that carried the kind anyway (API, another client) printed
// the bare `conference_invite` in Latin in the middle of the Russian summary.
describe('NotificationSettings.vue — виды событий в правиле', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  // The picker's options and the summary's labels are both derived from
  // EVENT_KINDS, so the summary is the cheap end to observe: a kind the list does
  // not know is one the picker cannot offer either, and here it also shows up as
  // the untranslated key.
  it('называет приглашение в конференцию в сводке правила, а не сырым ключом', async () => {
    const w = await mountSettings([
      {
        id: 'rt2',
        matcher: { kinds: ['conference_invite'] },
        channel_ids: ['ch1'],
        options: {},
        enabled: true,
        position: 1,
      },
    ])
    const title = w.findAll('.ns-rule-title').at(-1).text()
    expect(title).toBe('Приглашение в конференцию')
    expect(title).not.toContain('conference_invite')
  })
})

// Quiet hours are the one preference the *server* evaluates against a clock, so
// the zone that clock runs in has to travel with them. Everywhere else an empty
// profile timezone means "render in the browser's zone" (zonedParts falls back to
// undefined → local), and that is the zone the hint under the window promises:
// «Время — по вашему часовому поясу». The scanner reads an empty quiet_tz as UTC,
// so sending one through puts a Moscow user's 22:00–08:00 window at 01:00–11:00
// their time — pushes for the first three hours of the night, silence all morning.
describe('NotificationSettings.vue — часовой пояс тихих часов', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    themeState.timezone = ''
  })

  it('подставляет зону браузера, когда профильная не задана', async () => {
    const browserTz = Intl.DateTimeFormat().resolvedOptions().timeZone
    const w = await mountSettings()
    prefsApi.update.mockResolvedValue({ data: {} })

    await w.vm.savePrefs()
    await flushPromises()

    expect(prefsApi.update).toHaveBeenCalledTimes(1)
    expect(prefsApi.update.mock.calls[0][0].quiet_tz).toBe(browserTz)
  })

  it('не трогает явно выбранную профильную зону', async () => {
    themeState.timezone = 'Asia/Yekaterinburg'
    const w = await mountSettings()
    prefsApi.update.mockResolvedValue({ data: {} })

    await w.vm.savePrefs()
    await flushPromises()

    expect(prefsApi.update.mock.calls[0][0].quiet_tz).toBe('Asia/Yekaterinburg')
  })
})

// The mockup's «сохраняется сразу»: the schedule defaults have no save button
// any more — a change is written on its own, once, after a short debounce.
describe('NotificationSettings.vue — автосохранение расписания', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    themeState.timezone = ''
    vi.useFakeTimers()
  })
  afterEach(() => vi.useRealTimers())

  it('сохраняет изменение без кнопки и не сохраняет загрузку', async () => {
    const w = await mountSettings()
    prefsApi.update.mockResolvedValue({ data: {} })
    vi.advanceTimersByTime(1000)
    expect(prefsApi.update).not.toHaveBeenCalled()

    await w.find('[data-testid="notif-quiet"] .n-switch').trigger('click')
    vi.advanceTimersByTime(600)
    await flushPromises()
    expect(prefsApi.update).toHaveBeenCalledTimes(1)
    expect(prefsApi.update.mock.calls[0][0].quiet_enabled).toBe(true)
    w.unmount()
  })

  // «Только срочное и упоминания» (#3050) is a real switch now, saved with the
  // rest of the schedule.
  it('сохраняет исключения тихих часов', async () => {
    const w = await mountSettings()
    prefsApi.update.mockResolvedValue({ data: {} })
    await w.find('[data-testid="notif-quiet"] .n-switch').trigger('click')
    await flushPromises()
    await w.find('[data-testid="notif-quiet-exceptions"] .n-checkbox').trigger('click')
    vi.advanceTimersByTime(600)
    await flushPromises()
    expect(prefsApi.update).toHaveBeenCalledTimes(1)
    expect(prefsApi.update.mock.calls[0][0].quiet_allow_urgent).toBe(true)
    w.unmount()
  })
})
