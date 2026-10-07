// PopoverForm — tier 1 of the v2 interaction model (#2969).
import { describe, it, expect, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import PopoverForm from '@/components/kit/PopoverForm.vue'

async function open(props = {}) {
  const w = mount(PopoverForm, {
    props: { show: false, title: 'Новая доска', primary: 'Создать', anchor: () => ({ x: 0, y: 0 }), ...props },
    slots: { default: '<input class="kf-input" data-testid="name" />' },
    attachTo: document.body,
  })
  await w.setProps({ show: true })
  await nextTick()
  await nextTick()
  return w
}
const field = () => document.body.querySelector('[data-testid="name"]')
const key = (k, extra = {}) =>
  field().dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra }))

afterEach(() => {
  document.body.innerHTML = ''
})

describe('PopoverForm', () => {
  it('↵ in a field submits, ⌘↵ submits and opens', async () => {
    const w = await open()
    key('Enter')
    key('Enter', { metaKey: true })
    expect(w.emitted('submit')).toEqual([[{ open: false }], [{ open: true }]])
    w.unmount()
  })

  it('a disabled form does not submit', async () => {
    const w = await open({ disabled: true })
    key('Enter')
    expect(w.emitted('submit')).toBeUndefined()
    w.unmount()
  })

  it('Esc closes; while busy it asks to abort instead', async () => {
    const w = await open()
    key('Escape')
    expect(w.emitted('update:show')).toEqual([[false]])
    w.unmount()
    const busy = await open({ busy: true })
    key('Escape')
    expect(busy.emitted('abort')).toHaveLength(1)
    expect(busy.emitted('update:show')).toBeUndefined()
    busy.unmount()
  })

  it('⌥↵ asks for the free variant', async () => {
    const w = await open()
    key('Enter', { altKey: true })
    expect(w.emitted('suggest')).toHaveLength(1)
    expect(w.emitted('submit')).toBeUndefined()
    w.unmount()
  })

  it('focuses the first field on open', async () => {
    const w = await open()
    expect(document.activeElement).toBe(field())
    w.unmount()
  })
})
