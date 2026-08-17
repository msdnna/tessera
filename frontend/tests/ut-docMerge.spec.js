import { describe, it, expect } from 'vitest'
import { mergeRemoteBlocks, blockOrderChanged } from '@/utils/docMerge'

// A one-paragraph block with a stable id — the shape the editor stamps on load.
const block = (id, text) => ({
  type: 'paragraph',
  attrs: { id },
  content: [{ type: 'text', text }],
})
const doc = (...blocks) => ({ type: 'doc', content: blocks })
const textOf = (merged, id) =>
  merged.content.find((b) => b.attrs.id === id)?.content?.[0]?.text ?? null

describe('mergeRemoteBlocks', () => {
  it('takes the server version of a block nobody here is holding', () => {
    const local = doc(block('a', 'наш старый текст'), block('b', 'второй'))
    const remote = doc(block('a', 'правка коллеги'), block('b', 'второй'))

    const merged = mergeRemoteBlocks(local, remote)
    expect(textOf(merged, 'a')).toBe('правка коллеги')
  })

  // The point of the whole exercise: the block under the caret must survive a
  // colleague's save, or every autosave of theirs eats the word being typed.
  it('keeps the local text of a held block', () => {
    const local = doc(block('a', 'коллега пишет'), block('b', 'я печатаю прямо сейчас'))
    const remote = doc(block('a', 'коллега дописал'), block('b', 'старое содержимое'))

    const merged = mergeRemoteBlocks(local, remote, { keepIds: ['b'] })
    expect(textOf(merged, 'a')).toBe('коллега дописал')
    expect(textOf(merged, 'b')).toBe('я печатаю прямо сейчас')
  })

  it('follows the server for which blocks exist and in what order', () => {
    const local = doc(block('a', 'первый'), block('b', 'второй'))
    const remote = doc(block('b', 'второй'), block('c', 'новый'), block('a', 'первый'))

    const merged = mergeRemoteBlocks(local, remote)
    expect(merged.content.map((b) => b.attrs.id)).toEqual(['b', 'c', 'a'])
  })

  it('drops a block the server no longer has', () => {
    const local = doc(block('a', 'первый'), block('b', 'удалён коллегой'))
    const remote = doc(block('a', 'первый'))

    const merged = mergeRemoteBlocks(local, remote)
    expect(merged.content.map((b) => b.attrs.id)).toEqual(['a'])
  })

  // A paragraph started since the last save is not in the server's copy yet.
  // Letting the server's list delete it would erase text in front of the person
  // typing it — the one outcome this merge exists to prevent.
  it('keeps a held block the server has never seen, in its local place', () => {
    const local = doc(block('a', 'первый'), block('new', 'только что начал'), block('b', 'второй'))
    const remote = doc(block('a', 'первый'), block('b', 'второй'))

    const merged = mergeRemoteBlocks(local, remote, { keepIds: ['new'] })
    expect(merged.content.map((b) => b.attrs.id)).toEqual(['a', 'new', 'b'])
    expect(textOf(merged, 'new')).toBe('только что начал')
  })

  // Refusing is the safe answer: without ids there is nothing to merge *by*, and
  // a guess would silently overwrite one side. The caller reloads instead.
  it('refuses when a block cannot be addressed', () => {
    const withoutId = { type: 'paragraph', content: [{ type: 'text', text: 'из импорта' }] }
    expect(mergeRemoteBlocks(doc(withoutId), doc(block('a', 'x')))).toBeNull()
    expect(mergeRemoteBlocks(doc(block('a', 'x')), doc(withoutId))).toBeNull()
  })

  it('refuses anything that is not a document', () => {
    expect(mergeRemoteBlocks(null, doc(block('a', 'x')))).toBeNull()
    expect(mergeRemoteBlocks(doc(block('a', 'x')), undefined)).toBeNull()
    expect(mergeRemoteBlocks({ type: 'paragraph' }, doc(block('a', 'x')))).toBeNull()
  })
})

describe('blockOrderChanged', () => {
  it('is false when only the text inside blocks differs', () => {
    expect(blockOrderChanged(doc(block('a', 'до')), doc(block('a', 'после')))).toBe(false)
  })

  it('is true when a block is added, removed or moved', () => {
    expect(blockOrderChanged(doc(block('a', 'x')), doc(block('a', 'x'), block('b', 'y')))).toBe(
      true,
    )
    expect(blockOrderChanged(doc(block('a', 'x'), block('b', 'y')), doc(block('a', 'x')))).toBe(
      true,
    )
    expect(
      blockOrderChanged(
        doc(block('a', 'x'), block('b', 'y')),
        doc(block('b', 'y'), block('a', 'x')),
      ),
    ).toBe(true)
  })
})
