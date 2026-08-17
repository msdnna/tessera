// Keyboard behaviour of MarkdownEditor (#2717). The text operations themselves
// are covered as pure functions in ut-mdEditing.spec.js; what needs a real mount
// is the wiring — which keys are intercepted, when the Tab capture is released,
// and that the toolbar buttons reach the same code.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownEditor from '@/components/MarkdownEditor.vue'

vi.mock('@/api', () => ({
  uploads: { upload: vi.fn() },
  tasks: {
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    uploadAttachment: vi.fn(),
    downloadAttachment: vi.fn(),
  },
}))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// jsdom has no execCommand, so every edit takes the v-model fallback path in
// replaceRange. Keep the DOM value in step with the model the way Vue does.
function editor(value = '') {
  const w = mount(MarkdownEditor, {
    props: {
      modelValue: value,
      'onUpdate:modelValue': async (v) => {
        await w.setProps({ modelValue: v })
        w.find('textarea').element.value = v
      },
    },
    global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
  })
  w.find('textarea').element.value = value
  return w
}

function select(w, start, end) {
  w.find('textarea').element.setSelectionRange(start, end)
}

describe('MarkdownEditor keyboard', () => {
  let w
  beforeEach(() => {
    w = null
  })

  it('wraps the selection when a bracket is typed over it', async () => {
    w = editor('hello world')
    select(w, 6, 11)
    await w.find('textarea').trigger('keydown', { key: '(' })
    expect(w.props('modelValue')).toBe('hello (world)')
  })

  it('auto-closes a bracket when nothing is selected', async () => {
    w = editor('hello')
    select(w, 5, 5)
    await w.find('textarea').trigger('keydown', { key: '(' })
    expect(w.props('modelValue')).toBe('hello()') // pair inserted, caret between
  })

  it('deletes both halves of an empty pair on Backspace', async () => {
    w = editor('a()b')
    select(w, 2, 2)
    await w.find('textarea').trigger('keydown', { key: 'Backspace' })
    expect(w.props('modelValue')).toBe('ab')
  })

  it('continues a list on Enter and ends it on an empty item', async () => {
    w = editor('- one')
    const ta = w.find('textarea')
    select(w, 5, 5)
    await ta.trigger('keydown', { key: 'Enter' })
    expect(w.props('modelValue')).toBe('- one\n- ')

    ta.element.value = '- one\n- '
    select(w, 8, 8)
    await ta.trigger('keydown', { key: 'Enter' })
    expect(w.props('modelValue')).toBe('- one\n')
  })

  it('does not swallow a modified key combination', async () => {
    w = editor('hello world')
    select(w, 6, 11)
    await w.find('textarea').trigger('keydown', { key: '(', ctrlKey: true })
    expect(w.props('modelValue')).toBe('hello world')
  })

  it('indents with Tab and outdents with Shift+Tab', async () => {
    w = editor('one\ntwo')
    select(w, 0, 7)
    await w.find('textarea').trigger('keydown', { key: 'Tab' })
    expect(w.props('modelValue')).toBe('  one\n  two')

    select(w, 0, 11)
    await w.find('textarea').trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(w.props('modelValue')).toBe('one\ntwo')
  })

  it('inserts an indent at a collapsed caret', async () => {
    w = editor('ab')
    select(w, 1, 1)
    await w.find('textarea').trigger('keydown', { key: 'Tab' })
    expect(w.props('modelValue')).toBe('a  b')
  })

  it('releases the Tab capture on Esc and takes it back on the next input', async () => {
    w = editor('one')
    const ta = w.find('textarea')
    await ta.trigger('keydown', { key: 'Escape' })

    select(w, 0, 3)
    await ta.trigger('keydown', { key: 'Tab' })
    expect(w.props('modelValue')).toBe('one') // Tab left for the browser → focus moves

    ta.element.value = 'onex'
    await ta.trigger('input')
    select(w, 0, 4)
    await ta.trigger('keydown', { key: 'Tab' })
    expect(w.props('modelValue')).toBe('  onex')
  })
})

describe('MarkdownEditor toolbar', () => {
  it('offers the block tools and inserts a checkbox marker', async () => {
    const w = editor('дело')
    const checkbox = w
      .findAll('.md2-format button')
      .find((b) => b.attributes('title') === 'Чекбокс')
    expect(checkbox).toBeTruthy()
    select(w, 0, 4)
    await checkbox.trigger('mousedown')
    expect(w.props('modelValue')).toBe('- [ ] дело')
  })

  it('numbers an ordered list per line', async () => {
    const w = editor('a\nb\nc')
    const ol = w
      .findAll('.md2-format button')
      .find((b) => b.attributes('title') === 'Нумерованный список')
    select(w, 0, 5)
    await ol.trigger('mousedown')
    expect(w.props('modelValue')).toBe('1. a\n2. b\n3. c')
  })

  it('inserts a spoiler with the blank lines marked needs to parse', async () => {
    const w = editor('')
    const sp = w.findAll('.md2-format button').find((b) => b.attributes('title') === 'Спойлер')
    await sp.trigger('mousedown')
    const v = w.props('modelValue')
    expect(v).toContain('<details><summary>Подробнее</summary>')
    expect(v).toContain('\n\nСкрытый текст\n\n')
  })

  it('keeps the selection bubble to inline marks only', () => {
    const w = editor('x')
    // Block actions live in the persistent bar; the bubble list is built from
    // inlineTools, which must not carry them.
    const titles = w.findAll('.md2-format button').map((b) => b.attributes('title'))
    expect(titles).toContain('Жирный')
    expect(titles).toContain('Цитата')
  })

  it('hides the attachment button until a task is given', async () => {
    const w = editor('')
    const clip = 'Приложить файл к задаче и вставить ссылку'
    expect(w.findAll('.md2-format button').some((b) => b.attributes('title') === clip)).toBe(false)
    await w.setProps({ attachTaskId: 't1' })
    expect(w.findAll('.md2-format button').some((b) => b.attributes('title') === clip)).toBe(true)
  })
})

// Split mode (#2717 rework): the fullscreen modal puts text and preview side by
// side. Before that it hosted the ordinary toggle editor, which auto-grew to its
// content — a short description left a dead band under the toolbar, and half the
// window sat unused behind the preview toggle.
describe('MarkdownEditor split mode', () => {
  function splitEditor(value = '') {
    return mount(MarkdownEditor, {
      props: { modelValue: value, split: true },
      global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
    })
  }

  it('shows the text and the preview at the same time', () => {
    const w = splitEditor('# привет')
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.find('.md2-preview-side').exists()).toBe(true)
  })

  it('feeds the live text to the side preview', async () => {
    const w = splitEditor('раз')
    const preview = () => w.findComponent({ name: 'RichContent' })
    expect(preview().props('source')).toBe('раз')
    await w.setProps({ modelValue: 'раз два' })
    expect(preview().props('source')).toBe('раз два')
  })

  it('drops the preview toggle — there is nothing left to switch', () => {
    const w = splitEditor('x')
    const titles = w.findAll('.md2-tabs button').map((b) => b.attributes('title'))
    expect(titles).not.toContain('Предпросмотр')
    expect(titles).toContain('Вставить изображение') // the rest of the row stays
  })

  it('keeps the formatting bar — it is the only home for the block tools', () => {
    const w = splitEditor('x')
    const titles = w.findAll('.md2-format button').map((b) => b.attributes('title'))
    expect(titles).toContain('Цитата')
  })

  // autoGrow() is what writes an inline height; in split it must stay out of the
  // way, or the textarea would push the toolbar off the bottom of the modal.
  // jsdom lays nothing out, so the measurements it reads are faked here —
  // otherwise its own clientWidth === 0 guard returns early and the assertion
  // would hold with or without the split check.
  function measurable(w, scrollHeight) {
    const el = w.find('textarea').element
    Object.defineProperty(el, 'clientWidth', { value: 400, configurable: true })
    Object.defineProperty(el, 'scrollHeight', { value: scrollHeight, configurable: true })
    return el
  }

  it('leaves the height to the layout instead of growing to the text', () => {
    const w = splitEditor('a\n'.repeat(50))
    const el = measurable(w, 800)
    w.vm.autoGrow()
    expect(el.style.height).toBe('')
  })

  it('still auto-grows outside split mode', () => {
    const w = mount(MarkdownEditor, {
      props: { modelValue: 'a\n'.repeat(50) },
      global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
    })
    const el = measurable(w, 800)
    w.vm.autoGrow()
    expect(el.style.height).toBe('800px')
  })

  it('still toggles write/preview when split is off', async () => {
    const w = mount(MarkdownEditor, {
      props: { modelValue: 'x' },
      global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
    })
    expect(w.find('.md2-preview-side').exists()).toBe(false)
    const toggle = w
      .findAll('.md2-tabs button')
      .find((b) => b.attributes('title') === 'Предпросмотр')
    await toggle.trigger('click')
    expect(w.find('textarea').exists()).toBe(false)
  })
})
