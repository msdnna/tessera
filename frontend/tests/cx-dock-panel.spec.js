// DockPanel (#2985) — the panel docked inside the content area: a header with
// the title and a close button, Esc closes it, the footer lists key hints, and
// on a phone the same content becomes a bottom sheet.
import { describe, it, expect, afterEach, vi } from 'vitest'
import { nextTick, h } from 'vue'
import { mount } from '@vue/test-utils'
import DockPanel from '@/components/kit/DockPanel.vue'

function mq(matches) {
  window.matchMedia = vi.fn().mockReturnValue({
    matches,
    addEventListener() {},
    removeEventListener() {},
  })
}

afterEach(() => {
  delete window.matchMedia
  document.body.innerHTML = ''
})

function mountPanel(props = {}) {
  return mount(DockPanel, {
    props: { show: true, title: 'Вид доски', subtitle: 'Канбан', ...props },
    slots: { default: () => h('div', { class: 'body-probe' }, 'тело') },
    attachTo: document.body,
  })
}

describe('DockPanel', () => {
  it('draws a docked aside with the title, subtitle and body', async () => {
    mq(false)
    const w = mountPanel({ hints: [{ caps: ['↑↓'], text: 'раздел' }] })
    await nextTick()
    const aside = w.find('aside[data-testid="dock-panel"]')
    expect(aside.exists()).toBe(true)
    expect(aside.classes()).toContain('pop')
    expect(aside.text()).toContain('Вид доски')
    expect(aside.text()).toContain('Канбан')
    expect(aside.find('.body-probe').exists()).toBe(true)
    expect(aside.find('.dock-foot').text()).toContain('раздел')
    expect(aside.attributes('style')).toContain('width: 340px')
    w.unmount()
  })

  it('closes on its button and on Esc inside it', async () => {
    mq(false)
    const w = mountPanel()
    await nextTick()
    await w.find('[data-testid="dock-close"]').trigger('click')
    expect(w.emitted('update:show')).toEqual([[false]])
    await w.find('aside').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('update:show')).toHaveLength(2)
    w.unmount()
  })

  it('the sidebar variant drops the shadowed pop look', async () => {
    mq(false)
    const w = mountPanel({ variant: 'sidebar' })
    await nextTick()
    expect(w.find('aside').classes()).toContain('sidebar')
    w.unmount()
  })

  it('becomes a bottom sheet on a phone, without key hints', async () => {
    mq(true)
    const w = mountPanel({ hints: [{ caps: ['↑↓'], text: 'раздел' }] })
    await nextTick()
    await nextTick()
    expect(w.find('aside').exists()).toBe(false)
    const sheet = document.body.querySelector('[data-testid="dock-panel"]')
    expect(sheet).not.toBeNull()
    expect(sheet.textContent).toContain('Вид доски')
    expect(sheet.querySelector('.body-probe')).not.toBeNull()
    expect(sheet.textContent).not.toContain('раздел')
    w.unmount()
  })
})
