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

// Which block the editor asks the parent to claim (#2729). The claim is a lock
// on a block other people are then kept out of, so *when* it is emitted is the
// whole question: an editor that claims on load turns every reader into a
// squatter on the first paragraph.
function doc(...texts) {
  return {
    type: 'doc',
    content: texts.map((text) => ({
      type: 'paragraph',
      attrs: { id: `b-${text}` },
      content: [{ type: 'text', text }],
    })),
  }
}

// Pretends the editor has keyboard focus (see the note in the second test).
function focus(editor, value = true) {
  Object.defineProperty(editor, 'isFocused', { value, configurable: true })
}

describe('DocEditor block claim', () => {
  let wrapper
  afterEach(() => wrapper?.unmount())

  // The regression this file exists for. DocumentsView mounts the editor first
  // and fills it in once the document arrives, so the load lands as a
  // setContent *transaction* — which is exactly what used to be mistaken for
  // "the user put their caret in the first block". Mounting with the content
  // already in place does not reproduce it: there is no transaction then.
  it('claims nothing when a document is loaded into an unfocused editor', async () => {
    wrapper = mount(DocEditor, { props: { modelValue: null }, attachTo: document.body })
    await nextTick()

    await wrapper.setProps({ modelValue: doc('раз', 'два') })
    await nextTick()

    expect(wrapper.emitted('block-focus')).toBeUndefined()
  })

  it('claims the block the caret is in once focused, and only on a change', async () => {
    wrapper = mount(DocEditor, {
      props: { modelValue: doc('раз', 'два') },
      attachTo: document.body,
    })
    await nextTick()
    const editor = wrapper.vm.editor

    // jsdom will not focus a contenteditable div, so `editor.isFocused` (which
    // reads document.activeElement) can never become true here. Force it: what
    // this test is about is the branch the flag selects, not ProseMirror's focus
    // tracking. The real thing is covered in a browser by e2e/specs/doc-presence.
    focus(editor)
    editor.commands.setTextSelection(2)
    await nextTick()
    expect(wrapper.emitted('block-focus')?.at(-1)).toEqual(['b-раз'])

    // Moving within the same block is not a new claim: the claim doubles as the
    // lock heartbeat, and re-emitting per keystroke would flood the socket.
    const afterFirst = wrapper.emitted('block-focus').length
    editor.commands.setTextSelection(3)
    await nextTick()
    expect(wrapper.emitted('block-focus').length).toBe(afterFirst)

    // Moving to another block hands the lock over.
    editor.commands.setTextSelection(doc('раз').content[0].content[0].text.length + 5)
    await nextTick()
    expect(wrapper.emitted('block-focus').at(-1)).toEqual(['b-два'])
  })

  it('re-claims the same block after a blur', async () => {
    wrapper = mount(DocEditor, {
      props: { modelValue: doc('раз', 'два') },
      attachTo: document.body,
    })
    await nextTick()
    const editor = wrapper.vm.editor

    focus(editor)
    editor.commands.setTextSelection(2)
    await nextTick()
    const claims = wrapper.emitted('block-focus').length

    // The parent releases the block on blur, so returning to it has to claim it
    // again — a "no change since last time" short-circuit would leave the user
    // typing into a block the server thinks is free.
    focus(editor, false)
    editor.commands.setTextSelection(3)
    await nextTick()
    focus(editor)
    editor.commands.setTextSelection(2)
    await nextTick()
    expect(wrapper.emitted('block-focus').length).toBeGreaterThan(claims)
  })
})
