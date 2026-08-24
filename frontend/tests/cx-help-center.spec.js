import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { useHelpStore } from '@/stores/help'
import HelpNav from '@/components/help/HelpNav.vue'
import HelpSearch from '@/components/help/HelpSearch.vue'

// The help centre is a modal, not a page (#2792). The point of the rework is that
// nothing in it touches the router: opening it, walking the nav and picking a
// search hit must all leave the reader's board exactly where it was. So the
// router is mocked with a spy here and asserted to stay untouched — a regression
// that turns any of these back into a `router.push` fails loudly.
const push = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', params: {} }),
  useRouter: () => ({ push }),
}))

const stubs = {
  NIcon: { template: '<i class="n-icon-stub" />' },
  // The search box is a Naive input; the spec only cares about the result list,
  // so the input is reduced to something that carries v-model.
  NInput: {
    props: ['value'],
    emits: ['update:value'],
    template: '<input :value="value" @input="$emit(\'update:value\', $event.target.value)" />',
  },
}

describe('Справочный центр без роутера (#2792)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    push.mockClear()
  })

  describe('стор', () => {
    it('openCenter показывает модалку и открывает статью', async () => {
      const help = useHelpStore()
      expect(help.centerShown).toBe(false)
      await help.openCenter('first-steps')
      expect(help.centerShown).toBe(true)
      expect(help.current).toBe('first-steps')
      expect(help.body).not.toBe('')
    })

    it('без slug открывает статью по умолчанию, а потом — последнюю читанную', async () => {
      const help = useHelpStore()
      await help.openCenter()
      expect(help.current).toBe(help.defaultSlug)

      await help.open('faq')
      help.closeCenter()
      expect(help.centerShown).toBe(false)

      // Возврат в справку продолжает с того же места, а не с «Первых шагов».
      await help.openCenter()
      expect(help.current).toBe('faq')
    })

    it('закрытие не сбрасывает открытую статью', async () => {
      const help = useHelpStore()
      await help.openCenter('faq')
      help.closeCenter()
      expect(help.current).toBe('faq')
    })

    it('контекстный drawer и модалка читают порознь', async () => {
      const help = useHelpStore()
      await help.openCenter('first-steps')
      await help.openDrawer('faq')
      expect(help.current).toBe('first-steps')
      expect(help.drawerSlug).toBe('faq')
    })
  })

  describe('HelpNav.vue', () => {
    it('пункты — кнопки, а не ссылки', () => {
      const w = mount(HelpNav, { global: { stubs } })
      expect(w.findAll('a').length).toBe(0)
      expect(w.findAll('button.h-link').length).toBeGreaterThan(0)
    })

    it('клик открывает статью в сторе и не трогает роутер', async () => {
      const help = useHelpStore()
      const w = mount(HelpNav, { global: { stubs } })
      const links = w.findAll('button.h-link')
      await links[links.length - 1].trigger('click')
      expect(help.current).toBeTruthy()
      expect(push).not.toHaveBeenCalled()
    })

    it('активный пункт подсвечен по current из стора', async () => {
      const help = useHelpStore()
      await help.open('faq')
      const w = mount(HelpNav, { global: { stubs } })
      const active = w.findAll('button.h-link-active')
      expect(active.length).toBe(1)
      expect(active[0].text()).toBe(help.bySlug('faq').title)
    })
  })

  describe('HelpSearch.vue', () => {
    it('выбор результата открывает статью в сторе, чистит запрос и не навигирует', async () => {
      const help = useHelpStore()
      help.query = 'доск'
      const w = mount(HelpSearch, { global: { stubs } })
      const hits = w.findAll('button.h-result')
      expect(hits.length).toBeGreaterThan(0)

      const slug = help.results[0].slug
      await hits[0].trigger('click')
      expect(help.current).toBe(slug)
      expect(help.query).toBe('')
      expect(push).not.toHaveBeenCalled()
    })
  })
})
