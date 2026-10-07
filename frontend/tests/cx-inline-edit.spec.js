// InlineEdit — tier 0 of the v2 interaction model (#2969): the field reports ↵,
// ⇧↵, Tab, Esc and click-away; the owner decides what they mean.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import InlineEdit from '@/components/kit/InlineEdit.vue'

function make(props = {}) {
  return mount(InlineEdit, { props: { modelValue: 'Бэклог', ...props }, attachTo: document.body })
}

describe('InlineEdit', () => {
  it('↵ commits once — the blur after it is not a second save', async () => {
    const w = make()
    const input = w.find('input')
    await input.trigger('keydown', { key: 'Enter' })
    await input.trigger('blur')
    expect(w.emitted('commit')).toHaveLength(1)
    expect(w.emitted('commit')[0][1]).toEqual({ via: 'enter', shift: false })
    w.unmount()
  })

  it('⇧↵ is reported as such (create and add another row)', async () => {
    const w = make({ hint: 'create' })
    await w.find('input').trigger('keydown', { key: 'Enter', shiftKey: true })
    expect(w.emitted('commit')[0][1].shift).toBe(true)
    w.unmount()
  })

  it('Esc cancels and swallows the following blur', async () => {
    const w = make()
    const input = w.find('input')
    await input.trigger('keydown', { key: 'Escape' })
    await input.trigger('blur')
    expect(w.emitted('cancel')).toHaveLength(1)
    expect(w.emitted('commit')).toBeUndefined()
    w.unmount()
  })

  it('click-away commits', async () => {
    const w = make()
    await w.find('input').trigger('blur')
    expect(w.emitted('commit')[0][1].via).toBe('blur')
    w.unmount()
  })

  it('Tab commits only where it means something (new board, column)', async () => {
    const plain = make()
    await plain.find('input').trigger('keydown', { key: 'Tab' })
    expect(plain.emitted('commit')).toBeUndefined()
    plain.unmount()
    const board = make({ hint: 'createView' })
    await board.find('input').trigger('keydown', { key: 'Tab' })
    expect(board.emitted('commit')[0][1].via).toBe('tab')
    board.unmount()
  })

  it('typing after a refused ↵ makes click-away a save attempt again', async () => {
    const w = make({ error: 'Доска «Баги» уже есть в проекте' })
    const input = w.find('input')
    await input.trigger('keydown', { key: 'Enter' })
    await input.setValue('Баги 2')
    await input.trigger('blur')
    expect(w.emitted('commit')).toHaveLength(2)
    expect(w.text()).toContain('Доска «Баги» уже есть в проекте')
    w.unmount()
  })
})
