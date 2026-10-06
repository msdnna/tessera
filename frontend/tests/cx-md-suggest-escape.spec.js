// Esc inside the composer must dismiss exactly ONE layer (#2932).
//
// MarkdownEditor lives inside n-modal (task modal, fullscreen composer), and
// naive-ui closes a modal from a document-level keydown listener. So every Esc
// the editor consumes has to be kept from bubbling, or the user loses the whole
// modal — and the unsaved draft with it — instead of just closing the popup.
// The Tab-trap release already did that; the @-mention / slash popup did not.
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
let onDocEsc = null

afterEach(() => {
  if (onDocEsc) document.removeEventListener('keydown', onDocEsc)
  onDocEsc = null
  wrapper?.unmount()
  wrapper = null
})

// Mounted into the document so a keydown really bubbles up to it — that is the
// listener naive-ui's modal uses, and the only way to observe "the modal closed".
function editor(value = '') {
  wrapper = mount(MarkdownEditor, {
    attachTo: document.body,
    props: {
      modelValue: value,
      mentionItems: MENTIONS,
      'onUpdate:modelValue': async (v) => {
        await wrapper.setProps({ modelValue: v })
        wrapper.find('textarea').element.value = v
      },
    },
    global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
  })
  const ta = wrapper.find('textarea').element
  ta.value = value
  ta.setSelectionRange(value.length, value.length)
  onDocEsc = vi.fn()
  document.addEventListener('keydown', onDocEsc)
  return wrapper
}

// Type into the textarea the way the browser does: DOM value first, then input.
async function type(w, value) {
  const ta = w.find('textarea')
  ta.element.value = value
  ta.element.setSelectionRange(value.length, value.length)
  await ta.trigger('input')
}

async function esc(w) {
  await w.find('textarea').trigger('keydown', { key: 'Escape' })
}

const popupOpen = (w) => w.findAll('.md2-mention-item').length > 0

describe('MarkdownEditor: Esc closes one layer at a time', () => {
  it('keeps the Esc that dismisses the mention popup away from the modal', async () => {
    const w = editor('')
    await type(w, '@an')
    expect(popupOpen(w)).toBe(true)

    await esc(w)
    expect(popupOpen(w)).toBe(false) // the popup is gone…
    expect(onDocEsc).not.toHaveBeenCalled() // …and the task modal survived
    expect(w.emitted('cancel')).toBeUndefined() // the composer was not cancelled either
  })

  it('lets a later Esc through once there is nothing left to dismiss', async () => {
    const w = editor('')
    await type(w, '@an')

    await esc(w) // 1: closes the popup
    await esc(w) // 2: releases the Tab capture, asks the host to cancel
    expect(onDocEsc).not.toHaveBeenCalled()
    expect(w.emitted('cancel')).toHaveLength(1)

    await esc(w) // 3: nothing of ours left — the modal gets it
    expect(onDocEsc).toHaveBeenCalledTimes(1)
  })

  it('keeps Tab captured while the popup is the thing being dismissed', async () => {
    const w = editor('')
    await type(w, '@an')
    await esc(w)

    // Esc spent on the popup must not also hand Tab back to the browser: the
    // caret is still in the middle of a list item the user is indenting.
    w.find('textarea').element.setSelectionRange(0, 3)
    await w.find('textarea').trigger('keydown', { key: 'Tab' })
    expect(w.props('modelValue')).toBe('  @an')
  })

  it('does the same for the slash-command popup', async () => {
    wrapper = mount(MarkdownEditor, {
      attachTo: document.body,
      props: {
        modelValue: '',
        commandItems: [{ key: 'table', label: 'Таблица', apply: (c) => c.run() }],
        'onUpdate:modelValue': async (v) => {
          await wrapper.setProps({ modelValue: v })
          wrapper.find('textarea').element.value = v
        },
      },
      global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
    })
    onDocEsc = vi.fn()
    document.addEventListener('keydown', onDocEsc)

    await type(wrapper, '/tab')
    expect(wrapper.findAll('.md2-cmd-item').length).toBeGreaterThan(0)

    await esc(wrapper)
    expect(wrapper.findAll('.md2-cmd-item').length).toBe(0)
    expect(onDocEsc).not.toHaveBeenCalled()
  })
})
