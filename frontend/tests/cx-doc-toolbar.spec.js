import { describe, it, expect, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import { homeTools, insertTools } from '@/utils/docToolbar'

// "Dead toolbar buttons" was one of the three reasons TipTap was removed from
// this project the first time (ed63159), and which button it was is not
// recorded anywhere. Rather than guess, every button is exercised here: a
// command that is not wired up either throws (the extension is missing) or
// returns false (it could not apply), and both fail this test.

function makeEditor() {
  const editor = new Editor({
    element: document.createElement('div'),
    extensions: docExtensions(),
    content: ensureBlockIds({
      type: 'doc',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'текст документа' }] }],
    }),
  })
  editor.commands.selectAll()
  return editor
}

describe('document toolbar', () => {
  const handlers = { onSetLink: vi.fn(), onPickImage: vi.fn() }
  const all = [
    ...homeTools(makeEditor(), handlers).map((t) => ['Главная', t]),
    ...insertTools(makeEditor(), handlers).map((t) => ['Вставка', t]),
  ]

  it('exposes both tabs', () => {
    expect(all.some(([tab]) => tab === 'Главная')).toBe(true)
    expect(all.some(([tab]) => tab === 'Вставка')).toBe(true)
  })

  it.each(all.map(([tab, t]) => [`${tab}/${t.key}`, t.key]))(
    '%s applies its command',
    (_label, key) => {
      const editor = makeEditor()
      const onSetLink = vi.fn()
      const onPickImage = vi.fn()
      const tools = [...homeTools(editor, { onSetLink }), ...insertTools(editor, { onPickImage })]
      const tool = tools.find((t) => t.key === key)
      expect(tool, `tool ${key} is missing`).toBeTruthy()
      // Every button must expose a tooltip: the icon-only ones are otherwise
      // unlabelled for both users and screen readers.
      expect(tool.title).toBeTruthy()

      if (tool.external) {
        tool.run()
        expect(onSetLink.mock.calls.length + onPickImage.mock.calls.length).toBe(1)
        editor.destroy()
        return
      }
      // Outdent at zero is a legitimate no-op, so give it something to undo.
      if (key === 'outdent') editor.commands.indent()
      expect(tool.run(), `command for ${key} did nothing`).toBe(true)
      expect(typeof tool.isActive()).toBe('boolean')
      editor.destroy()
    },
  )

  it('reports the active state of a mark it just applied', () => {
    const editor = makeEditor()
    const bold = homeTools(editor, handlers).find((t) => t.key === 'bold')
    expect(bold.isActive()).toBe(false)
    bold.run()
    expect(bold.isActive()).toBe(true)
    editor.destroy()
  })
})
