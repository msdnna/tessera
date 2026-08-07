import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'

// Mock the api module so component imports of it don't build a real axios client.
vi.mock('@/api', () => ({
  columns: { update: vi.fn(), remove: vi.fn() },
  users: { updatePrefs: vi.fn() },
}))

// ColumnHeader calls useMessage() at setup, which throws without an <n-message-provider>.
// Keep the rest of naive-ui intact and only stub useMessage to a no-op collector.
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// Naive UI is heavy and some pieces (useMessage) need a provider. Stub the Naive
// pieces the smoke targets use so we validate our own template/props wiring, not
// Naive internals. Stubs render a marker element with a passthrough default slot.
const naiveStubs = {
  NIcon: { template: '<i class="n-icon-stub"><slot /></i>' },
  NButton: { template: '<button class="n-button-stub"><slot /></button>' },
  NInput: { template: '<input class="n-input-stub" />' },
  NPopover: { template: '<div class="n-popover-stub"><slot name="trigger" /><slot /></div>' },
  NPopconfirm: { template: '<div class="n-popconfirm-stub"><slot name="trigger" /><slot /></div>' },
  NDropdown: { template: '<div class="n-dropdown-stub" />' },
}

// ── EmptyState ────────────────────────────────────────────────────────────────
describe('EmptyState.vue', () => {
  it('renders the caption text and applies the size class', async () => {
    const EmptyState = (await import('@/components/EmptyState.vue')).default
    const w = mount(EmptyState, {
      props: { text: 'Ничего нет', size: 'small' },
      global: { stubs: naiveStubs },
    })
    expect(w.text()).toContain('Ничего нет')
    expect(w.find('.empty-state').classes()).toContain('small')
  })

  it('renders the default slot as an action', async () => {
    const EmptyState = (await import('@/components/EmptyState.vue')).default
    const w = mount(EmptyState, {
      props: { text: 'Пусто' },
      slots: { default: '<button class="act">Добавить</button>' },
      global: { stubs: naiveStubs },
    })
    expect(w.find('.act').exists()).toBe(true)
  })
})

// ── LoaderOverlay ─────────────────────────────────────────────────────────────
describe('LoaderOverlay.vue', () => {
  it('hides when show=false and reveals the first caption when show=true', async () => {
    const LoaderOverlay = (await import('@/components/LoaderOverlay.vue')).default
    const w = mount(LoaderOverlay, {
      props: { show: false, messages: ['Грузим…'] },
      global: { stubs: { ...naiveStubs, TesseraSpinner: { template: '<span class="spin" />' } } },
    })
    expect(w.find('.lo-overlay').exists()).toBe(false)

    await w.setProps({ show: true })
    expect(w.find('.lo-overlay').exists()).toBe(true)
    expect(w.text()).toContain('Грузим…')
  })

  it('adds the contained class when contained=true', async () => {
    const LoaderOverlay = (await import('@/components/LoaderOverlay.vue')).default
    const w = mount(LoaderOverlay, {
      props: { show: true, contained: true, messages: ['x'] },
      global: { stubs: { ...naiveStubs, TesseraSpinner: { template: '<span />' } } },
    })
    expect(w.find('.lo-overlay').classes()).toContain('contained')
  })
})

// ── BrandLogo ─────────────────────────────────────────────────────────────────
describe('BrandLogo.vue', () => {
  it('renders both the mark and wordmark by default with the aria label', async () => {
    const BrandLogo = (await import('@/components/BrandLogo.vue')).default
    const w = mount(BrandLogo)
    expect(w.find('.bl-mark').exists()).toBe(true)
    expect(w.find('.bl-word').exists()).toBe(true)
    expect(w.attributes('aria-label')).toBe('tessera')
  })

  it('renders the mark alone (solo) when wordmark=false', async () => {
    const BrandLogo = (await import('@/components/BrandLogo.vue')).default
    const w = mount(BrandLogo, { props: { wordmark: false } })
    expect(w.find('.bl-word').exists()).toBe(false)
    expect(w.find('.brand-logo').classes()).toContain('solo')
  })
})

// ── UserAvatar ────────────────────────────────────────────────────────────────
describe('UserAvatar.vue', () => {
  it('falls back to initials when there is no image source', async () => {
    const UserAvatar = (await import('@/components/UserAvatar.vue')).default
    const w = mount(UserAvatar, { props: { name: 'Иван Петров' } })
    // No userId/src → no <img>, initials shown instead.
    expect(w.find('img').exists()).toBe(false)
    expect(w.text().length).toBeGreaterThan(0)
  })
})

// ── ColumnHeader ──────────────────────────────────────────────────────────────
describe('ColumnHeader.vue', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('renders the column name and count', async () => {
    const ColumnHeader = (await import('@/components/ColumnHeader.vue')).default
    const w = mount(ColumnHeader, {
      props: { dcol: { key: 'c1', name: 'В работе', color: '' }, count: 3 },
      global: {
        stubs: {
          ...naiveStubs,
          // TesseraIcon pulls the icon pack; stub it to keep the smoke light.
          TesseraIcon: { template: '<span class="ticon-stub" />' },
        },
        // useMessage needs a message provider; supply a no-op via provide isn't
        // enough — instead mock it at the naive-ui import boundary below.
      },
    })
    expect(w.text()).toContain('В работе')
    expect(w.find('.count').text()).toBe('3')
  })

  it('emits toggle-collapse when the collapse button is clicked', async () => {
    const ColumnHeader = (await import('@/components/ColumnHeader.vue')).default
    const w = mount(ColumnHeader, {
      props: { dcol: { key: 'c2', name: 'Готово' }, count: 0 },
      global: {
        stubs: { ...naiveStubs, TesseraIcon: { template: '<span />' } },
      },
    })
    await w.find('.col-collapse').trigger('click')
    expect(w.emitted('toggle-collapse')).toBeTruthy()
  })
})

// ── ProjectCreateModal ────────────────────────────────────────────────────────
// The name a project is created with decides its URL address, which is assigned
// once — so the preview has to track what the server would derive, and the
// manual-address control has to stay manager-only (the server refuses others).
describe('ProjectCreateModal.vue', () => {
  beforeEach(() => setActivePinia(createPinia()))

  const mountModal = async (role) => {
    const { useWorkspacesStore } = await import('@/stores/workspaces')
    const store = useWorkspacesStore()
    store.list = [{ id: 'ws1', name: 'WS', my_role: role }]
    store.currentId = 'ws1'
    const ProjectCreateModal = (await import('@/components/ProjectCreateModal.vue')).default
    return mount(ProjectCreateModal, {
      props: { show: true },
      global: {
        // n-modal teleports its body out of the wrapper; keep it inline so the
        // rendered card is what `w.text()` sees.
        stubs: { ...naiveStubs, teleport: true },
      },
    })
  }

  it('previews the address the server would derive from the name', async () => {
    const w = await mountModal('owner')
    w.vm.name = 'Мой Проект!'
    await w.vm.$nextTick()
    expect(w.text()).toContain('/project/moy-proekt')
  })

  it('offers the manual-address control to managers only', async () => {
    const asOwner = await mountModal('owner')
    expect(asOwner.text()).toContain('Задать адрес вручную')
    const asMember = await mountModal('member')
    expect(asMember.text()).not.toContain('Задать адрес вручную')
  })
})
