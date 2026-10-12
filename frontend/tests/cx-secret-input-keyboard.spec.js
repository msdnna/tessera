// SecretInput, клавиатура (#2932): стиралка секрета — единственный способ стереть
// уже сохранённый секрет (backend читает пустое значение как «оставить как было»).
// Она нарисована как <span role="button">, то есть по умолчанию НЕ фокусируется
// табом и не активируется Enter/Space: клавиатурному пользователю фича недоступна
// целиком, обходного пути нет. Плюс при взведённой стирке сам n-input disabled —
// значит и отмена («вернуть») тоже мышь-онли. Эти тесты закрепляют контракт по
// образцу VersionBadge.vue (role + tabindex + Enter/Space).
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

describe('SecretInput — клавиатура', () => {
  it('стиралка достижима табом', () => {
    const w = mountIt({ value: '', stored: true })
    expect(eraser(w).attributes('tabindex')).toBe('0')
  })

  it('у стиралки есть доступное имя', () => {
    const w = mountIt({ value: '', stored: true })
    expect(eraser(w).attributes('aria-label')).toBeTruthy()
  })

  it('Enter взводит стирку так же, как клик', async () => {
    const w = mountIt({ value: '', stored: true, cleared: false })
    await eraser(w).trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:value')?.at(-1)).toEqual([''])
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([true])
  })

  it('Space взводит стирку так же, как клик', async () => {
    const w = mountIt({ value: '', stored: true, cleared: false })
    await eraser(w).trigger('keydown', { key: ' ' })
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([true])
  })

  it('Enter при взведённой стирке отменяет её', async () => {
    const w = mountIt({ value: '', stored: true, cleared: true })
    await eraser(w).trigger('keydown', { key: 'Enter' })
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([false])
  })

  it('Space при взведённой стирке отменяет её', async () => {
    const w = mountIt({ value: '', stored: true, cleared: true })
    await eraser(w).trigger('keydown', { key: ' ' })
    expect(w.emitted('update:cleared')?.at(-1)).toEqual([false])
  })

  it('Space не скроллит страницу (событие погашено)', () => {
    const w = mountIt({ value: '', stored: true })
    const ev = new KeyboardEvent('keydown', { key: ' ', bubbles: true, cancelable: true })
    eraser(w).element.dispatchEvent(ev)
    expect(ev.defaultPrevented).toBe(true)
  })

  it('прочие клавиши стиралку не трогают', async () => {
    const w = mountIt({ value: '', stored: true, cleared: false })
    await eraser(w).trigger('keydown', { key: 'a' })
    await eraser(w).trigger('keydown', { key: 'Tab' })
    expect(w.emitted('update:cleared')).toBeUndefined()
  })
})
