import { describe, it, expect, vi } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { ensureBlockIds } from '@/utils/docExtensions/blockId'
import { toolbarGroups, toolbarCommands, lineHeightLabel } from '@/utils/docToolbar'

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
  const groups = toolbarGroups(makeEditor(), handlers)
  const all = toolbarCommands(makeEditor(), handlers)

  // The panel is one row now, so the groups (and the separators between them)
  // carry the structure the tabs used to. Order is part of the design: history
  // first, insert last.
  it('lays its commands out in groups', () => {
    expect(groups.map((g) => g.key)).toEqual([
      'history',
      'headings',
      'selects',
      'format',
      'align',
      'lists',
      'insert',
    ])
    // The font/size/spacing pickers are naive selects rendered by the component;
    // every other group has to carry its buttons as data or they fall out from
    // under this test.
    for (const g of groups) {
      if (g.kind === 'selects') continue
      expect(g.items.length, `group ${g.key} is empty`).toBeGreaterThan(0)
    }
  })

  it('keeps every list type in one group', () => {
    const lists = groups.find((g) => g.key === 'lists').items.map((t) => t.key)
    expect(lists).toContain('bulletList')
    expect(lists).toContain('orderedList')
    expect(lists).toContain('taskList')
  })

  it.each(all.map((t) => [t.key, t.key]))('%s applies its command', (_label, key) => {
    const editor = makeEditor()
    const onSetLink = vi.fn()
    const onPickImage = vi.fn()
    const tool = toolbarCommands(editor, { onSetLink, onPickImage }).find((t) => t.key === key)
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
    // A button that reports itself disabled has nothing to apply (undo with an
    // empty history) — that is a live button in a legitimate state, not a dead
    // one, and it is checked separately below.
    if (tool.disabled?.()) {
      editor.destroy()
      return
    }
    // Outdent at zero is a legitimate no-op, so give it something to undo.
    if (key === 'outdent') editor.commands.indent()
    expect(tool.run(), `command for ${key} did nothing`).toBe(true)
    expect(typeof tool.isActive()).toBe('boolean')
    editor.destroy()
  })

  it('reports the active state of a mark it just applied', () => {
    const editor = makeEditor()
    const bold = toolbarCommands(editor, handlers).find((t) => t.key === 'bold')
    expect(bold.isActive()).toBe(false)
    bold.run()
    expect(bold.isActive()).toBe(true)
    editor.destroy()
  })

  // Undo is the one button whose enabled state changes from typing rather than
  // from clicking, which is what a stale panel would get wrong.
  it('enables undo only once there is something to undo', () => {
    const editor = new Editor({
      element: document.createElement('div'),
      extensions: docExtensions(),
      content: ensureBlockIds({ type: 'doc', content: [{ type: 'paragraph' }] }),
    })
    const undo = () => toolbarCommands(editor, handlers).find((t) => t.key === 'undo')
    expect(undo().disabled()).toBe(true)
    editor.commands.insertContent('правка')
    expect(undo().disabled()).toBe(false)
    expect(undo().run()).toBe(true)
    editor.destroy()
  })

  it('applies alignment as buttons rather than a list', () => {
    const editor = makeEditor()
    const align = toolbarGroups(editor, handlers).find((g) => g.key === 'align')
    expect(align.items.map((t) => t.key)).toEqual([
      'align-left',
      'align-center',
      'align-right',
      'align-justify',
    ])
    // Each one carries its own pictogram: the four values are what the old
    // 150px select spent its width on.
    expect(align.items.every((t) => t.vicon)).toBe(true)
    const center = align.items.find((t) => t.key === 'align-center')
    expect(center.run()).toBe(true)
    expect(center.isActive()).toBe(true)
    editor.destroy()
  })

  // Shown Russian, stored as written: the value goes straight into the
  // lineHeight attribute, where blockStyle compares it verbatim.
  it('shows line height with a comma but does not change the value', () => {
    expect(lineHeightLabel('1.5')).toBe('1,5')
    expect(lineHeightLabel('2')).toBe('2')
  })
})
