import { describe, it, expect } from 'vitest'
import { fragmentHref, findText, readFragment, scopeOf } from '@/utils/fragmentLink'

describe('fragment links (#3119)', () => {
  const task = { board_id: 'b1', number: 42 }

  it('builds the task link with where the text lives and the text', () => {
    const href = fragmentHref(task, 'c:abc-1', '  две\n строки  ', 'https://t.test')
    expect(href).toBe(
      'https://t.test/board/b1?task=42&frag=c%3Aabc-1&q=%D0%B4%D0%B2%D0%B5+%D1%81%D1%82%D1%80%D0%BE%D0%BA%D0%B8',
    )
    const q = Object.fromEntries(new URL(href).searchParams)
    expect(readFragment(q)).toEqual({ scope: 'c:abc-1', q: 'две строки' })
  })

  it('refuses a scope it does not know', () => {
    expect(readFragment({ frag: 'x<script>' })).toBeNull()
    expect(readFragment({})).toBeNull()
    expect(readFragment({ frag: 'desc' })).toEqual({ scope: 'desc', q: '' })
  })

  it('finds text across inline tags and line breaks, any case', () => {
    const root = document.createElement('div')
    root.innerHTML = '<p>Сначала <b>собрать</b>\n  билд, потом</p><p>выкатить</p>'
    const r = findText(root, 'Собрать билд')
    expect(r.toString().replace(/\s+/g, ' ')).toBe('собрать билд')
    expect(findText(root, 'нет такого')).toBeNull()
  })

  it('knows a comment from the description', () => {
    const root = document.createElement('div')
    root.innerHTML = '<div data-comment-id="c7"><p id="in">x</p></div><p id="out">y</p>'
    expect(scopeOf(root.querySelector('#in').firstChild)).toBe('c:c7')
    expect(scopeOf(root.querySelector('#out'))).toBe('desc')
  })
})
