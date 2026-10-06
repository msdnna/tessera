// ServerEditModal's «Удалить сервер» (#2932). Moved here from the old
// cx-server-rail spec when the v2 rail (#2955) replaced ServerRail.
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { beforeEach, afterEach, describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { NPopconfirm } from 'naive-ui'

import ServerEditModal from '@/components/ServerEditModal.vue'

let wrapper = null

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  localStorage.clear()
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
