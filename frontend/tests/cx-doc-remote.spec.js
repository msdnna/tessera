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

function doc(...texts) {
  return { type: 'doc', content: texts.map((text, i) => blk(`b${i + 1}`, text)) }
}

const textsOf = (editor) => editor.getJSON().content.map((b) => b.content?.[0]?.text ?? '')
const idsOf = (editor) => editor.getJSON().content.map((b) => b.attrs.id)

// applyRemote is how a colleague's edit reaches the screen (#2729 rework). The
// requirement it has to meet is not "the text arrives" — setContent does that
// too — but "the text arrives without disturbing the person typing".
describe('DocEditor.applyRemote', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  function mountEditor(content) {
    wrapper = mount(DocEditor, { props: { modelValue: content }, attachTo: document.body })
    return wrapper.vm.editor
  }

  it('replaces only the blocks that changed', async () => {
    const editor = mountEditor(doc('первый', 'второй'))
    await nextTick()

    expect(wrapper.vm.applyRemote(doc('правка коллеги', 'второй'))).toBe(true)
    expect(textsOf(editor)).toEqual(['правка коллеги', 'второй'])
  })

  // The whole reason this is a transaction and not setContent. A caret thrown
  // back to the top of the document on every colleague's autosave — roughly once
  // a second — makes the editor unusable while anyone else is in it. Note what
  // "stays put" means: the caret keeps its place *in the text*, so it moves when
  // an earlier block grows. Asserting the raw position would assert the opposite.
  it('leaves the caret in the same place in the text', async () => {
    const editor = mountEditor(doc('первый', 'второй абзац'))
    await nextTick()
    // Into the second block, which the remote edit does not touch.
    editor.commands.setTextSelection(editor.state.doc.content.size - 3)
    const offset = editor.state.selection.$from.parentOffset

    // The first block gets longer, so every position after it shifts.
    wrapper.vm.applyRemote(doc('правка коллеги от соседа', 'второй абзац'))
    await nextTick()

    expect(editor.state.selection.$from.parent.attrs.id).toBe('b2')
    expect(editor.state.selection.$from.parentOffset).toBe(offset)
  })

  // Without this, applying an edit schedules a save that announces it back to
  // its author, who applies it and saves it back: two clients resaving the
  // document to each other for as long as both tabs are open.
  it('does not report a remote edit as a local change', async () => {
    const editor = mountEditor(doc('первый'))
    await nextTick()
    const before = wrapper.emitted('change')?.length ?? 0

    wrapper.vm.applyRemote(doc('правка коллеги'))
    await nextTick()

    expect(wrapper.emitted('change')?.length ?? 0).toBe(before)
    // The parent's copy still has to follow, or its next save would send the
    // pre-merge tree and undo the colleague.
    expect(wrapper.emitted('update:modelValue').at(-1)[0].content[0].content[0].text).toBe(
      'правка коллеги',
    )
    expect(textsOf(editor)).toEqual(['правка коллеги'])
  })

  it('keeps a remote edit out of the undo stack', async () => {
    const editor = mountEditor(doc('первый'))
    await nextTick()
    editor.commands.setTextSelection(1)
    editor.commands.insertContent('мой ')
    await nextTick()

    wrapper.vm.applyRemote(doc('правка коллеги'))
    await nextTick()
    editor.commands.undo()
    await nextTick()

    // Undo reached past the colleague's edit to this user's own insertion —
    // which is the point: Ctrl+Z is for your typing, not for theirs.
    expect(textsOf(editor)[0]).not.toBe('мой первый')
  })

  // A colleague reordering the document while you read it is the case the
  // block-by-block path cannot serve: positions move, so the content is swapped
  // wholesale and the selection remapped through it.
  it('takes added and reordered blocks', async () => {
    const editor = mountEditor(doc('первый', 'второй'))
    await nextTick()

    wrapper.vm.applyRemote({
      type: 'doc',
      content: [blk('b2', 'второй'), blk('b3', 'третий'), blk('b1', 'первый')],
    })
    await nextTick()

    expect(idsOf(editor)).toEqual(['b2', 'b3', 'b1'])
    expect(textsOf(editor)).toEqual(['второй', 'третий', 'первый'])
  })

  it('does nothing when the document is already what arrived', async () => {
    const editor = mountEditor(doc('первый'))
    await nextTick()
    const before = wrapper.emitted('update:modelValue')?.length ?? 0

    expect(wrapper.vm.applyRemote(doc('первый'))).toBe(false)
    expect(wrapper.emitted('update:modelValue')?.length ?? 0).toBe(before)
    expect(textsOf(editor)).toEqual(['первый'])
  })
})
