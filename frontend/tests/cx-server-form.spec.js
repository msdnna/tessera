// The server form's «Удалить сервер» (#2932; a popover form since #2969).
// Removing purges the profile's session namespace — for the last profile the
// typed address is gone from the app — so it takes two presses, the second
// within four seconds.
import { setActivePinia, createPinia } from 'pinia'
import { nextTick } from 'vue'
import { beforeEach, afterEach, describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ServerFormPopover from '@/components/rail/ServerFormPopover.vue'

let wrapper = null

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  localStorage.clear()
  document.body.innerHTML = ''
  vi.useRealTimers()
})

async function mountForm(server) {
  wrapper = mount(ServerFormPopover, {
    props: { show: false, server, anchor: () => ({ x: 0, y: 0 }) },
    // The colour picker wants a message provider and is not what these cases are about.
    global: { stubs: { IconColorPicker: true } },
    attachTo: document.body,
  })
  await wrapper.setProps({ show: true })
  await nextTick()
  return wrapper
}
const removeBtn = () => document.querySelector('[data-testid="server-form-remove"]')
const SERVER = { id: 'srv-1', name: 'A', url: 'https://a.example.com', color: '#7c5cff' }

describe('ServerFormPopover remove (#2932)', () => {
  it('does not emit remove on the first press', async () => {
    await mountForm(SERVER)
    removeBtn().click()
    await nextTick()
    expect(wrapper.emitted('remove')).toBeUndefined()
    expect(removeBtn().classList.contains('armed')).toBe(true)
  })

  it('emits remove on the second press', async () => {
    await mountForm(SERVER)
    removeBtn().click()
    await nextTick()
    removeBtn().click()
    await nextTick()
    expect(wrapper.emitted('remove')).toEqual([['srv-1']])
  })

  it('disarms after four seconds', async () => {
    vi.useFakeTimers()
    await mountForm(SERVER)
    removeBtn().click()
    await nextTick()
    vi.advanceTimersByTime(4100)
    await nextTick()
    expect(removeBtn().classList.contains('armed')).toBe(false)
  })

  it('offers no remove button when adding a server', async () => {
    await mountForm(null)
    expect(removeBtn()).toBeNull()
  })
})
