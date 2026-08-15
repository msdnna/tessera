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

  it('leaves the default alone when nothing is selected', async () => {
    w = editor('hello')
    select(w, 5, 5)
    await w.find('textarea').trigger('keydown', { key: '(' })
    expect(w.props('modelValue')).toBe('hello') // the browser inserts it, not us
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
    const checkbox = w.findAll('.md2-format button').find((b) => b.attributes('title') === 'Чекбокс')
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
