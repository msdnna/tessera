import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'

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

  it('tells the user what an empty gallery accepts', () => {
    wrapper = mountGallery({ templates: [] })
    expect(document.body.textContent).toContain('.md')
    expect(document.body.textContent).toContain('.json')
  })
})
