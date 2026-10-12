import { describe, it, expect, afterEach, beforeEach } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { i18n } from '@/i18n'
import DateField from '@/components/kit/DateField.vue'

// The v2 date field (#3034) keeps NDatePicker's value contract — epoch ms of a
// local date (and time) — so hosts swap the tag and nothing else.

let w = null
function field(props) {
  w = mount(DateField, { props, attachTo: document.body, global: { plugins: [i18n] } })
  return w
}
const day = (n) =>
  [...document.querySelectorAll('[data-testid="date-field-cal"] button')].find(
    (b) => b.textContent.trim() === String(n),
  )
const tick = async () => {
  await nextTick()
  await nextTick()
}

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
})

describe('DateField', () => {
  it('date: a pick emits the local midnight and closes', async () => {
    field({ value: null })
    await w.find('[data-testid="date-field"]').trigger('click')
    await tick()
    day(17).click()
    await tick()
    const [ms] = w.emitted('update:value').at(-1)
    const d = new Date(ms)
    expect([d.getDate(), d.getHours(), d.getMinutes()]).toEqual([17, 0, 0])
    expect(document.querySelector('[data-testid="date-field-cal"]')).toBeNull()
  })

  it('datetime: a day takes the default time, a chip changes it', async () => {
    field({ value: null, type: 'datetime' })
    await w.find('[data-testid="date-field"]').trigger('click')
    await tick()
    day(16).click()
    await tick()
    let d = new Date(w.emitted('update:value').at(-1)[0])
    expect([d.getDate(), d.getHours()]).toEqual([16, 9])
    await w.setProps({ value: d.getTime() })
    document.querySelector('.df-chip[data-time="18:00"]').click()
    await tick()
    d = new Date(w.emitted('update:value').at(-1)[0])
    expect([d.getDate(), d.getHours()]).toEqual([16, 18])
  })

  it('Esc closes without touching the value; the cross clears it', async () => {
    field({ value: new Date(2026, 6, 15).getTime() })
    await w.find('[data-testid="date-field"]').trigger('click')
    await tick()
    document
      .querySelector('.df')
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await tick()
    expect(document.querySelector('[data-testid="date-field-cal"]')).toBeNull()
    expect(w.emitted('update:value')).toBeUndefined()
    await w.find('[data-testid="date-field-clear"]').trigger('click')
    expect(w.emitted('update:value').at(-1)).toEqual([null])
  })
})
