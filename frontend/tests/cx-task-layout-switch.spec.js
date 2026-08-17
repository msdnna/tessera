// TaskLayoutSwitch (#2716): the header control that picks where a task is shown.
// It is deliberately a controlled input — it never touches localStorage itself, so
// the only contract to pin is "renders the three modes, marks the active one, emits
// the picked value". The menu lives in a popover teleported to <body>, so the tests
// open it for real and assert against the document.
import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import * as naive from 'naive-ui'
import TaskLayoutSwitch from '@/components/TaskLayoutSwitch.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
}

let wrapper = null
async function open(value) {
  wrapper = mount(TaskLayoutSwitch, {
    props: { value },
    attachTo: document.body,
    global: { stubs },
  })
  await wrapper.find('[data-testid="task-layout-trigger"]').trigger('click')
  await nextTick()
  await nextTick()
  return wrapper
}
const item = (v) => document.querySelector(`[data-testid="task-layout-${v}"]`)
const active = (v) => (item(v)?.className || '').includes('n-button--primary-type')

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('TaskLayoutSwitch', () => {
  it('offers all three layouts', async () => {
    await open('modal')
    for (const v of ['modal', 'fullscreen', 'sidebar']) expect(item(v)).toBeTruthy()
  })

  it('marks the active layout and only that one', async () => {
    await open('sidebar')
    expect(active('sidebar')).toBe(true)
    expect(active('modal')).toBe(false)
    expect(active('fullscreen')).toBe(false)
  })

  it('emits the picked layout', async () => {
    const w = await open('modal')
    item('fullscreen').click()
    await nextTick()
    expect(w.emitted('update:value')?.at(-1)).toEqual(['fullscreen'])
  })

  it('treats an unknown value as the modal rather than marking nothing', async () => {
    await open('drawer')
    expect(active('modal')).toBe(true)
  })
})
