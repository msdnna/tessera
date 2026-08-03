import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import TagPill from '@/components/TagPill.vue'

const names = { 'effort::': 'Сложность' }

function pill(props) {
  return mount(TagPill, { props })
}

describe('TagPill.vue', () => {
  beforeEach(() => {
    // The pill reads the theme store to clamp the tag colour for the active theme.
    setActivePinia(createPinia())
  })

  it('renders a scoped tag as two segments with the friendly scope name', () => {
    const w = pill({ tag: { name: 'effort::small', color: '#7c5cff' }, prefixNames: names })
    expect(w.find('.tp-scope').text()).toBe('Сложность')
    expect(w.find('.tp-name').text()).toBe('small')
    expect(w.find('.tpill').classes()).toContain('scoped')
  })

  it('falls back to the raw prefix when no friendly name is configured', () => {
    const w = pill({ tag: { name: 'area::backend' }, prefixNames: names })
    expect(w.find('.tp-scope').text()).toBe('area')
    expect(w.find('.tp-name').text()).toBe('backend')
  })

  it('renders an unscoped tag as a single segment', () => {
    const w = pill({ tag: { name: 'urgent' } })
    expect(w.find('.tp-scope').exists()).toBe(false)
    expect(w.find('.tp-name').text()).toBe('urgent')
    expect(w.find('.tpill').classes()).not.toContain('scoped')
  })

  it('suppresses the scope segment when scopeMode is "hide"', () => {
    const w = pill({
      tag: { name: 'effort::small' },
      prefixNames: names,
      scopeMode: 'hide',
    })
    expect(w.find('.tp-scope').exists()).toBe(false)
    expect(w.find('.tp-name').text()).toBe('small')
    // No divider without a scope segment to divide from.
    expect(w.find('.tpill').classes()).not.toContain('scoped')
  })

  it('accepts a bare name string plus a colour', () => {
    const w = pill({ tag: 'effort::small', color: '#123456', prefixNames: names })
    expect(w.find('.tp-scope').text()).toBe('Сложность')
    expect(w.find('.tp-name').text()).toBe('small')
  })

  it('titles the pill with the full raw name so the source value stays visible', () => {
    const w = pill({ tag: { name: 'effort::small' }, prefixNames: names })
    expect(w.find('.tpill').attributes('title')).toBe('effort::small')
  })

  it('paints the outline variant with a gradient border-box and gradient text', () => {
    const w = pill({ tag: { name: 'effort::small', color: '#7c5cff' }, variant: 'outline' })
    expect(w.find('.tpill').attributes('style')).toContain('border-box')
    expect(w.find('.tp-name').classes()).toContain('accent-grad-text')
  })

  it('leaves colour to the call site for the inherit variant', () => {
    const w = pill({ tag: { name: 'effort::small', color: '#7c5cff' }, variant: 'inherit' })
    const style = w.find('.tpill').attributes('style') || ''
    expect(style).not.toContain('background')
    expect(style).toContain('currentColor')
    expect(w.find('.tp-name').classes()).not.toContain('accent-grad-text')
  })
})
