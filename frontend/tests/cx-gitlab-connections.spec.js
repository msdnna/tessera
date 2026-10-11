// «Подключения» окна GitLab (#3065): срок токена у аккаунта, привязка на паузе
// после отказа GitLab с «Переподключить», «Войти через GitLab», «Ещё проект на ту
// же доску» и фильтр импорта по лейблу, который уходит списком.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

const ok = (data) => Promise.resolve({ data })
const PAUSED = {
  id: 'b1',
  name: '',
  project_path: 'web/client',
  board_id: 'board-1',
  enabled: true,
  scope: 'all',
  import_labels: ['backend'],
  paused_at: '2026-10-11T10:00:00Z',
  pause_reason: 'unauthorized',
  auth_kind: 'pat',
  token_expires_at: '2020-01-01',
  label_rules: { rules: [] },
  writeback: { enabled: false },
}
const api = {
  gitlab: {
    getConnection: vi.fn(() =>
      ok({
        connected: true,
        base_url: 'https://gl',
        gl_username: 'octo',
        auth_kind: 'pat',
        token_expires_at: '2099-01-12',
      }),
    ),
    connect: vi.fn(() => ok({ connected: true, base_url: 'https://gl', gl_username: 'octo' })),
    connectOAuth: vi.fn(() => ok({ url: 'https://gl/oauth/authorize?x' })),
    listIntegrations: vi.fn(() =>
      ok({ integrations: [PAUSED], default_rules: {}, service_configured: false, is_admin: true }),
    ),
    createIntegration: vi.fn((ws, payload) => ok({ ...payload, id: 'b2' })),
    updateIntegration: vi.fn((ws, id, payload) => ok({ ...PAUSED, ...payload, id })),
    conflicts: vi.fn(() => ok([])),
    syncRuns: vi.fn(() => ok([])),
    previewRules: vi.fn(() => ok({ hits: [], linked: 0, recorded: 0 })),
  },
  auth: { providers: vi.fn(() => ok({ gitlab: true })) },
  projects: {
    boards: vi.fn(() => ok([{ id: 'board-1', name: 'Доска' }])),
    tagPrefixes: vi.fn(() => ok([])),
    setTagPrefixes: vi.fn(() => ok([])),
  },
  boards: { columns: vi.fn(() => ok([])) },
}
vi.mock('@/api', () => api)
const toast = { error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }
vi.mock('@/composables/useMessage', () => ({ useMessage: () => toast }))
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

describe('GitLab · подключения (#3065)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('срок токена, пауза и «Переподключить» открывает форму токена', async () => {
    const w = await open()
    expect(w.find('[data-testid="gitlab-token-expiry"]').text()).toContain('2099')
    const card = w.find('[data-testid="gitlab-binding-card"]')
    expect(card.classes()).toContain('err')
    expect(card.text()).toContain('backend')
    // Форма токена скрыта, пока аккаунт подключён…
    expect(w.find('[data-testid="gitlab-token"]').exists()).toBe(false)
    await w.find('[data-testid="gitlab-reconnect"]').trigger('click')
    await flushPromises()
    // …и появляется по «Переподключить».
    expect(w.find('[data-testid="gitlab-token"]').exists()).toBe(true)
  })

  it('«Войти через GitLab» идёт на адрес авторизации', async () => {
    const assign = vi.fn()
    const loc = window.location
    Object.defineProperty(window, 'location', {
      value: { ...loc, assign, pathname: '/board/x', search: '' },
      configurable: true,
    })
    const w = await open()
    await w.find('[data-testid="gitlab-oauth-connect"]').trigger('click')
    await flushPromises()
    expect(api.gitlab.connectOAuth).toHaveBeenCalledWith('/board/x')
    expect(assign).toHaveBeenCalledWith('https://gl/oauth/authorize?x')
    Object.defineProperty(window, 'location', { value: loc, configurable: true })
  })

  it('ещё проект на ту же доску — новая привязка с этой доской и фильтром', async () => {
    const w = await open()
    await w.find('[data-testid="gitlab-same-board"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="gitlab-project-path"] input').setValue('web/server')
    await w.find('[data-testid="gitlab-create-binding"]').trigger('click')
    await flushPromises()
    const [, payload] = api.gitlab.createIntegration.mock.calls[0]
    expect(payload.board_id).toBe('board-1')
    expect(payload.project_path).toBe('web/server')
    expect(payload.import_labels).toEqual([])
  })
})
