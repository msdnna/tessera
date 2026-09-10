import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// The rail's badge poller would otherwise reach the network on mount.
vi.mock('@/composables/useServerBadges', () => ({ useServerBadges: () => ({ poll: vi.fn() }) }))

import ServerRail from '@/components/ServerRail.vue'
import { useServersStore } from '@/stores/servers'
import { addServer, setActiveServerId } from '@/utils/servers'

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
})
