// Автодополнение в композере: popup и живая каретка (#2932).
//
// `detectSuggest` вызывается ТОЛЬКО из `onInput`, а каретку двигают и без ввода —
// ArrowLeft/Right, Home/End, PageUp/Down, щелчок. Popup при этом оставался открыт
// со старым `start`, и выбор строки вклеивал подсказку поверх диапазона, который
// её уже не содержит: при каретке в начале строки `replaceRange` получал end
// МЕНЬШЕ start и печатал кусок между ними дважды.
//
// Второй класс из того же блока: popup не смотрел на модификаторы и забирал себе
// Ctrl/⌘+Enter (отправка), Shift+Enter (перевод строки) и Shift+Tab (outdent).
import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownEditor from '@/components/MarkdownEditor.vue'

vi.mock('@/api', () => ({
  uploads: { upload: vi.fn() },
  tasks: {
    attachments: vi.fn(() => Promise.resolve({ data: [] })),
    uploadAttachment: vi.fn(),
    downloadAttachment: vi.fn(),
  },
}))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

const MENTIONS = [
  { id: 'u1', label: 'anna', display: 'Анна' },
  { id: 'u2', label: 'andrey', display: 'Андрей' },
]

const COMMANDS = [
  { key: 'due', description: 'Срок', aliases: ['срок'], arg: 'date', builtin: true },
  { key: 'done', description: 'Выполнено', aliases: [], arg: 'none', builtin: true },
]

let wrapper = null

// Незакрытый mount глушит планировщик соседним тестам — размонтируем всегда.
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

function editor(props = {}) {
  wrapper = mount(MarkdownEditor, {
    attachTo: document.body,
    props: {
      modelValue: '',
      mentionItems: MENTIONS,
      'onUpdate:modelValue': async (v) => {
        await wrapper.setProps({ modelValue: v })
        wrapper.find('textarea').element.value = v
      },
      ...props,
    },
    global: { stubs: { RichContent: true, TesseraSpinner: true, UserAvatar: true } },
  })
  return wrapper
}

// Набор текста так, как это делает браузер: сначала значение в DOM, потом input.
async function type(w, value, caret = value.length) {
  const ta = w.find('textarea')
  ta.element.value = value
  ta.element.setSelectionRange(caret, caret)
  await ta.trigger('input')
}

// Перевод каретки БЕЗ ввода — ровно то, чего детектор не видит.
function moveCaret(w, pos, end = pos) {
  w.find('textarea').element.setSelectionRange(pos, end)
}

async function key(w, k, mods = {}) {
  await w.find('textarea').trigger('keydown', { key: k, ...mods })
  await new Promise((r) => setTimeout(r, 0))
}

const text = (w) => w.find('textarea').element.value
const mentionsOpen = (w) => w.findAll('.md2-mention-item:not(.md2-cmd-item)').length > 0
const cmdsOpen = (w) => w.findAll('.md2-cmd-item').length > 0

describe('popup автодополнения и ушедшая каретка', () => {
  it('Home + Enter не дублирует строку и не вставляет упоминание', async () => {
    const w = editor()
    await type(w, 'привет @an')
    expect(mentionsOpen(w)).toBe(true)
    moveCaret(w, 0)
    await key(w, 'Enter')
    // До фикса: «привет @anna привет @an» — кусок [0, start) напечатан дважды.
    expect(text(w)).toBe('привет @an')
    expect(mentionsOpen(w)).toBe(false)
  })

  it('каретка внутри запроса: Enter не режет набранное', async () => {
    const w = editor()
    await type(w, 'привет @andr')
    moveCaret(w, 9) // между «@» и «a»
    await key(w, 'Enter')
    expect(text(w)).toBe('привет @andr')
    expect(mentionsOpen(w)).toBe(false)
  })

  it('каретка ушла вправо за запрос — подсказка не вклеивается', async () => {
    const w = editor()
    await type(w, '@an и ещё текст', 3) // popup открыт на «@an»
    expect(mentionsOpen(w)).toBe(true)
    moveCaret(w, 10) // внутрь «ещё текст»
    await key(w, 'Enter')
    expect(text(w)).toBe('@an и ещё текст')
  })

  it('ArrowLeft закрывает popup сразу, а не к следующей клавише', async () => {
    const w = editor()
    await type(w, 'привет @an')
    expect(mentionsOpen(w)).toBe(true)
    await key(w, 'ArrowLeft')
    expect(mentionsOpen(w)).toBe(false)
  })

  it('Home закрывает popup сразу', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await key(w, 'Home')
    expect(mentionsOpen(w)).toBe(false)
  })

  it('клик по строке popup после ухода каретки ничего не пишет', async () => {
    const w = editor()
    await type(w, 'привет @an')
    moveCaret(w, 0)
    await w.findAll('.md2-mention-item')[0].trigger('mousedown')
    await new Promise((r) => setTimeout(r, 0))
    expect(text(w)).toBe('привет @an')
  })

  it('выделение (а не каретка) тоже снимает выбор', async () => {
    const w = editor()
    await type(w, 'привет @an')
    moveCaret(w, 0, 6) // выделено «привет»
    await key(w, 'Enter')
    expect(text(w)).toBe('привет @an')
  })
})

describe('popup и модификаторы', () => {
  it('Ctrl+Enter при открытом popup отправляет, а не дописывает упоминание', async () => {
    const w = editor()
    await type(w, 'спасибо @an')
    expect(mentionsOpen(w)).toBe(true)
    await key(w, 'Enter', { ctrlKey: true })
    expect(w.emitted('submit')).toBeTruthy()
    expect(text(w)).toBe('спасибо @an')
  })

  it('⌘+Enter ведёт себя так же', async () => {
    const w = editor()
    await type(w, 'спасибо @an')
    await key(w, 'Enter', { metaKey: true })
    expect(w.emitted('submit')).toBeTruthy()
    expect(text(w)).toBe('спасибо @an')
  })

  it('Shift+Enter при открытом popup не выбирает строку', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await key(w, 'Enter', { shiftKey: true })
    expect(text(w)).toBe('привет @an') // перевод строки печатает браузер, не мы
  })

  it('Shift+Tab при открытом popup делает outdent, а не выбор', async () => {
    const w = editor()
    await type(w, '    @an')
    expect(mentionsOpen(w)).toBe(true)
    await key(w, 'Tab', { shiftKey: true })
    expect(text(w)).not.toContain('@anna')
  })
})

describe('обычный путь не задет', () => {
  it('Enter выбирает подсказку', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await key(w, 'Enter')
    expect(text(w)).toBe('привет @anna ')
  })

  it('Tab выбирает подсказку', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await key(w, 'Tab')
    expect(text(w)).toBe('привет @anna ')
  })

  it('ArrowDown переводит выбор на вторую строку', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await key(w, 'ArrowDown')
    expect(mentionsOpen(w)).toBe(true)
    await key(w, 'Enter')
    expect(text(w)).toBe('привет @andrey ')
  })

  it('клик по строке popup при живой каретке вставляет', async () => {
    const w = editor()
    await type(w, 'привет @an')
    await w.findAll('.md2-mention-item')[1].trigger('mousedown')
    await new Promise((r) => setTimeout(r, 0))
    expect(text(w)).toBe('привет @andrey ')
  })

  it('слэш-команда: Enter выбирает', async () => {
    const w = editor({ commandItems: COMMANDS })
    await type(w, '/du')
    expect(cmdsOpen(w)).toBe(true)
    await key(w, 'Enter')
    expect(text(w)).toBe('/due ')
  })

  it('слэш-команда с ушедшей кареткой не вклеивается', async () => {
    const w = editor({ commandItems: COMMANDS })
    await type(w, 'строка\n/du')
    expect(cmdsOpen(w)).toBe(true)
    moveCaret(w, 0)
    await key(w, 'Enter')
    expect(text(w)).toBe('строка\n/du')
  })

  it('Ctrl+Enter без popup отправляет как раньше', async () => {
    const w = editor()
    await type(w, 'обычный текст')
    await key(w, 'Enter', { ctrlKey: true })
    expect(w.emitted('submit')).toBeTruthy()
  })

  it('Escape закрывает popup и не уходит в документ', async () => {
    const w = editor()
    const onDoc = vi.fn()
    document.addEventListener('keydown', onDoc)
    try {
      await type(w, 'привет @an')
      await key(w, 'Escape')
      expect(mentionsOpen(w)).toBe(false)
      expect(onDoc).not.toHaveBeenCalled()
    } finally {
      document.removeEventListener('keydown', onDoc)
    }
  })
})
