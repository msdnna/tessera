import { describe, it, expect, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { zindexable } from 'vdirs'
import ActionMenu from '@/components/kit/ActionMenu.vue'
import PopoverForm from '@/components/kit/PopoverForm.vue'

// #3063 / #3034: kit layers used to sit on constant z-indexes (2050…2200) while
// Naive's layers take 2000, 2001, … from vdirs' manager. After a long session a
// modal outgrew the constants and the role menu opened behind the members
// window; a date panel opened under the PopoverForm it belonged to. Kit layers
// now take their z-index from the same manager, so the last opened is on top.

// A Naive layer opened after `n` others that are still registered.
function naiveLayer(n) {
  const els = []
  for (let i = 0; i <= n; i++) {
    const el = document.createElement('div')
    zindexable.mounted(el, { value: { enabled: true } })
    els.push(el)
  }
  return {
    z: Number(els.at(-1).style.zIndex),
    release: () => els.forEach((el) => zindexable.unmounted(el, { value: {} })),
  }
}

let release = () => {}
afterEach(() => {
  release()
  document.body.innerHTML = ''
})

describe('kit layers share the Naive z-index manager', () => {
  it('a menu opened from a long-lived modal is above it', async () => {
    const modal = naiveLayer(300)
    release = modal.release
    expect(modal.z).toBeGreaterThan(2200)
    const w = mount(ActionMenu, {
      props: { show: false, items: [{ key: 'a', label: 'A' }], anchor: () => ({ x: 1, y: 1 }) },
      attachTo: document.body,
    })
    await w.setProps({ show: true })
    await nextTick()
    const menu = document.body.querySelector('[data-testid="action-menu"]')
    expect(Number(menu.style.zIndex)).toBeGreaterThan(modal.z)
    w.unmount()
  })

  it('a Naive panel opened from a popover form is above the form', async () => {
    const w = mount(PopoverForm, {
      props: { show: false, title: 'Новая встреча', anchor: () => ({ x: 1, y: 1 }) },
      attachTo: document.body,
    })
    await w.setProps({ show: true })
    await nextTick()
    const form = document.body.querySelector('.pf')
    expect(form.style.zIndex).not.toBe('')
    const panel = naiveLayer(0)
    release = panel.release
    expect(panel.z).toBeGreaterThan(Number(form.style.zIndex))
    w.unmount()
  })
})
