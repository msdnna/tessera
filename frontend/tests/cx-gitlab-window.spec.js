// Окно GitLab на языке v2 (#2975): привязка по-прежнему сохраняется одним PUT
// всей формы, поэтому кнопка «Сохранить» осталась — но появляется только когда
// форма отличается от сохранённой; «Сбросить» возвращает её. И попутная
// поломка: в connect() локальная `t` (токен) затеняла i18n-`t`, и после
// успешного подключения строка «подключён как …» бросала TypeError — catch
// показывал ошибку при живом подключении.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

const ok = (data) => Promise.resolve({ data })
const BINDING = {
  id: 'b1',
  name: 'Скрам',
  project_path: 'web/client',
  board_id: 'board-1',
  enabled: true,
  sync_interval_sec: 0,
  full_sync_interval_sec: 86400,
  scope: 'all',
  label_rules: { rules: [{ match: 'S: ', match_type: 'prefix', action: 'status' }] },
  writeback: { enabled: false },
}
const api = {
  gitlab: {
    getConnection: vi.fn(() => ok({ connected: false })),
    connect: vi.fn(() => ok({ connected: true, base_url: 'https://gl', gl_username: 'octo' })),
    listIntegrations: vi.fn(() =>
      ok({ integrations: [BINDING], default_rules: {}, service_configured: false, is_admin: true }),
    ),
    updateIntegration: vi.fn((ws, id, payload) => ok({ ...BINDING, ...payload, id })),
    conflicts: vi.fn(() => ok([])),
    syncRuns: vi.fn(() => ok([])),
    previewRules: vi.fn(() =>
      ok({
        hits: [5],
        linked: 5,
        recorded: 5,
        check: { outcome: { label: 'S: Done', rule: 0, action: 'status', result: '' }, tasks: 2 },
      }),
    ),
  },
  projects: {
    boards: vi.fn(() => ok([{ id: 'board-1', name: 'Доска' }])),
    tagPrefixes: vi.fn(() => ok([])),
    setTagPrefixes: vi.fn(() => ok([])),
  },
  boards: { columns: vi.fn(() => ok([])) },
}
vi.mock('@/api', () => api)
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return { ...actual, useMessage: () => toast }
})
// The message API moved out of naive (composables/useMessage, #3117): route it
// to the naive mock above, where this spec keeps its spies.
vi.mock('@/composables/useMessage', async () => ({
  useMessage: (await import('naive-ui')).useMessage,
}))
vi.mock('@/composables/useRealtime', () => ({ useRealtime: () => ({ on: () => () => {} }) }))

const stubs = {
  Modal: { template: '<div class="n-modal-stub"><slot /></div>' },
  Tooltip: { template: '<div class="n-tooltip-stub"><slot name="trigger" /></div>' },
}

async function open() {
  const { useWorkspacesStore } = await import('@/stores/workspaces')
  useWorkspacesStore().projects = [{ id: 'p1', name: 'Проект' }]
  const C = (await import('@/components/GitLabModal.vue')).default
  const w = mount(C, { props: { show: false, wsId: 'ws1' }, global: { stubs } })
  await w.setProps({ show: true })
  await flushPromises()
  return w
}
const nameInput = (w) =>
  w.findAll('[data-testid="gitlab-binding-form"] input').find((i) => i.element.value === 'Скрам')

describe('GitLabModal · сохранение привязки', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('без правок панели сохранения нет', async () => {
    const w = await open()
    expect(w.find('[data-testid="gitlab-binding-card"]').exists()).toBe(true)
    expect(w.find('[data-testid="gitlab-dirty"]').exists()).toBe(false)
  })

  it('правка показывает панель, «Сохранить» шлёт всю привязку', async () => {
    const w = await open()
    await nameInput(w).setValue('Канбан')
    await flushPromises()
    expect(w.find('[data-testid="gitlab-dirty"]').exists()).toBe(true)
    await w.find('[data-testid="gitlab-save"]').trigger('click')
    await flushPromises()
    expect(api.gitlab.updateIntegration).toHaveBeenCalledTimes(1)
    const [, id, payload] = api.gitlab.updateIntegration.mock.calls[0]
    expect(id).toBe('b1')
    expect(payload.name).toBe('Канбан')
    expect(payload.project_path).toBe('web/client')
    expect(payload.label_rules.rules).toHaveLength(1)
    expect(w.find('[data-testid="gitlab-dirty"]').exists()).toBe(false)
  })

  it('«Сбросить» возвращает форму к сохранённой', async () => {
    const w = await open()
    await nameInput(w).setValue('Канбан')
    await flushPromises()
    const reset = w.findAll('[data-testid="gitlab-dirty"] button')[0]
    await reset.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="gitlab-dirty"]').exists()).toBe(false)
    expect(nameInput(w)).toBeTruthy()
    expect(api.gitlab.updateIntegration).not.toHaveBeenCalled()
  })
})

describe('GitLabModal · личный аккаунт', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('удачное подключение говорит «подключён», а не ошибку', async () => {
    const w = await open()
    await w.find('[data-testid="gitlab-base-url"] input').setValue('https://gl')
    await w.find('[data-testid="gitlab-token"] input').setValue('glpat-x')
    await w.find('[data-testid="gitlab-account-connect"]').trigger('click')
    await flushPromises()
    expect(api.gitlab.connect).toHaveBeenCalledWith({ base_url: 'https://gl', token: 'glpat-x' })
    expect(toast.error).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalledTimes(1)
    expect(toast.success.mock.calls[0][0]).toContain('octo')
  })
})

// #3087: у правила есть выключатель и счётчик задач, «Проверить лейбл» отвечает
// на правилах из формы — выключенное правило уходит в предпросмотр и в PUT как
// disabled. #3088: политика конфликтов уходит в writeback.conflict_policy.
describe('GitLabModal · правила и обратная запись', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  it('выключатель, счётчик и проверка лейбла', async () => {
    const w = await open()
    await w.find('[data-testid="gitlab-tab-rules"]').trigger('click')
    await flushPromises()
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(w.find('[data-testid="gitlab-rule-hits"]').text()).toContain('5')
    await w.find('[data-testid="gitlab-rule-check-input"]').setValue('S: Done')
    vi.advanceTimersByTime(400)
    await flushPromises()
    const last = api.gitlab.previewRules.mock.calls.at(-1)
    expect(last[0]).toBe('ws1')
    expect(last[1]).toMatchObject({ integration_id: 'b1', label: 'S: Done' })
    expect(w.find('[data-testid="gitlab-rule-check-out"]').text()).toContain('2')
    // Space на строке — выключить правило.
    const row = w.find('[data-testid="gitlab-rule-row"]')
    await row.trigger('focus')
    await row.trigger('keydown', { key: ' ' })
    vi.advanceTimersByTime(400)
    await flushPromises()
    expect(api.gitlab.previewRules.mock.calls.at(-1)[1].label_rules.rules[0].disabled).toBe(true)
    await w.find('[data-testid="gitlab-save"]').trigger('click')
    await flushPromises()
    const [, , payload] = api.gitlab.updateIntegration.mock.calls[0]
    expect(payload.label_rules.rules[0].disabled).toBe(true)
    vi.useRealTimers()
  })

  it('политика конфликтов уходит в writeback', async () => {
    const w = await open()
    await w.find('[data-testid="gitlab-tab-writeback"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="gitlab-conflict-policy-gitlab"]').trigger('click')
    await w.find('[data-testid="gitlab-save"]').trigger('click')
    await flushPromises()
    const [, , payload] = api.gitlab.updateIntegration.mock.calls[0]
    expect(payload.writeback.conflict_policy).toBe('gitlab')
    vi.useRealTimers()
  })
})
