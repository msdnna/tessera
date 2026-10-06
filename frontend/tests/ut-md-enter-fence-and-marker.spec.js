// Enter in the markdown editor, at the two places where the line the caret sits
// on is not the line handleEnter assumed it was:
//
//   • a ``` line that CLOSES a block looks exactly like one that opens it, so
//     Enter after a finished code block appended a third fence — an unpaired
//     opener, which in the preview swallows everything below it into code;
//   • the list marker was carried no matter where in the line the caret stood,
//     so Home-then-Enter (the everyday way to open a line above) copied the
//     marker onto the text it was still attached to: "- - foo".
//
// The controls below pin the behaviour that must NOT change: a real opening
// fence still self-closes, and a caret in the item's content still continues
// the list.

import { describe, it, expect } from 'vitest'
import { handleEnter } from '../src/utils/mdEdit.js'

describe('handleEnter — закрывающий фенс не открывает новый блок', () => {
  it('Enter в конце закрывающего ``` не дописывает фенс', () => {
    // ```js / code / ```| — каретка за последним закрывающим фенсом.
    expect(handleEnter('```js\ncode\n```', 14)).toBeNull()
  })

  it('то же, когда ниже есть текст', () => {
    expect(handleEnter('```js\ncode\n```\n\nхвост', 14)).toBeNull()
  })

  it('закрывающий фенс с отступом тоже распознан', () => {
    expect(handleEnter('  ```\n  code\n  ```', 18)).toBeNull()
  })

  it('фенс без языка: первый открывает, третий закрывает', () => {
    // Первый ``` (ничего не закрывает ниже) — открывающий, блок самозакрывается.
    expect(handleEnter('```', 3).value).toBe('```\n\n```')
    // Он же, но уже с парой ниже — и открывающий, и закрывающий молчат.
    expect(handleEnter('```\ncode\n```', 3)).toBeNull()
    expect(handleEnter('```\ncode\n```', 12)).toBeNull()
  })

  it('второй блок в документе открывается как обычно', () => {
    const v = '```\na\n```\n\n```js'
    expect(handleEnter(v, v.length).value).toBe('```\na\n```\n\n```js\n\n```')
  })

  // ── контроли: открывающий фенс работает как раньше ──
  it('контроль: открывающий фенс с языком самозакрывается', () => {
    expect(handleEnter('```js', 5).value).toBe('```js\n\n```')
  })

  it('контроль: каретка не в конце строки фенса — обычный перевод строки', () => {
    expect(handleEnter('```js', 3)).toBeNull()
  })

  it('контроль: отступ открывающего фенса переносится на закрывающий', () => {
    expect(handleEnter('  ```js', 7).value).toBe('  ```js\n  \n  ```')
  })
})

describe('handleEnter — маркер списка не копируется на себя', () => {
  it('каретка в начале строки списка — обычный перевод строки', () => {
    expect(handleEnter('- foo', 0)).toBeNull()
  })

  it('каретка внутри маркера', () => {
    expect(handleEnter('- foo', 1)).toBeNull()
  })

  it('чекбокс: каретка до содержимого', () => {
    expect(handleEnter('- [ ] foo', 0)).toBeNull()
    expect(handleEnter('- [x] foo', 5)).toBeNull()
  })

  it('нумерованный: каретка до содержимого', () => {
    expect(handleEnter('1. foo', 0)).toBeNull()
    expect(handleEnter('12) foo', 2)).toBeNull()
  })

  it('вложенный пункт: каретка на отступе', () => {
    expect(handleEnter('  - foo', 1)).toBeNull()
  })

  it('пустой пункт: Enter в начале строки не съедает маркер', () => {
    // Список закрывают Enter'ом В КОНЦЕ пустого пункта, а не в его начале.
    expect(handleEnter('- one\n- ', 6)).toBeNull()
  })

  it('строка не первая в документе — позиция считается от начала строки', () => {
    expect(handleEnter('текст\n- foo', 6)).toBeNull()
  })

  // ── контроли: продолжение списка не сломано ──
  it('контроль: каретка сразу за маркером — пункт разрезается', () => {
    expect(handleEnter('- foo', 2)).toEqual({ value: '- \n- foo', start: 5, end: 5 })
  })

  it('контроль: каретка в конце — маркер переносится', () => {
    expect(handleEnter('- foo', 5).value).toBe('- foo\n- ')
    expect(handleEnter('1. one', 6).value).toBe('1. one\n2. ')
    expect(handleEnter('- [x] done', 10).value).toBe('- [x] done\n- [ ] ')
    expect(handleEnter('  - a', 5).value).toBe('  - a\n  - ')
  })

  it('контроль: Enter в конце пустого пункта по-прежнему закрывает список', () => {
    expect(handleEnter('- one\n- ', 8)).toEqual({ value: '- one\n', start: 6, end: 6 })
    expect(handleEnter('1. ', 3)).toEqual({ value: '', start: 0, end: 0 })
  })

  it('контроль: обычный текст — null', () => {
    expect(handleEnter('plain text', 10)).toBeNull()
    expect(handleEnter('plain text', 0)).toBeNull()
  })
})
