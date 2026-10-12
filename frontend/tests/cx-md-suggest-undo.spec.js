// Picking a row from the @-mention / slash popup must join the textarea's own
// undo stack (#2932).
//
// MarkdownEditor already knows this rule — replaceRange() inserts through
// execCommand('insertText') precisely so Ctrl+Z steps back over the edit, and
// every toolbar button, snippet and upload goes through it. The suggestion
// popup did not: it built the whole next value and pushed it out through
// v-model. Assigning `.value` from script drops the browser's undo history for
// that field, so one accepted mention threw away the undo stack of a long
// comment — Ctrl+Z after it does nothing at all.
import { describe, it, expect, afterEach, vi } from 'vitest'
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

const MENTIONS = [
  { id: 'u1', label: 'anna', display: 'Анна' },
  { id: 'u2', label: 'andrey', display: 'Андрей' },
]

let wrapper = null
let calls = null

afterEach(() => {
  delete document.execCommand
  calls = null
  wrapper?.unmount()
  wrapper = null
})

// jsdom has no execCommand at all, so the component can only ever take the
// fallback there. Stand in for the browser: record the call, splice the text in
// at the current selection and fire `input`, exactly as the real one does.
function stubExecCommand() {
  calls = []
  document.execCommand = vi.fn((cmd, _ui, text) => {
    const el = wrapper.find('textarea').element
    calls.push({ cmd, text, start: el.selectionStart, end: el.selectionEnd })
    el.value = el.value.slice(0, el.selectionStart) + text + el.value.slice(el.selectionEnd)
    el.dispatchEvent(new Event('input'))
    return true
  })
}

function editor(props = {}) {
  wrapper = mount(MarkdownEditor, {
    attachTo: document.body,
    props: {
      modelValue: '',
      mentionItems: MENTIONS,
      'onUpdate:modelValue': async (v) => {
        await wrapper.setProps({ modelValue: v })
        if (wrapper.find('textarea').element.value !== v) {
          wrapper.find('textarea').element.value = v
        }
      },
      ...props,
    },
    global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
  })
  return wrapper
}

// Type into the textarea the way the browser does: DOM value first, then input.
async function type(w, value) {
  const ta = w.find('textarea')
  ta.element.value = value
  ta.element.setSelectionRange(value.length, value.length)
  await ta.trigger('input')
}

describe('MarkdownEditor: the suggestion popup keeps the undo stack', () => {
  it('inserts an accepted mention through the native path', async () => {
    stubExecCommand()
    const w = editor()
    await type(w, 'привет @an')
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await w.vm.$nextTick()

    expect(document.execCommand).toHaveBeenCalledTimes(1)
    expect(calls[0].cmd).toBe('insertText')
    expect(calls[0].text).toBe('@anna ')
  })

  it('replaces exactly the typed query, not the whole field', async () => {
    stubExecCommand()
    const w = editor()
    await type(w, 'привет @an')
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await w.vm.$nextTick()

    // [7, 10) is "@an" — the half-typed mention and nothing else. A wider range
    // would make undo restore text the popup never touched.
    expect(calls[0].start).toBe(7)
    expect(calls[0].end).toBe(10)
  })

  it('still produces the right text and caret', async () => {
    stubExecCommand()
    const w = editor()
    await type(w, 'привет @an')
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await w.vm.$nextTick()
    await w.vm.$nextTick()

    expect(w.props('modelValue')).toBe('привет @anna ')
    expect(w.find('textarea').element.selectionStart).toBe('привет @anna '.length)
  })

  it('keeps text that follows the caret', async () => {
    stubExecCommand()
    const w = editor()
    const ta = w.find('textarea')
    ta.element.value = 'привет @an, как дела'
    ta.element.setSelectionRange(10, 10) // caret right after "@an"
    await ta.trigger('input')
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await w.vm.$nextTick()
    await w.vm.$nextTick()

    expect(w.props('modelValue')).toBe('привет @anna , как дела')
  })

  it('inserts a picked slash command through the native path too', async () => {
    stubExecCommand()
    const w = editor({
      mentionItems: [],
      commandItems: [{ key: 'done', label: 'Закрыть', builtin: true, arg: 'none' }],
    })
    await type(w, '/do')
    await w.findAll('.md2-cmd-item')[0].trigger('mousedown')
    await w.vm.$nextTick()

    expect(document.execCommand).toHaveBeenCalledTimes(1)
    expect(calls[0].text).toBe('/done\n')
    expect(calls[0].start).toBe(0)
    expect(calls[0].end).toBe(3)
  })

  it('falls back to a plain write where execCommand is missing', async () => {
    const w = editor() // no stub: this is WebKitGTK/Tauri and jsdom
    await type(w, 'привет @an')
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await w.vm.$nextTick()
    await w.vm.$nextTick()

    expect(w.props('modelValue')).toBe('привет @anna ')
  })
})
