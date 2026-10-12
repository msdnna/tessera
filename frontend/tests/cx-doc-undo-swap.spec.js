import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// useMessage() throws without an <n-message-provider>; keep the rest of naive-ui
// intact and stub only that, as the other component specs do.
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const { default: DocEditor } = await import('@/components/documents/DocEditor.vue')

const blk = (id, text) => ({
  type: 'paragraph',
  attrs: { id },
  content: [{ type: 'text', text }],
})

const doc = (id, ...texts) => ({
  type: 'doc',
  content: texts.map((text, i) => blk(`${id}${i + 1}`, text)),
})

const textsOf = (editor) => editor.getJSON().content.map((b) => b.content?.[0]?.text ?? '')
const allText = (editor) => textsOf(editor).join(' ')

/**
 * The document the editor shows is swapped from the outside — by an internal
 * link, the Back button or a version restore — without the component being
 * remounted (DocumentsView keeps one editor for the whole section, #2727).
 *
 * So the undo stack outlives the document it was recorded against, and that is
 * the trap: Ctrl+Z is not "undo my last edit" any more but "apply a step from
 * another document to this one". The saved result is the damage — an undo is a
 * local edit, so it goes straight into the autosave.
 */
describe('DocEditor: смена документа и стек отмены', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  function mountEditor(content) {
    wrapper = mount(DocEditor, { props: { modelValue: content }, attachTo: document.body })
    return wrapper.vm.editor
  }

  async function swapTo(content) {
    await wrapper.setProps({ modelValue: content })
    await nextTick()
  }

  // The plain case: open one document, open another, press Ctrl+Z. The previous
  // document must not come back — and above all must not be saved over the one
  // on screen.
  it('отмена не возвращает предыдущий документ', async () => {
    const editor = mountEditor(doc('a', 'первый документ'))
    await nextTick()
    await swapTo(doc('b', 'второй документ'))
    expect(textsOf(editor)).toEqual(['второй документ'])

    const changes = wrapper.emitted('change')?.length ?? 0
    editor.commands.undo()
    await nextTick()

    expect(textsOf(editor)).toEqual(['второй документ'])
    expect(wrapper.emitted('change')?.length ?? 0).toBe(changes)
  })

  // Steps recorded in the previous document are remapped through the swap rather
  // than dropped, so an undo of a *deletion* there is an insertion here: the old
  // document's text lands in the middle of the new one.
  it('отмена не переносит текст из прошлого документа', async () => {
    const editor = mountEditor(doc('a', 'секретный абзац'))
    await nextTick()
    editor.commands.setTextSelection({ from: 1, to: editor.state.doc.content.size - 1 })
    editor.commands.deleteSelection()
    await nextTick()

    await swapTo(doc('b', 'второй документ'))
    editor.commands.undo()
    await nextTick()

    expect(allText(editor)).not.toContain('секретный')
    expect(textsOf(editor)).toEqual(['второй документ'])
  })

  // Same for typing: the undo of an insertion is a deletion, and a deletion that
  // lands in the wrong document eats text nobody touched.
  it('отмена набранного в прошлом документе не режет новый', async () => {
    const editor = mountEditor(doc('a', 'первый'))
    await nextTick()
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    editor.commands.insertContent(' дописано')
    await nextTick()

    await swapTo(doc('b', 'второй документ'))
    editor.commands.undo()
    await nextTick()

    expect(textsOf(editor)).toEqual(['второй документ'])
  })

  // The stack is reset, not disabled: editing the newly opened document must be
  // undoable as usual.
  it('в новом документе отмена работает как обычно', async () => {
    const editor = mountEditor(doc('a', 'первый'))
    await nextTick()
    await swapTo(doc('b', 'второй'))

    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    editor.commands.insertContent(' правка')
    await nextTick()
    expect(textsOf(editor)).toEqual(['второй правка'])

    editor.commands.undo()
    await nextTick()
    expect(textsOf(editor)).toEqual(['второй'])
  })

  // The swap goes in with `emitUpdate: false`, and resetting the history must not
  // quietly turn that back on: an echo here marks a freshly opened document dirty
  // and autosaves it over itself.
  it('смена документа не считается правкой', async () => {
    mountEditor(doc('a', 'первый'))
    await nextTick()
    const changes = wrapper.emitted('change')?.length ?? 0

    await swapTo(doc('b', 'второй документ'))

    expect(wrapper.emitted('change')?.length ?? 0).toBe(changes)
  })

  // The decorations of the other plugins survive the reset — they are pushed in
  // from outside (locks, comment markers) and nothing re-pushes them on a swap.
  it('блокировки блоков переживают смену документа', async () => {
    const editor = mountEditor(doc('a', 'первый'))
    await nextTick()
    await wrapper.setProps({ locks: [{ block_id: 'b1', name: 'Сосед' }] })
    await swapTo(doc('b', 'второй документ'))
    await nextTick()

    expect(editor.view.dom.querySelector('.doc-block-locked')).not.toBeNull()
  })
})
