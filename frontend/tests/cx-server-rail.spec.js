import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { NPopconfirm } from 'naive-ui'

// The rail's badge poller would otherwise reach the network on mount.
vi.mock('@/composables/useServerBadges', () => ({ useServerBadges: () => ({ poll: vi.fn() }) }))

import ServerRail from '@/components/ServerRail.vue'
import ServerEditModal from '@/components/ServerEditModal.vue'
import { useServersStore } from '@/stores/servers'
import { addServer, setActiveServerId, listServers } from '@/utils/servers'

let wrapper = null

function mountRail() {
  wrapper = mount(ServerRail, {
    global: { stubs: { NTooltip: false, NDropdown: true, NIcon: true, ServerEditModal: true } },
  })
  return wrapper
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  delete window.__TAURI_INTERNALS__
  localStorage.clear()
})

describe('ServerRail (#2930)', () => {
  it('does not render in a browser', () => {
    // The web build has exactly one server — the origin serving the page — so
    // the whole feature must be invisible there.
    expect(mountRail().find('[data-testid="server-rail"]').exists()).toBe(false)
  })

  it('draws a tile per server and marks the active one', () => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'Работа', url: 'https://a.example.com' })
    addServer({ name: 'Дом', url: 'https://b.example.com' })
    setActiveServerId(a.id)

    const store = useServersStore()
    store.reload()
    const tiles = mountRail().findAll('.sr-tile:not(.sr-add)')

    expect(tiles).toHaveLength(2)
    expect(tiles[0].classes()).toContain('active')
    expect(tiles[1].classes()).not.toContain('active')
    // Initials over the profile colour — the first iteration's tile face.
    expect(tiles[0].text()).toContain('РА')
    expect(wrapper.find('[data-testid="server-rail-add"]').exists()).toBe(true)
  })

  it('renders a chosen icon instead of the initials (#2930 rework)', () => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'Работа', url: 'https://a.example.com', icon: 'rocket' })
    setActiveServerId(a.id)

    const store = useServersStore()
    store.reload()
    const tile = mountRail().findAll('.sr-tile:not(.sr-add)')[0]

    // With an icon set the face is the glyph (ProjectIcon), not the initials.
    expect(tile.findComponent({ name: 'ProjectIcon' }).exists()).toBe(true)
    expect(tile.text()).not.toContain('РА')
  })

  it('badges an inactive server with its unread count', () => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'A', url: 'https://a.example.com' })
    const b = addServer({ name: 'B', url: 'https://b.example.com' })
    setActiveServerId(a.id)

    const store = useServersStore()
    store.reload()
    store.setBadge(b.id, 3)
    mountRail()

    const tiles = wrapper.findAll('.sr-tile:not(.sr-add)')
    expect(tiles[0].find('.sr-badge').exists()).toBe(false)
    expect(tiles[1].find('.sr-badge').text()).toBe('3')
  })

  it('caps the badge at 99+', () => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'A', url: 'https://a.example.com' })
    const b = addServer({ name: 'B', url: 'https://b.example.com' })
    setActiveServerId(a.id)

    const store = useServersStore()
    store.reload()
    store.setBadge(b.id, 250)
    mountRail()

    expect(wrapper.findAll('.sr-tile:not(.sr-add)')[1].find('.sr-badge').text()).toBe('99+')
  })

  it('fades a server whose session has ended', () => {
    window.__TAURI_INTERNALS__ = {}
    localStorage.setItem('tessera_servers', '[]')
    const a = addServer({ name: 'A', url: 'https://a.example.com' })
    const b = addServer({ name: 'B', url: 'https://b.example.com' })
    setActiveServerId(a.id)

    const store = useServersStore()
    store.reload()
    store.setSignedOut(b.id, true)
    mountRail()

    expect(wrapper.findAll('.sr-tile:not(.sr-add)')[1].classes()).toContain('out')
  })

  // Removing a profile purges its refresh token and, for the last one, replaces
  // it with a fresh profile aimed at the public default — so the address of a
  // self-hosted server is gone from the app with no undo. The menu item sits one
  // row under «Изменить»; it must not fire on the slip.
  describe('removing a server asks first', () => {
    // The menu itself is left real here (it paints into a teleport, so there is
    // nothing to click in the wrapper): the test speaks to it through the
    // `select` event the rail actually listens on.
    function mountWithMenu() {
      wrapper = mount(ServerRail, {
        global: { stubs: { NTooltip: false, NIcon: true, ServerEditModal: true } },
      })
      return wrapper
    }

    function twoServers() {
      window.__TAURI_INTERNALS__ = {}
      localStorage.setItem('tessera_servers', '[]')
      const a = addServer({ name: 'A', url: 'https://a.example.com' })
      const b = addServer({ name: 'B', url: 'https://b.example.com' })
      setActiveServerId(a.id)
      useServersStore().reload()
      return { a, b }
    }

    async function pickRemove(tileIndex) {
      const tiles = wrapper.findAll('.sr-tile:not(.sr-add)')
      await tiles[tileIndex].trigger('contextmenu')
      wrapper.findComponent({ name: 'Dropdown' }).vm.$emit('select', 'remove')
      await nextTick()
    }

    it('keeps the profile until the confirm is answered', async () => {
      const { a, b } = twoServers()
      mountWithMenu()

      await pickRemove(1)

      // The menu closed, the confirm opened — and the registry is untouched.
      expect(listServers().map((s) => s.id)).toEqual([a.id, b.id])
      expect(wrapper.findComponent(NPopconfirm).props('show')).toBe(true)
    })

    it('drops the profile once confirmed', async () => {
      const { a } = twoServers()
      mountWithMenu()

      await pickRemove(1)
      wrapper.findComponent(NPopconfirm).vm.$emit('positive-click')
      await nextTick()

      expect(listServers().map((s) => s.id)).toEqual([a.id])
    })

    it('leaves the profile alone when the confirm is dismissed', async () => {
      const { a, b } = twoServers()
      mountWithMenu()

      await pickRemove(1)
      wrapper.findComponent(NPopconfirm).vm.$emit('clickoutside')
      await nextTick()

      expect(listServers().map((s) => s.id)).toEqual([a.id, b.id])
      expect(wrapper.findComponent(NPopconfirm).props('show')).toBe(false)
    })
  })
})

describe('ServerEditModal remove (#2932)', () => {
  // The modal's «Удалить сервер» sits next to «Сохранить» in the same footer and
  // destroys the same thing, so it asks over the button rather than acting on it.
  // The modal's body is teleported out of the wrapper, so the footer is read off
  // the document; the colour picker is stubbed because it wants a message
  // provider and is not what these cases are about.
  function mountModal(server) {
    wrapper = mount(ServerEditModal, {
      props: { show: true, server },
      global: { stubs: { IconColorPicker: true } },
    })
    return wrapper
  }

  it('does not emit remove on the bare click', async () => {
    const server = { id: 'srv-1', name: 'A', url: 'https://a.example.com', color: '#7c5cff' }
    mountModal(server)

    document.querySelector('.se-actions button').click()
    await nextTick()

    // The button is the confirm's trigger, not the action itself.
    expect(wrapper.emitted('remove')).toBeUndefined()
    expect(wrapper.findComponent(NPopconfirm).exists()).toBe(true)
  })

  it('emits remove once the confirm is answered', async () => {
    const server = { id: 'srv-1', name: 'A', url: 'https://a.example.com', color: '#7c5cff' }
    mountModal(server)

    wrapper.findComponent(NPopconfirm).vm.$emit('positive-click')
    await nextTick()

    expect(wrapper.emitted('remove')).toEqual([['srv-1']])
  })

  it('offers no remove button when adding a server', () => {
    mountModal(null)

    expect(wrapper.findComponent(NPopconfirm).exists()).toBe(false)
  })
})
