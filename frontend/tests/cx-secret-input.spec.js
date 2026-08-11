// SecretInput (#2691): a password field that can *erase* an already-stored
// secret. The backend reads an empty value as "keep", so the eraser must emit an
// explicit `cleared` flag — and only when a secret is actually stored, else
// "erasing" a blank field is a silent no-op. These tests pin that contract.
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import * as naive from 'naive-ui'
import SecretInput from '@/components/SecretInput.vue'

const stubs = {
  ...naive,
  ...Object.fromEntries(Object.entries(naive).map(([k, v]) => ['N' + k, v])),
}

const mountIt = (props = {}) => mount(SecretInput, { props, global: { stubs } })
const eraser = (w) => w.find('.secret-eraser')

describe('SecretInput', () => {
  it('hides the eraser when nothing is stored', () => {
    const w = mountIt({ value: '', stored: false })
    expect(eraser(w).exists()).toBe(false)
  })

  it('hides the eraser when not resettable, even if stored', () => {
    const w = mountIt({ value: '', stored: true, resettable: false })
    expect(eraser(w).exists()).toBe(false)
  })

  it('shows the eraser when a secret is stored', () => {
    const w = mountIt({ value: '', stored: true })
    expect(eraser(w).exists()).toBe(true)
  })

  it('arms the erase on click: clears the value and sets cleared', async () => {
    const w = mountIt({ value: '', stored: true, cleared: false })
    await eraser(w).trigger('click')
    expect(w.emitted('update:value')?.at(-1)).toEqual([''])
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([true])
  })

  it('undoes the erase on a second click', async () => {
    const w = mountIt({ value: '', stored: true, cleared: true })
    await eraser(w).trigger('click')
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([false])
  })

  it('typing while armed cancels the erase (replace beats erase)', async () => {
    const w = mountIt({ value: '', stored: true, cleared: true })
    w.findComponent(naive.NInput).vm.$emit('update:value', 'new-secret')
    await w.vm.$nextTick()
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([false])
    expect(w.emitted('update:value')?.at(-1)).toEqual(['new-secret'])
  })
})
