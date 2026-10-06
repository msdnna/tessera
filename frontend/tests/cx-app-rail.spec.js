// The rail (v2, #2955): workspaces on the web; on desktop, a group per server
// — its head, then its workspaces — with the old server-rail behaviour (#2930):
// active marker, unread badge, faded signed-out server.
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { NPopconfirm } from 'naive-ui'

// The badge poller would otherwise reach the network on mount.
vi.mock('@/composables/useServerBadges', () => ({ useServerBadges: () => ({ poll: vi.fn() }) }))
const push = vi.fn()
vi.mock('vue-router', () => ({ useRoute: () => ({ params: {} }), useRouter: () => ({ push }) }))
vi.mock('naive-ui', async (importOriginal) => ({
  ...(await importOriginal()),
  useMessage: () => ({ error: vi.fn(), success: vi.fn(), warning: vi.fn() }),
}))

import AppRail from '@/components/rail/AppRail.vue'
import { useServersStore } from '@/stores/servers'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { addServer, setActiveServerId, listServers } from '@/utils/servers'

let wrapper = null
const WS = [
  { id: 'w1', name: 'Тессера Дев', my_role: 'owner', color: '#7c5cff' },
  { id: 'w2', name: 'Сайт', my_role: 'member', icon: 'rocket' },
]

function mountRail() {
  wrapper = mount(AppRail, {
    global: {
      stubs: {
        NDropdown: true,
        NModal: true,
        ServerEditModal: true,
        WorkspaceAppearanceModal: true,
        ConfirmByName: true,
      },
    },
  })
  return wrapper
}
const tiles = (sel = '') => wrapper.findAll(`button.rt${sel}`)

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  push.mockClear()
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  delete window.__TAURI_INTERNALS__
  localStorage.clear()
})

describe('AppRail on the web', () => {
  it('draws a tile per workspace and marks the current one', () => {
    const ws = useWorkspacesStore()
    ws.list = WS
    ws.currentId = 'w1'
    mountRail()
    const all = tiles()
    expect(all).toHaveLength(2)
    expect(all[0].classes()).toContain('active')
    expect(all[0].text()).toContain('ТД')
    // An icon replaces the initials.
    expect(all[1].findComponent({ name: 'ProjectIcon' }).exists()).toBe(true)
    expect(wrapper.find('[data-tour="ws-switch"]').exists()).toBe(true)
  })

  it('switches workspace on click', async () => {
    const ws = useWorkspacesStore()
    ws.list = WS
    ws.currentId = 'w1'
    useAuthStore().token = 'x'
    ws.selectWorkspace = vi.fn()
    mountRail()
    await wrapper.find('[data-ws="w2"]').trigger('click')
    expect(ws.selectWorkspace).toHaveBeenCalledWith('w2')
  })
})

describe('AppRail on desktop', () => {
  function twoServers() {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'Работа', url: 'https://a.example.com' })
    const b = addServer({ name: 'Дом', url: 'https://b.example.com' })
    setActiveServerId(a.id)
    const store = useServersStore()
    store.reload()
    return { a, b, store }
  }

  it('groups workspaces under their server', () => {
    const { a, b, store } = twoServers()
    const ws = useWorkspacesStore()
    ws.list = WS
    ws.currentId = 'w1'
    store.setWorkspaces(b.id, [{ id: 'h1', name: 'Дача' }])
    mountRail()

    const groups = wrapper.findAll('.ar-group')
    expect(groups).toHaveLength(2)
    expect(groups[0].attributes('data-server')).toBe(a.id)
    // Head + the live list of the active server; head + the cached one of the other.
    expect(groups[0].findAll('button.rt')).toHaveLength(3)
    expect(groups[1].findAll('button.rt')).toHaveLength(2)
    expect(groups[1].find('[data-ws="h1"]').text()).toContain('ДА')
  })

  it('lands in another server’s workspace through a server switch', async () => {
    const { b, store } = twoServers()
    store.setWorkspaces(b.id, [{ id: 'h1', name: 'Дача' }])
    store.switchTo = vi.fn()
    mountRail()
    await wrapper.find('[data-ws="h1"]').trigger('click')
    expect(localStorage.getItem(`tessera_ws::${b.id}`)).toBe('h1')
    expect(store.switchTo).toHaveBeenCalledWith(b.id)
  })

  it('badges an inactive server and fades a signed-out one', () => {
    const { b, store } = twoServers()
    store.setBadge(b.id, 250)
    store.setSignedOut(b.id, true)
    mountRail()
    const head = wrapper.findAll('.ar-group')[1].find('button.rt.small')
    expect(head.find('.rt-badge').text()).toBe('99+')
    expect(head.classes()).toContain('muted')
  })

  // Removing a profile purges its refresh token and, for the last one, replaces
  // it with a fresh profile aimed at the public default — the address of a
  // self-hosted server is gone with no undo. The menu item sits one row under
  // «Изменить»; it must not fire on the slip (#2932).
  describe('removing a server asks first', () => {
    async function pickRemove() {
      await wrapper.findAll('.ar-group')[1].find('button.rt.small').trigger('contextmenu')
      // The context menu is the manual dropdown (the «+» one opens on click).
      const menu = wrapper
        .findAllComponents({ name: 'Dropdown' })
        .find((d) => d.props('trigger') === 'manual')
      menu.vm.$emit('select', 'server-remove')
      await nextTick()
    }

    it('keeps the profile until the confirm is answered', async () => {
      const { a, b } = twoServers()
      mountRail()
      await pickRemove()
      expect(listServers().map((s) => s.id)).toEqual([a.id, b.id])
      expect(wrapper.findComponent(NPopconfirm).props('show')).toBe(true)
    })

    it('drops the profile once confirmed', async () => {
      const { a } = twoServers()
      mountRail()
      await pickRemove()
      wrapper.findComponent(NPopconfirm).vm.$emit('positive-click')
      await nextTick()
      expect(listServers().map((s) => s.id)).toEqual([a.id])
    })

    it('leaves the profile alone when the confirm is dismissed', async () => {
      const { a, b } = twoServers()
      mountRail()
      await pickRemove()
      wrapper.findComponent(NPopconfirm).vm.$emit('clickoutside')
      await nextTick()
      expect(listServers().map((s) => s.id)).toEqual([a.id, b.id])
      expect(wrapper.findComponent(NPopconfirm).props('show')).toBe(false)
    })
  })

  it('keeps each server’s workspaces cached for the next start', () => {
    const { b, store } = twoServers()
    store.setWorkspaces(b.id, [{ id: 'h1', name: 'Дача', my_role: 'owner', extra: 1 }])
    setActivePinia(createPinia())
    const fresh = useServersStore()
    expect(fresh.workspaces[b.id]).toEqual([
      { id: 'h1', name: 'Дача', name_key: null, color: '', icon: '', icon_mode: 'badge' },
    ])
  })
})
