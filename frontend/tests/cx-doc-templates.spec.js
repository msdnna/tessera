import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

// The meeting picker formats dates through useFormat, which reads the theme store.
setActivePinia(createPinia())

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: DocTemplates } = await import('@/components/documents/DocTemplates.vue')
const { builtinCards } = await import('@/utils/docTemplates')

function saved(over = {}) {
  return {
    id: 't1',
    title: 'Протокол ревью',
    description: 'Наш формат',
    icon: '',
    preview: 'Повестка…',
    author_name: 'Иван Петров',
    ...over,
  }
}

// The gallery is rendered into a modal, so assertions go through the attached
// document rather than the wrapper's own subtree.
function mountGallery(props = {}) {
  return mount(DocTemplates, {
    props: { show: true, templates: [saved()], ...props },
    attachTo: document.body,
  })
}

describe('DocTemplates gallery', () => {
  let wrapper
  afterEach(() => {
    wrapper?.unmount()
    document.body.innerHTML = ''
  })

  it('renders a card per template', () => {
    wrapper = mountGallery({ templates: [saved(), ...builtinCards()] })
    const tiles = document.querySelectorAll('[data-testid="tpl-tile"]')
    expect(tiles).toHaveLength(1 + builtinCards().length)
    expect(document.body.textContent).toContain('Протокол ревью')
  })

  it('emits the picked template', async () => {
    wrapper = mountGallery()
    document.querySelector('[data-testid="tpl-use"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('use')[0][0].id).toBe('t1')
  })

  // A built-in is a frontend constant: there is no row to delete, and offering
  // the button would be a control that cannot work.
  it('offers deletion only for saved templates', () => {
    wrapper = mountGallery({ templates: builtinCards() })
    expect(document.querySelector('[title="Удалить шаблон"]')).toBeNull()
    wrapper.unmount()
    document.body.innerHTML = ''

    wrapper = mountGallery()
    expect(document.querySelector('[title="Удалить шаблон"]')).not.toBeNull()
  })

  it('filters by title and description', async () => {
    wrapper = mountGallery({ templates: [saved(), saved({ id: 't2', title: 'Смета' })] })
    await wrapper.vm.$nextTick()
    wrapper.vm.query = 'смет'
    await wrapper.vm.$nextTick()
    const tiles = document.querySelectorAll('[data-testid="tpl-tile"]')
    expect(tiles).toHaveLength(1)
    expect(tiles[0].textContent).toContain('Смета')
  })

  // One click, one document: the create round-trips through the server, and a
  // second click while it is in flight would make two.
  it('disables the buttons while a template is being used', () => {
    wrapper = mountGallery({ busy: 't1' })
    const btn = document.querySelector('[data-testid="tpl-use"]')
    expect(btn.classList.toString()).toMatch(/disabled|loading/)
  })

  it('shows an import error instead of failing silently', () => {
    wrapper = mountGallery({ error: 'Файл больше 2 МБ' })
    expect(document.body.textContent).toContain('Файл больше 2 МБ')
  })

  // #3045: the column lists the mockup's categories with counts, «Мои» is the
  // reader's own saved templates.
  it('filters by category and by «Мои шаблоны»', async () => {
    wrapper = mountGallery({
      userId: 'u1',
      templates: [
        saved({ id: 'a', title: 'Мой спринт', category: 'sprint', author_id: 'u1' }),
        saved({ id: 'b', title: 'Чужой', category: '', author_id: 'u2' }),
        ...builtinCards(),
      ],
    })
    const count = (s) =>
      document.querySelector(`[data-testid="tpl-source-${s}"] .tpl-src-n`).textContent.trim()
    expect(count('mine')).toBe('1')
    expect(Number(count('sprint'))).toBe(
      1 + builtinCards().filter((c) => c.category === 'sprint').length,
    )
    document.querySelector('[data-testid="tpl-source-mine"]').click()
    await wrapper.vm.$nextTick()
    const tiles = document.querySelectorAll('[data-testid="tpl-tile"]')
    expect(tiles).toHaveLength(1)
    expect(tiles[0].textContent).toContain('Мой спринт')
  })

  // #3046: the agenda template is used together with the meeting the footer
  // preselected.
  it('passes the preselected meeting with the agenda template', async () => {
    const meeting = { id: 'm1', title: 'Планирование', status: 'live' }
    const agenda = builtinCards().find((c) => c.key === 'meeting')
    wrapper = mountGallery({ templates: [agenda], meetings: [meeting] })
    await wrapper.vm.$nextTick()
    document.querySelector('[data-testid="tpl-use"]').click()
    await wrapper.vm.$nextTick()
    const [card, opts] = wrapper.emitted('use')[0]
    expect(card.key).toBe('meeting')
    expect(opts.conferenceId).toBe('m1')
  })

  it('tells the user what an empty gallery accepts', () => {
    wrapper = mountGallery({ templates: [] })
    expect(document.body.textContent).toContain('.md')
    expect(document.body.textContent).toContain('.json')
  })
})
