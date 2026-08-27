import { describe, it, expect, afterEach } from 'vitest'
import { Editor } from '@tiptap/core'
import { setI18nLocale } from '@/i18n'
import { slashItems } from '@/utils/docSlash'
import { toolbarCommands, alignments, lineHeightLabel } from '@/utils/docToolbar'
import { fontFamilies, docExtensions } from '@/utils/docSchema'
import { untitledHeading } from '@/utils/docToc'
import { builtinCards, builtinContent } from '@/utils/docTemplates'

// Same guard as ut-i18n-board-switch / ut-i18n-task-switch, for the editor
// surfaces of wave 3. Every table here — slash menu, toolbar, alignments, font
// picker, template gallery — used to be a module-level array of Russian
// strings, evaluated once at import. Translating them "in place" would look
// right until the first language switch and then never move again, so these
// specs flip the locale and demand the text follows.

afterEach(async () => {
  await setI18nLocale('ru')
})

describe('editor labels follow a language switch', () => {
  it('re-renders the slash menu, keywords included', async () => {
    const ru = slashItems()
    expect(ru.find((i) => i.key === 'table').label).toBe('Таблица 3×3')
    expect(ru.find((i) => i.key === 'table').group).toBe('Вставка')

    await setI18nLocale('en')
    const en = slashItems()
    expect(en.find((i) => i.key === 'table').label).toBe('Table 3×3')
    expect(en.find((i) => i.key === 'table').group).toBe('Insert')
    // The typing aliases are not translated: "/таблица" has to keep working in
    // an English interface, and "/table" in a Russian one.
    expect(en.find((i) => i.key === 'table').keywords).toContain('таблица')
  })

  it('re-renders the toolbar titles and the alignment labels', async () => {
    const editor = new Editor({
      element: document.createElement('div'),
      extensions: docExtensions(),
      content: { type: 'doc', content: [{ type: 'paragraph' }] },
    })
    const titleOf = (key) => toolbarCommands(editor).find((c) => c.key === key).title
    expect(titleOf('bold')).toBe('Жирный (Ctrl+B)')
    expect(titleOf('h2')).toBe('Заголовок 2')
    expect(alignments()[0].label).toBe('По левому краю')

    await setI18nLocale('en')
    expect(titleOf('bold')).toBe('Bold (Ctrl+B)')
    expect(titleOf('h2')).toBe('Heading 2')
    expect(alignments()[0].label).toBe('Align left')
    editor.destroy()
  })

  it('formats the line height by locale instead of always with a comma', async () => {
    expect(lineHeightLabel('1.5')).toBe('1,5')
    await setI18nLocale('en')
    expect(lineHeightLabel('1.5')).toBe('1.5')
  })

  it('re-renders the font picker and the untitled-heading fallback', async () => {
    expect(fontFamilies().map((f) => f.label)).toEqual([
      'По умолчанию',
      'Системный',
      'С засечками',
      'Моноширинный',
    ])
    expect(untitledHeading()).toBe('Без заголовка')

    await setI18nLocale('en')
    expect(fontFamilies().map((f) => f.label)).toEqual(['Default', 'System', 'Serif', 'Monospaced'])
    // The font stacks are CSS, not text — they must not move with the language.
    expect(fontFamilies()[1].value).toContain('system-ui')
    expect(untitledHeading()).toBe('Untitled heading')
  })

  it('re-renders the built-in template gallery and its bodies', async () => {
    expect(builtinCards().map((c) => c.title)).toEqual([
      'Протокол совещания',
      'Техническое задание',
      'Ретроспектива',
    ])
    expect(JSON.stringify(builtinContent('meeting'))).toContain('Повестка')

    await setI18nLocale('en')
    expect(builtinCards().map((c) => c.title)).toEqual([
      'Meeting notes',
      'Technical spec',
      'Retrospective',
    ])
    // The card id stays keyed, not translated: the gallery and the create call
    // match on it.
    expect(builtinCards()[0].id).toBe('builtin:meeting')
    expect(JSON.stringify(builtinContent('meeting'))).toContain('Agenda')
  })

  // `|` is vue-i18n's plural separator, so the Markdown table in the spec
  // template is escaped as {'|'} in the bundles. Without the escape t() returns
  // only the first plural branch and the table silently disappears — this
  // asserts the escape survives all the way into the parsed document.
  it('keeps the Markdown table in the spec template', () => {
    const json = JSON.stringify(builtinContent('spec'))
    expect(json).toContain('"table"')
    expect(json).toContain('Кто отвечает')
  })
})
