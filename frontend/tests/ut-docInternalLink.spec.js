import { describe, it, expect, afterEach, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import {
  FLASH_MS,
  blockPosById,
  internalLinkKey,
  scrollToBlockId,
} from '@/utils/docExtensions/internalLink'
import { internalHref } from '@/utils/docToc'

// A document whose first paragraph links to the heading further down — the
// shape both the outline and a hand-written cross-reference produce.
function makeEditor({ editable = true } = {}) {
  return new Editor({
    element: document.createElement('div'),
    editable,
    extensions: docExtensions(),
    content: ensureBlockIds({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'к разделу',
              marks: [{ type: 'link', attrs: { href: '#target' } }],
            },
          ],
        },
        {
          type: 'heading',
          attrs: { id: 'target', level: 2 },
          content: [{ type: 'text', text: 'Раздел' }],
        },
      ],
    }),
  })
}

function flashed(editor) {
  return internalLinkKey.getState(editor.state)?.id || ''
}

// The click the plugin listens for, with a DOM node that answers closest().
function clickOn(editor, href, over = {}) {
  const anchor = { getAttribute: () => href }
  const event = {
    target: { closest: (sel) => (sel.startsWith('a[') ? anchor : null) },
    preventDefault: vi.fn(),
    ...over,
  }
  const handled = editor.view.someProp('handleDOMEvents', (fns) =>
    fns.click ? fns.click(editor.view, event) : false,
  )
  return { handled: !!handled, event }
}

describe('internal links', () => {
  let editor
  afterEach(() => {
    editor?.destroy()
    editor = null
    vi.useRealTimers()
  })

  it('finds the block an id points at', () => {
    editor = makeEditor()
    const found = blockPosById(editor.state.doc, 'target')
    expect(found).toBeTruthy()
    expect(found.node.type.name).toBe('heading')
    expect(blockPosById(editor.state.doc, 'нет такого')).toBeNull()
  })

  it('marks the block it jumped to and clears the mark afterwards', () => {
    vi.useFakeTimers()
    editor = makeEditor()
    expect(scrollToBlockId(editor.view, 'target')).toBe(true)
    // A smooth scroll ends somewhere in the middle of the page; without the cue
    // nothing says which block was the point of it.
    expect(flashed(editor)).toBe('target')
    vi.advanceTimersByTime(FLASH_MS + 10)
    expect(flashed(editor)).toBe('')
  })

  it('does not undo a newer jump when the older timer fires', () => {
    vi.useFakeTimers()
    editor = makeEditor()
    const first = editor.state.doc.firstChild.attrs.id
    scrollToBlockId(editor.view, 'target')
    vi.advanceTimersByTime(FLASH_MS - 100)
    scrollToBlockId(editor.view, first)
    // The first timer fires here, and the highlight it would clear is no longer
    // its own — the user is looking at the second jump.
    vi.advanceTimersByTime(200)
    expect(flashed(editor)).toBe(first)
  })

  it('reports a jump to a block that is gone instead of flashing nothing', () => {
    editor = makeEditor()
    expect(scrollToBlockId(editor.view, 'удалённый')).toBe(false)
    expect(flashed(editor)).toBe('')
  })

  it('scrolls the target into view when the DOM can', () => {
    editor = makeEditor()
    const dom = editor.view.nodeDOM(blockPosById(editor.state.doc, 'target').pos)
    dom.scrollIntoView = vi.fn()
    scrollToBlockId(editor.view, 'target')
    expect(dom.scrollIntoView).toHaveBeenCalled()
  })

  it('follows an internal link on a read-only surface', () => {
    editor = makeEditor({ editable: false })
    const { handled, event } = clickOn(editor, internalHref('target'))
    expect(handled).toBe(true)
    // Without preventDefault the bare fragment is a real navigation and pushes a
    // hash onto the SPA's URL.
    expect(event.preventDefault).toHaveBeenCalled()
    expect(flashed(editor)).toBe('target')
  })

  it('leaves a plain click in the editor alone, and follows it with Ctrl', () => {
    editor = makeEditor()
    // A link in text you are writing has to stay editable, so a plain click goes
    // on placing the caret.
    expect(clickOn(editor, '#target').handled).toBe(false)
    expect(flashed(editor)).toBe('')

    expect(clickOn(editor, '#target', { ctrlKey: true }).handled).toBe(true)
    expect(flashed(editor)).toBe('target')
  })

  it('ignores a link that leads outside the document', () => {
    editor = makeEditor({ editable: false })
    for (const href of ['https://example.com', '/documents/other#target', '#']) {
      const { handled, event } = clickOn(editor, href)
      expect(handled).toBe(false)
      expect(event.preventDefault).not.toHaveBeenCalled()
    }
  })
})
