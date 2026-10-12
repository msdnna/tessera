import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { nextTick } from 'vue'

const apiMock = vi.hoisted(() => ({
  acknowledgements: { ack: vi.fn(() => Promise.resolve()), list: vi.fn() },
}))
vi.mock('@/api', () => apiMock)

import TourOverlay from '@/components/TourOverlay.vue'
import { useTourStore } from '@/stores/tour'
import { setI18nLocale } from '@/i18n'

// The guide walks a brand-new board, and the four columns it points at are the
// seeded ones — whose captions are drawn from the reader's catalogue (#2800),
// not from the Russian names the server stores. So the wording may not quote a
// column literally: an English reader told to use the «К работе» column is
// looking at a board that says "To do" (#2932).

function anchor(key) {
  const el = document.createElement('button')
  el.setAttribute('data-tour', key)
  el.getBoundingClientRect = () => ({
    left: 100,
    top: 60,
    width: 40,
    height: 24,
    right: 140,
    bottom: 84,
  })
  document.body.appendChild(el)
  return el
}

let wrapper = null

async function popText(stepId) {
  const tour = useTourStore()
  anchor('a')
  tour.start([{ id: stepId, anchor: 'a' }])
  wrapper = mount(TourOverlay)
  await nextTick()
  await nextTick()
  return document.querySelector('[data-testid="tour-pop"]').textContent
}

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  setActivePinia(createPinia())
  document.body.innerHTML = ''
})
afterEach(async () => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  localStorage.clear()
  await setI18nLocale('ru')
})

describe('tour wording follows the column captions on screen', () => {
  it('names the seeded columns in Russian for a Russian reader', async () => {
    const create = await popText('task-create')
    expect(create).toContain('К работе')

    wrapper.unmount()
    document.body.innerHTML = ''
    const dnd = await popText('dnd-card')
    expect(dnd).toContain('В процессе')
  })

  it('names them in English for an English reader', async () => {
    await setI18nLocale('en')

    const create = await popText('task-create')
    expect(create).toContain('To do')
    expect(create).not.toContain('К работе')

    wrapper.unmount()
    document.body.innerHTML = ''
    const dnd = await popText('dnd-card')
    expect(dnd).toContain('In progress')
    expect(dnd).not.toContain('В процессе')
  })
})
