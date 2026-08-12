import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'

// Deleting a channel or a routing rule used to go through window.confirm. It now
// goes through an n-popconfirm like every other destructive action — so what's
// worth locking down is that the API call sits behind the confirmation and that
// the prompt names what's about to disappear.

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

// The real popconfirm teleports its body out and only mounts it once the popover
// opens — neither the prompt text nor the confirm button is reachable from the
// wrapper. `global.stubs` can't reach it either: the component is an import
// binding inside <script setup>, not a name-registered child. So swap it at the
// module boundary and leave the rest of Naive alone. The stub renders the
// trigger and the prompt side by side; clicking the trigger must NOT delete.
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    NPopconfirm: {
      name: 'NPopconfirm',
      props: ['positiveText', 'positiveButtonProps'],
      emits: ['positive-click'],
      template: `<div class="pc">
        <span class="pc-trigger"><slot name="trigger" /></span>
        <span class="pc-text"><slot /></span>
        <button class="pc-ok" @click="$emit('positive-click')" />
      </div>`,
    },
  }
})

const CHANNELS = [
  { id: 'ch1', type: 'email', label: 'Почта', enabled: true, config: {} },
  { id: 'ch2', type: 'webhook', label: '', enabled: true, config: {} },
]
const ROUTES = [
  { id: 'rt1', matcher: {}, channel_ids: ['ch1'], options: {}, enabled: true, position: 1 },
]

async function mountSettings() {
  chApi.list.mockResolvedValue({ data: CHANNELS })
  rtApi.list.mockResolvedValue({ data: ROUTES })
  prefsApi.get.mockResolvedValue({ data: {} })
  const NotificationSettings = (await import('@/components/NotificationSettings.vue')).default
  const w = mount(NotificationSettings)
  await flushPromises()
  return w
}

describe('NotificationSettings.vue — удаление через поповер', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('прячет корзину канала за поповер и не удаляет до подтверждения', async () => {
    const w = await mountSettings()
    const popconfirms = w.findAll('.pc')
    // one per channel + one per route
    expect(popconfirms).toHaveLength(CHANNELS.length + ROUTES.length)
    // the trash button lives inside the popconfirm's trigger slot, not standalone
    expect(popconfirms[0].find('.pc-trigger button').exists()).toBe(true)

    await popconfirms[0].find('.pc-trigger button').trigger('click')
    await flushPromises()
    expect(chApi.remove).not.toHaveBeenCalled()

    await popconfirms[0].find('.pc-ok').trigger('click')
    await flushPromises()
    expect(chApi.remove).toHaveBeenCalledWith('ch1')
  })

  it('называет канал в тексте подтверждения и предупреждает о правилах', async () => {
    const w = await mountSettings()
    const texts = w.findAll('.pc-text').map((t) => t.text())
    // ch1 is referenced by one route → the prompt has to say so
    expect(texts[0]).toContain('Почта')
    expect(texts[0]).toContain('в 1 правиле')
    // ch2 has no label → falls back to the type's display name, and no route hint
    expect(texts[1]).toContain('Webhook')
    expect(texts[1]).not.toContain('правил')
  })

  it('прячет корзину правила маршрутизации за тот же поповер', async () => {
    const w = await mountSettings()
    const routePc = w.findAll('.pc').at(-1)
    expect(routePc.find('.pc-text').text()).toContain('Удалить правило маршрутизации?')

    await routePc.find('.pc-trigger button').trigger('click')
    await flushPromises()
    expect(rtApi.remove).not.toHaveBeenCalled()

    await routePc.find('.pc-ok').trigger('click')
    await flushPromises()
    expect(rtApi.remove).toHaveBeenCalledWith('rt1')
  })
})
