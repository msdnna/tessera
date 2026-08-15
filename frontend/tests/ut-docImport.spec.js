import { describe, it, expect } from 'vitest'
import { fileToTemplate, firstHeading, markdownToDoc, parseDocJSON } from '@/utils/docImport'
import { ALLOWED_NODES } from '@/utils/docSchema'

// Uploading a ready-made template (#2734). The importer is the one place in the
// document section that takes a file from outside the editor, so these tests
// care about two things: that a supported file becomes a document the editor
// can hold, and that an unsupported one fails with something a person can act
// on rather than with a 400 from the server.

function types(node, out = []) {
  if (!node) return out
  out.push(node.type)
  for (const child of node.content || []) types(child, out)
  return out
}

// Minimal stand-in for the File the browser hands a change handler. jsdom has
// File, but not File.prototype.text in every version — and the importer only
// uses name/size/text().
function fakeFile(name, text) {
  return { name, size: text.length, text: async () => text }
}

describe('markdownToDoc', () => {
  it('maps markdown structure onto document nodes', () => {
    const doc = markdownToDoc('# Заголовок\n\nАбзац\n\n- один\n- два\n')
    expect(doc.type).toBe('doc')
    const seen = types(doc)
    expect(seen).toContain('heading')
    expect(seen).toContain('bulletList')
    expect(seen).toContain('paragraph')
  })

  it('produces only nodes the schema allows', () => {
    const doc = markdownToDoc('# Т\n\n> цитата\n\n```js\nconst a = 1\n```\n\n- [ ] дело\n')
    const unknown = [...new Set(types(doc))].filter((t) => !ALLOWED_NODES.includes(t))
    expect(unknown).toEqual([])
  })

  // Every block carries an id: locks (#2729) and annotations (#2730) hang off
  // it, so a document imported without ids would be uncommentable until it was
  // opened, edited and saved.
  it('stamps block ids', () => {
    const doc = markdownToDoc('первый\n\nвторой\n')
    const ids = (doc.content || []).map((n) => n.attrs?.id)
    expect(ids.filter(Boolean)).toHaveLength(ids.length)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('drops script tags smuggled through inline html', () => {
    const doc = markdownToDoc('текст <script>alert(1)</script>\n')
    expect(types(doc)).not.toContain('script')
    expect(JSON.stringify(doc)).not.toContain('alert(1)')
  })

  it('survives an empty file', () => {
    expect(markdownToDoc('').type).toBe('doc')
  })
})

describe('parseDocJSON', () => {
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [] }] }

  it('reads a bare document', () => {
    expect(parseDocJSON(JSON.stringify(doc)).content.type).toBe('doc')
  })

  it('reads the envelope the gallery exports', () => {
    const out = parseDocJSON(JSON.stringify({ title: 'Протокол', icon: '📋', content: doc }))
    expect(out.title).toBe('Протокол')
    expect(out.icon).toBe('📋')
    expect(out.content.type).toBe('doc')
  })

  it('rejects json that is not a document', () => {
    expect(() => parseDocJSON('{"a":1}')).toThrow(/документа/i)
  })

  it('rejects a file that is not json', () => {
    expect(() => parseDocJSON('# просто markdown')).toThrow(/JSON/i)
  })
})

describe('fileToTemplate', () => {
  it('names a markdown template after its first heading', async () => {
    const out = await fileToTemplate(fakeFile('notes.md', '# Протокол встречи\n\nтекст\n'))
    expect(out.title).toBe('Протокол встречи')
    expect(out.content.type).toBe('doc')
  })

  it('falls back to the file name when the markdown has no heading', async () => {
    const out = await fileToTemplate(fakeFile('черновик.md', 'просто текст\n'))
    expect(out.title).toBe('черновик')
  })

  it('refuses an unsupported extension', async () => {
    await expect(fileToTemplate(fakeFile('spec.docx', 'PK'))).rejects.toThrow(/\.md/)
  })

  it('refuses a file over the size cap', async () => {
    const big = { name: 'big.md', size: 5 * 1024 * 1024, text: async () => '' }
    await expect(fileToTemplate(big)).rejects.toThrow(/2 МБ/)
  })
})

describe('firstHeading', () => {
  it('takes the first heading of any level', () => {
    expect(firstHeading('текст\n\n## Второй уровень\n')).toBe('Второй уровень')
  })

  it('ignores a hash that is not a heading', () => {
    expect(firstHeading('#нетпробела\n')).toBe('')
  })
})
