// The v2 description of the task modal (#3113): prose that opens the editor in
// place, and the editor's way out.
//
// Pinned here:
//   • a click on the text opens the editor — but not a click on a link, a
//     checkbox, a mention, a #N chip, a picture or a diagram, and not the click
//     that ends a text selection (isEditClick);
//   • an empty description is a «Добавить описание…» line, not an open editor;
//   • the v2 editor's Esc: nothing typed → it just leaves; something typed → it
//     asks «Отменить правки?» in the footer, ↵ / «Да» throws the typing away and
//     the description goes back to what it was (persisted: a blur may already
//     have saved the typing), Esc / «Нет» keeps editing;
//   • ⌘↵ saves and leaves.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { isEditClick } from '@/utils/descEdit'

vi.mock('@/api', () => ({
  uploads: { upload: vi.fn() },
  tasks: {
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    uploadAttachment: vi.fn(),
  },
  workspaces: { taskByNumber: vi.fn() },
}))
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))

const sel = (text = '') => ({ isCollapsed: !text, toString: () => text })
function clickOn(html, pick, selection = sel()) {
  const box = document.createElement('div')
  box.innerHTML = html
  const target = pick(box)
  return isEditClick({ target, button: 0 }, selection)
}

describe('isEditClick — what a click on the read description means', () => {
  it('plain text opens the editor', () => {
    expect(clickOn('<p>Сейчас <b>блоки</b></p>', (b) => b.querySelector('b'))).toBe(true)
  })
  it('a link, a checkbox, a mention, a #N chip are live, not the editor', () => {
    expect(clickOn('<p><a href="#">гайд</a></p>', (b) => b.querySelector('a'))).toBe(false)
    expect(clickOn('<li><input type="checkbox"> пункт</li>', (b) => b.querySelector('input'))).toBe(
      false,
    )
    expect(
      clickOn('<p><span class="mention">@Аня</span></p>', (b) => b.querySelector('span')),
    ).toBe(false)
    expect(
      clickOn('<p><span data-task-ref="9">#9</span></p>', (b) => b.querySelector('span')),
    ).toBe(false)
  })
  it('a picture and a diagram are looked at, not edited', () => {
    expect(clickOn('<p><img src="x.png"></p>', (b) => b.querySelector('img'))).toBe(false)
    expect(
      clickOn('<div class="mermaid-diagram"><svg><g></g></svg></div>', (b) => b.querySelector('g')),
    ).toBe(false)
  })
  it('the click that ends a selection is a copy, not an edit', () => {
    expect(clickOn('<p>текст</p>', (b) => b.querySelector('p'), sel('текст'))).toBe(false)
  })
  it('a modified click is left alone', () => {
    const box = document.createElement('div')
    box.innerHTML = '<p>текст</p>'
    expect(isEditClick({ target: box.firstChild, button: 0, ctrlKey: true }, sel())).toBe(false)
  })
})

async function mountTab(props = {}) {
  const Tab = (await import('@/components/task/TaskDescriptionTab.vue')).default
  const w = mount(Tab, {
    props: {
      modelValue: 'Первый абзац с [ссылкой](https://example.com).',
      taskId: 't-1',
      'onUpdate:modelValue': (v) => w.setProps({ modelValue: v }),
      ...props,
    },
    global: { stubs: { TesseraSpinner: true, UserAvatar: true, teleport: true } },
    attachTo: document.body,
  })
  await w.vm.$nextTick()
  return w
}
const tick = async (w, n = 3) => {
  for (let i = 0; i < n; i++) await w.vm.$nextTick()
}

describe('TaskDescriptionTab — reading and the way into the editor', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
  })

  it('a click on the prose opens the v2 editor; a click on the link does not', async () => {
    const w = await mountTab()
    await w.find('[data-testid="desc-prose"] a').trigger('click')
    await tick(w)
    expect(w.find('[data-testid="md2-bar"]').exists()).toBe(false)
    await w.find('[data-testid="desc-prose"] p').trigger('click')
    await tick(w)
    expect(w.find('[data-testid="md2-bar"]').exists()).toBe(true)
    expect(w.find('[data-testid="md2-save"]').exists()).toBe(true)
    w.unmount()
  })

  it('an empty description is a line to fill, not an open editor', async () => {
    const w = await mountTab({ modelValue: '' })
    expect(w.find('textarea').exists()).toBe(false)
    await w.find('[data-testid="desc-add"]').trigger('click')
    await tick(w)
    expect(w.find('textarea').exists()).toBe(true)
    w.unmount()
  })

  it('a read-only description has no door into the editor', async () => {
    const w = await mountTab({ readonly: true })
    await w.find('[data-testid="desc-prose"] p').trigger('click')
    await tick(w)
    expect(w.find('textarea').exists()).toBe(false)
    w.unmount()
  })

  it('Esc with nothing typed just leaves', async () => {
    const w = await mountTab()
    await w.vm.startEdit()
    await tick(w)
    await w.find('textarea').trigger('keydown', { key: 'Escape' })
    await tick(w)
    expect(w.find('textarea').exists()).toBe(false)
    expect(w.emitted('persist')).toBeUndefined()
    w.unmount()
  })

  it('Esc after typing asks first; ↵ throws the typing away and saves the original', async () => {
    const w = await mountTab()
    await w.vm.startEdit()
    await tick(w)
    await w.setProps({ modelValue: 'Переписано' })
    await w.find('textarea').trigger('keydown', { key: 'Escape' })
    await tick(w)
    // Still editing, with the question in the footer.
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.find('[data-testid="md2-discard"]').exists()).toBe(true)
    await w.find('textarea').trigger('keydown', { key: 'Enter' })
    await tick(w)
    expect(w.find('textarea').exists()).toBe(false)
    expect(w.props('modelValue')).toBe('Первый абзац с [ссылкой](https://example.com).')
    expect(w.emitted('persist')).toBeTruthy()
    w.unmount()
  })

  it('Esc on the question keeps editing, with the typing intact', async () => {
    const w = await mountTab()
    await w.vm.startEdit()
    await tick(w)
    await w.setProps({ modelValue: 'Переписано' })
    await w.find('textarea').trigger('keydown', { key: 'Escape' })
    await tick(w)
    await w.find('textarea').trigger('keydown', { key: 'Escape' })
    await tick(w)
    expect(w.find('[data-testid="md2-discard"]').exists()).toBe(false)
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.props('modelValue')).toBe('Переписано')
    w.unmount()
  })

  it('the Esc of the editor never reaches the dialog underneath', async () => {
    const w = await mountTab()
    await w.vm.startEdit()
    await tick(w)
    const outer = vi.fn()
    document.addEventListener('keydown', outer)
    w.find('textarea').element.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    )
    document.removeEventListener('keydown', outer)
    expect(outer).not.toHaveBeenCalled()
    w.unmount()
  })

  it('⌘↵ saves and leaves', async () => {
    const w = await mountTab()
    await w.vm.startEdit()
    await tick(w)
    await w.setProps({ modelValue: 'Новое' })
    await w.find('textarea').trigger('keydown', { key: 'Enter', ctrlKey: true })
    await tick(w)
    expect(w.find('textarea').exists()).toBe(false)
    expect(w.emitted('persist')).toBeTruthy()
    expect(w.props('modelValue')).toBe('Новое')
    w.unmount()
  })

  it('offers @-mentions in the editor (they were off in the description)', async () => {
    const members = [{ id: 'u1', label: 'Аня Ковалёва' }]
    const w = await mountTab({ members })
    await w.vm.startEdit()
    await tick(w)
    const ed = w.findComponent({ name: 'MarkdownEditor' })
    expect(ed.props('mentionItems')).toEqual(members)
    expect(w.find('[data-testid="md2-tool-mention"]').exists()).toBe(true)
    w.unmount()
  })
})
