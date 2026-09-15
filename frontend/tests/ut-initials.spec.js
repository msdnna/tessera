// Avatar initials have to mean one thing across the app (#2932).
//
// `utils/initials` knows three branches — two words, a dot-separated handle, a
// single token — but the two discs showing the viewer's *own* face re-derived it
// locally and knew only the first and the last. For a handle like «a.fokin» (the
// shape a GitLab roster hands us) those copies returned the first two characters
// of the raw string: «A.», a letter and a punctuation mark, while the same person
// was «AF» on every card and in every comment. Both places fall back to the
// e-mail when the name is empty, which made the dot the common case, not the rare
// one.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { initials } from '@/utils/initials'

vi.mock('@/api', () => ({
  users: { updatePrefs: vi.fn() },
  accountFlows: {},
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useRoute: () => ({ path: '/', params: {}, query: {} }),
}))

const mounted = []

beforeEach(() => {
  setActivePinia(createPinia())
})

afterEach(() => {
  while (mounted.length) mounted.pop().unmount()
})

describe('the initials rule', () => {
  it('reads a dot-separated handle part by part', () => {
    expect(initials('a.fokin')).toBe('AF')
    expect(initials('v.sokolov')).toBe('VS')
  })

  it('reads an e-mail as the person behind it', () => {
    expect(initials('ivan.petrov@example.com')).toBe('IP')
  })

  it('reads two words, a single token and nothing at all', () => {
    expect(initials('Василий Соколов')).toBe('ВС')
    expect(initials('msdnna')).toBe('MS')
    expect(initials('')).toBe('?')
    expect(initials('?')).toBe('?')
  })

  // A dot ending a word is punctuation, not a separator: «Иван П.» is still a
  // two-word name.
  it('does not mistake a trailing dot for a handle', () => {
    expect(initials('Иван П.')).toBe('ИП')
  })
})

describe('the discs showing your own avatar', () => {
  const withUser = async (user) => {
    const { useAuthStore } = await import('@/stores/auth')
    useAuthStore().user = user
  }

  // Collapsed rail: the avatar is the popover's trigger, which is the shape that
  // sits on screen all day long.
  const footerInitials = async () => {
    const SidebarFooter = (await import('@/components/SidebarFooter.vue')).default
    const w = mount(SidebarFooter, { props: { collapsed: true } })
    mounted.push(w)
    await w.vm.$nextTick()
    return w.find('.ava').text()
  }

  it('spells a handle the way the cards do, not as a letter and a dot', async () => {
    await withUser({ id: 'u1', name: 'a.fokin', email: 'a.fokin@example.com' })
    expect(await footerInitials()).toBe('AF')
  })

  it('falls back to the e-mail with the same rule', async () => {
    await withUser({ id: 'u1', name: '', email: 'ivan.petrov@example.com' })
    expect(await footerInitials()).toBe('IP')
  })

  it('still spells an ordinary two-word name', async () => {
    await withUser({ id: 'u1', name: 'Василий Соколов', email: 'v@example.com' })
    expect(await footerInitials()).toBe('ВС')
  })
})
