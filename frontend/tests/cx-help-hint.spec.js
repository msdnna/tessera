import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { useHelpStore } from '@/stores/help'

// The «?» button (#2794). Its whole job is picking a slug and handing it to the
// store, so that is what is asserted here — the drawer itself is a Naive shell
// around HelpArticle, already covered by the store and index specs.
const route = { path: '/' }
vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ push: vi.fn() }),
}))

const stubs = {
  // Tooltip renders its trigger; without this the button never mounts.
  NTooltip: { template: '<span class="tip-stub"><slot name="trigger" /><slot /></span>' },
  NIcon: { template: '<i class="n-icon-stub" />' },
}

const HelpHint = (await import('@/components/help/HelpHint.vue')).default

function mountHint(props = {}) {
  return mount(HelpHint, { props, global: { stubs } })
}

describe('HelpHint.vue', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    route.path = '/'
  })

  it('берёт статью из маршрута и выставляет её в data-help', () => {
    route.path = '/project/tessera/board/main'
    const w = mountHint()
    expect(w.find('button').attributes('data-help')).toBe('boards-and-tasks')
  })

  it('явный slug важнее маршрута', () => {
    route.path = '/notes'
    const w = mountHint({ slug: 'faq' })
    expect(w.find('button').attributes('data-help')).toBe('faq')
  })

  it('на странице справки кнопки нет', () => {
    route.path = '/help/first-steps'
    expect(mountHint().find('button').exists()).toBe(false)
  })

  it('несуществующая статья не даёт мёртвой кнопки', () => {
    expect(mountHint({ slug: 'нет-такой-статьи' }).find('button').exists()).toBe(false)
  })

  it('клик открывает панель именно на этой статье', async () => {
    route.path = '/milestones'
    const w = mountHint()
    const help = useHelpStore()
    await w.find('button').trigger('click')
    expect(help.drawerShown).toBe(true)
    expect(help.drawerSlug).toBe('milestones')
  })

  it('подпись кнопки называет статью — иначе «?» ни о чём', () => {
    route.path = '/reminders'
    const label = mountHint().find('button').attributes('aria-label')
    expect(label).toContain(useHelpStore().bySlug('reminders').title)
  })
})
