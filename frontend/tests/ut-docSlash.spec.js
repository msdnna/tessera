import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { docExtensions } from '@/utils/docSchema'
import { SLASH_ITEMS, filterSlashItems, groupSlashItems } from '@/utils/docSlash'
import { slashRangeAt, slashState } from '@/utils/docExtensions/slashMenu'

function makeEditor(opts = {}) {
  return new Editor({
    element: document.createElement('div'),
    extensions: docExtensions(opts),
    content: { type: 'doc', content: [{ type: 'paragraph' }] },
  })
}

function type(editor, text) {
  editor.commands.insertContent(text)
}

describe('filterSlashItems', () => {
  it('returns everything for an empty query', () => {
    expect(filterSlashItems(SLASH_ITEMS, '')).toHaveLength(SLASH_ITEMS.length)
    expect(filterSlashItems(SLASH_ITEMS, '  ')).toHaveLength(SLASH_ITEMS.length)
  })

  // Both languages have to reach the same entry: the labels are Russian, but
  // "/table" is what fingers trained on Notion type.
  it('matches the Russian label and the English keyword alike', () => {
    expect(filterSlashItems(SLASH_ITEMS, 'табл').map((i) => i.key)).toEqual(['table'])
    expect(filterSlashItems(SLASH_ITEMS, 'table').map((i) => i.key)).toEqual(['table'])
  })

  it('matches inside a word, not just at its start', () => {
    expect(filterSlashItems(SLASH_ITEMS, 'спис').map((i) => i.key)).toEqual([
      'bulletList',
      'orderedList',
      'taskList',
    ])
  })

  it('returns nothing for a query that matches no block', () => {
    expect(filterSlashItems(SLASH_ITEMS, 'кот')).toEqual([])
  })

  // Every non-external entry has to actually produce a block; an entry whose
  // command silently fails is indistinguishable from one that worked. The probe
  // document is an h5 rather than a paragraph so that "Текст" and the three
  // heading levels all have something to change.
  it.each(SLASH_ITEMS.filter((i) => !i.external))('$key inserts its block', (item) => {
    const editor = new Editor({
      element: document.createElement('div'),
      extensions: docExtensions(),
      content: {
        type: 'doc',
        content: [{ type: 'heading', attrs: { level: 5 }, content: [{ type: 'text', text: 'x' }] }],
      },
    })
    const before = editor.getHTML()
    expect(item.apply(editor.chain().focus())).toBe(true)
    expect(editor.getHTML()).not.toBe(before)
    editor.destroy()
  })
})

describe('groupSlashItems', () => {
  it('gives every entry a group', () => {
    expect(SLASH_ITEMS.filter((i) => !i.group)).toEqual([])
  })

  // The whole risk of grouping is losing a block from the menu: it still exists,
  // the filter still finds it, and it is simply never drawn.
  it('keeps every item exactly once', () => {
    const groups = groupSlashItems(SLASH_ITEMS)
    const flat = groups.flatMap((g) => g.items.map((i) => i.key))
    expect(flat).toEqual(SLASH_ITEMS.map((i) => i.key))
  })

  it('preserves the declared order inside a group', () => {
    const groups = groupSlashItems(SLASH_ITEMS)
    const lists = groups.find((g) => g.group === 'Списки')
    expect(lists.items.map((i) => i.key)).toEqual(['bulletList', 'orderedList', 'taskList'])
  })

  // Arrow keys walk the flat list, so a menu drawn in a different order than the
  // cursor moves would make the highlight jump around. Declaration order has to
  // already be group order.
  it('draws the groups in one contiguous run each', () => {
    const groups = groupSlashItems(SLASH_ITEMS)
    expect(groups.map((g) => g.group)).toEqual(['Текст', 'Списки', 'Вставка', 'Загрузка'])
  })

  it('drops a group the filter emptied', () => {
    const groups = groupSlashItems(filterSlashItems(SLASH_ITEMS, 'спис'))
    expect(groups.map((g) => g.group)).toEqual(['Списки'])
  })

  it('survives an empty list', () => {
    expect(groupSlashItems([])).toEqual([])
    expect(groupSlashItems(null)).toEqual([])
  })
})

describe('slashRangeAt', () => {
  let editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => editor.destroy())

  it('finds the range of a slash typed at the start of a block', () => {
    type(editor, '/таб')
    expect(slashRangeAt(editor.state)).toMatchObject({ query: 'таб' })
  })

  // A slash inside a word is a path or a date, not a command.
  it('ignores a slash that follows a non-space character', () => {
    type(editor, 'a/b')
    expect(slashRangeAt(editor.state)).toBeNull()
    editor.commands.clearContent()
    type(editor, '12/08')
    expect(slashRangeAt(editor.state)).toBeNull()
  })

  it('ignores a slash inside a code block', () => {
    editor.chain().focus().toggleCodeBlock().insertContent('/').run()
    expect(slashRangeAt(editor.state)).toBeNull()
  })
})

describe('SlashMenu', () => {
  let editor

  beforeEach(() => {
    editor = makeEditor()
  })

  afterEach(() => editor.destroy())

  it('opens on a typed slash and narrows as the query grows', () => {
    type(editor, '/')
    expect(slashState(editor.state).active).toBe(true)
    expect(slashState(editor.state).items).toHaveLength(SLASH_ITEMS.length)
    type(editor, 'табл')
    expect(slashState(editor.state).items.map((i) => i.key)).toEqual(['table'])
  })

  it('closes when nothing matches', () => {
    type(editor, '/кот')
    expect(slashState(editor.state).active).toBe(false)
  })

  it('cycles the highlighted item in both directions', () => {
    type(editor, '/')
    const count = slashState(editor.state).items.length
    editor.commands.slashNext()
    expect(slashState(editor.state).index).toBe(1)
    editor.commands.slashPrev()
    editor.commands.slashPrev()
    expect(slashState(editor.state).index).toBe(count - 1)
  })

  // Running an item must take the typed "/table" with it — leaving the text
  // behind is the classic slash-menu bug.
  it('replaces the typed command with the block it names', () => {
    type(editor, '/табл')
    expect(editor.commands.slashRun()).toBe(true)
    expect(editor.state.doc.textContent).not.toContain('/')
    expect(editor.isActive('table')).toBe(true)
    expect(slashState(editor.state).active).toBe(false)
  })

  // Deleting the command and inserting the block have to land together, or a
  // single Ctrl+Z removes the block and leaves the "/цитата" text behind.
  it('deletes the command and inserts the block in one transaction', () => {
    type(editor, '/цитата')
    let dispatched = 0
    const count = () => (dispatched += 1)
    editor.on('transaction', count)
    editor.commands.slashRun()
    editor.off('transaction', count)
    expect(dispatched).toBe(1)
    expect(editor.isActive('blockquote')).toBe(true)
    expect(editor.state.doc.textContent).toBe('')
  })

  it('hands external items to the host instead of editing', () => {
    let picked = null
    const host = makeEditor({ onSlashExternal: (item) => (picked = item.key) })
    host.commands.insertContent('/картинка')
    expect(host.commands.slashRun()).toBe(true)
    expect(picked).toBe('image')
    expect(host.state.doc.textContent).toBe('')
    host.destroy()
  })

  // Escape has to stick: recomputing the range on the next keystroke would pop
  // the menu straight back up over the text the user is trying to write.
  it('stays closed after Escape until the caret leaves the command', () => {
    type(editor, '/таб')
    expect(editor.commands.slashClose()).toBe(true)
    expect(slashState(editor.state).active).toBe(false)
    type(editor, 'л')
    expect(slashState(editor.state).active).toBe(false)
    editor.commands.clearContent()
    type(editor, '/таб')
    expect(slashState(editor.state).active).toBe(true)
  })

  // Only typing opens the menu. Clicking into text that already contains a
  // slash must not turn it into a command.
  it('does not open when the caret merely moves next to an existing slash', () => {
    editor.commands.setContent({
      type: 'doc',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'путь /таб' }] }],
    })
    editor.commands.setTextSelection(1)
    editor.commands.setTextSelection(editor.state.doc.content.size - 1)
    expect(slashRangeAt(editor.state)).not.toBeNull()
    expect(slashState(editor.state).active).toBe(false)
  })
})
