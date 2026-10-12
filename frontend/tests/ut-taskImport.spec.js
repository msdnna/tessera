import { describe, it, expect } from 'vitest'
import {
  parseCSV,
  sniffDelimiter,
  parseImportFile,
  autoMapping,
  parsePriority,
  parseDate,
  previewRows,
} from '@/utils/taskImport'

describe('utils/taskImport (#3095)', () => {
  it('sniffs the delimiter outside quotes', () => {
    expect(sniffDelimiter('a;b;"c,d"\n1;2;3')).toBe(';')
    expect(sniffDelimiter('a,b\n')).toBe(',')
    expect(sniffDelimiter('a\tb\tc')).toBe('\t')
  })

  it('reads quoted cells, doubled quotes and newlines inside quotes', () => {
    const rows = parseCSV('t,d\n"a, b","say ""hi""\nnext"\r\nc,\n', ',')
    expect(rows).toEqual([
      ['t', 'd'],
      ['a, b', 'say "hi"\nnext'],
      ['c', ''],
    ])
  })

  it('parses a CSV with a BOM and skips blank rows', () => {
    const t = parseImportFile('\uFEFFНазвание;Метки\nПервая;x, y\n;\nВторая;\n', 'a.csv')
    expect(t.columns).toEqual(['Название', 'Метки'])
    expect(t.rows).toHaveLength(2)
  })

  it('parses our own JSON export', () => {
    const t = parseImportFile('{"tasks":[{"title":"A","tags":["x","y"],"priority":"high"}]}')
    expect(t.rows[0]).toEqual({ priority: 'high', tags: 'x, y', title: 'A' })
    expect(() => parseImportFile('{"x":1}', 'a.json')).toThrow()
  })

  it('maps columns by name in either language', () => {
    expect(autoMapping(['Название', 'Статус', 'Метки', 'Срок', 'whatever'])).toEqual({
      title: 'Название',
      column: 'Статус',
      tags: 'Метки',
      due_date: 'Срок',
    })
    expect(autoMapping(['number', 'title', 'due_date']).title).toBe('title')
  })

  it('reads priorities and dates the way the server does', () => {
    expect(parsePriority('Высокий')).toBe(3)
    expect(parsePriority('4')).toBe(4)
    expect(parsePriority('')).toBe(0)
    expect(parsePriority('meh')).toBeNull()
    expect(parseDate('5.11.2026')).toBe('2026-11-05')
    expect(parseDate('2026-11-05T10:00:00Z')).toBe('2026-11-05')
    expect(parseDate('tomorrow')).toBeNull()
  })

  it('flags what the import would warn about', () => {
    const rows = [
      { T: 'A', C: 'Нет такой', P: 'x', W: 'nobody@x' },
      { T: '', C: 'К работе', P: 'low', W: 'me@x' },
    ]
    const out = previewRows(
      rows,
      { title: 'T', column: 'C', priority: 'P', assignees: 'W' },
      { columns: [{ name: 'К работе' }], members: [{ email: 'me@x', name: 'Me' }] },
    )
    expect(out[0].warn.sort()).toEqual(['assignee', 'column', 'priority'])
    expect(out[1].warn).toEqual(['title'])
  })
})
