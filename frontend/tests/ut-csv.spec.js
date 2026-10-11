import { describe, it, expect } from 'vitest'
import { csvCell, toCSV } from '@/utils/csv'

describe('csv', () => {
  it('quotes separators, quotes and line breaks', () => {
    expect(csvCell('Иван')).toBe('Иван')
    expect(csvCell('a,b')).toBe('"a,b"')
    expect(csvCell('say "hi"')).toBe('"say ""hi"""')
    expect(csvCell('two\nlines')).toBe('"two\nlines"')
    expect(csvCell(null)).toBe('')
  })

  it('defuses formulas', () => {
    expect(csvCell('=HYPERLINK("x")')).toBe(`"'=HYPERLINK(""x"")"`)
    expect(csvCell('-1')).toBe("'-1")
    expect(csvCell('@me')).toBe("'@me")
  })

  it('writes a BOM, a header and CRLF rows', () => {
    const text = toCSV(
      [{ n: 'Анна', e: 'a@x' }],
      [
        { label: 'Имя', value: (r) => r.n },
        { label: 'Email', value: (r) => r.e },
      ],
    )
    expect(text).toBe('﻿Имя,Email\r\nАнна,a@x\r\n')
  })
})
