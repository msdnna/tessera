// Поле «№ или название» во вкладке «Связи» и Enter по набранному (#2932).
//
// Поле под списком связей обещает два способа адресации — номер ИЛИ название, — и
// пикер под ним честно фильтрует по обоим: строки в нём сгруппированы по проекту
// и доске, потому что задач в пространстве много и номера наизусть никто не
// помнит. А уходил в запрос только `Number(relNumber)`: набранное название
// давало NaN, `if (!n) return` съедал нажатие, и ни Enter, ни «Связать» не делали
// РОВНО НИЧЕГО — ни запроса, ни сообщения, ни открытого списка. Тупик ровно на
// том вводе, который поле и рекламирует. Туда же уезжала вставка «#2932»: «#» в
// поле нарисован префиксом, но скопированный номер несёт его с собой.
//
// Здесь пинится разбор набранного ДО запроса: «#123» это 123, а название,
// совпавшее ровно с одной задачей, связывает именно её (несколько совпадений —
// открываем отфильтрованный список, пустое поле — тоже список, но связывать
// наугад нечего).
//
// Рядом — вторая половина той же дороги: связь с уже связанной задачей ТЕМ ЖЕ
// видом. Ключ связи это (пара, вид), вставка идёт ON CONFLICT DO NOTHING — запрос
// возвращался 201, не сделав ничего: список не менялся, объяснения не было, а в
// историю ОБЕИХ задач падала вторая строка «добавил связь». Такие задачи убраны
// из пикера (как это давно делает DocLinks) и отсекаются при наборе номера — но
// ровно по своему виду: та же пара с другим видом связи это другая связь.
//
// И гвард «запрос в полёте»: поле очищается только по факту ответа, так что
// нетерпеливый второй Enter слал второе создание.
//
// С пакета J (#3118) форма — это RelationPicker в рамке PropPicker: совпадения
// стоят строками под полем, ↵ связывает строку под курсором (лучшее совпадение —
// выбор на экране, а не догадка) или, если указать не на что, набранный номер;
// после связи форма остаётся открытой для следующей.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

const error = vi.fn()
const info = vi.fn()
const warning = vi.fn()
vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error, info, warning, success: vi.fn() }),
  }
})
// The message API moved out of naive (composables/useMessage, #3117): route it
// to the naive mock above, where this spec keeps its spies.
vi.mock('@/composables/useMessage', async () => ({
  useMessage: (await import('naive-ui')).useMessage,
}))

const addRelation = vi.fn()
const relations = vi.fn()
const removeRelation = vi.fn()
const wsTasks = vi.fn()
vi.mock('@/api', () => ({
  tasks: {
    addRelation: (...a) => addRelation(...a),
    relations: (...a) => relations(...a),
    removeRelation: (...a) => removeRelation(...a),
  },
  workspaces: { tasks: (...a) => wsTasks(...a) },
}))

const TASK = 't-1'
// Задачи пространства: две с «оплат» в названии и одна уникальная.
const WS_TASKS = [
  { id: 't-2', number: 11, title: 'Оплата картой', project_name: 'P', board_name: 'B' },
  { id: 't-3', number: 12, title: 'Оплата по счёту', project_name: 'P', board_name: 'B' },
  { id: 't-4', number: 13, title: 'Экспорт в PDF', project_name: 'P', board_name: 'B' },
]

function deferred() {
  let resolve
  const promise = new Promise((res) => {
    resolve = res
  })
  return { promise, resolve }
}

let w = null
const flush = () => new Promise((r) => setTimeout(r, 0))
async function tick(n = 4) {
  for (let i = 0; i < n; i++) await w.vm.$nextTick()
}
async function openForm(rels = []) {
  const RelationPicker = (await import('@/components/task/props/RelationPicker.vue')).default
  w = mount(RelationPicker, {
    props: {
      show: false,
      anchor: () => document.body,
      taskId: TASK,
      relations: rels,
      wsId: 'ws-1',
    },
    attachTo: document.body,
  })
  await w.setProps({ show: true })
  await tick(6)
  await flush()
  await tick()
  return w
}
const input = () => document.querySelector('[data-testid="rel-number"]')
const shown = () =>
  [...document.querySelectorAll('[data-testid="rel-pick"] .pp-mono')].map((n) =>
    Number(n.textContent.replace('#', '')),
  )
async function type(text) {
  input().value = text
  input().dispatchEvent(new Event('input', { bubbles: true }))
  await tick()
}
async function key(k, extra = {}) {
  input().dispatchEvent(
    new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...extra }),
  )
  await tick()
}
// Набрать в поле и нажать Enter.
async function enter(text) {
  await type(text)
  await key('Enter')
  await flush()
  await tick()
}

beforeEach(() => {
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})
afterEach(() => {
  w?.unmount()
  w = null
  document.body.innerHTML = ''
})

describe('форма «Связать»: набранное в поле «№ или название»', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    addRelation.mockResolvedValue({ data: {} })
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('название, совпавшее ровно с одной задачей, связывает именно её', async () => {
    await openForm()
    await enter('экспорт')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'relates')
  })

  it('несколько совпадений: список отфильтрован, ↵ связывает строку под курсором', async () => {
    await openForm()
    await type('оплат')
    expect(shown()).toEqual([11, 12])
    await key('ArrowDown')
    await key('Enter')
    expect(addRelation).toHaveBeenCalledWith(TASK, 12, 'relates')
  })

  it('название, не совпавшее ни с чем, говорит об этом вместо тишины', async () => {
    await openForm()
    await enter('такого нет')
    expect(addRelation).not.toHaveBeenCalled()
    expect(warning).toHaveBeenCalled()
  })

  it('вставленный «#13» читается как номер 13', async () => {
    await openForm()
    await enter('#13')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'relates')
  })

  it('обычный путь: набранный номер по-прежнему связывает', async () => {
    await openForm()
    await enter('42')
    expect(addRelation).toHaveBeenCalledWith(TASK, 42, 'relates')
  })

  it('пустое поле не связывает ничего', async () => {
    wsTasks.mockResolvedValue({ data: [] })
    await openForm()
    await enter('   ')
    expect(addRelation).not.toHaveBeenCalled()
  })

  it('после связи форма остаётся открытой, поле пустое', async () => {
    await openForm()
    await enter('#13')
    expect(input().value).toBe('')
    expect(w.emitted('update:show')).toBeFalsy()
    expect(w.emitted('update:relations')).toBeTruthy()
  })
})

describe('форма «Связать»: задача, уже связанная этим видом', () => {
  const LINKED = [
    { id: 'r-1', kind: 'relates', related_number: 13, related_title: 'Экспорт в PDF' },
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    addRelation.mockResolvedValue({ data: {} })
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('набранный номер уже связанной задачи не уходит в запрос', async () => {
    await openForm(LINKED)
    await enter('13')
    expect(addRelation).not.toHaveBeenCalled()
    expect(info).toHaveBeenCalled()
    expect(input().value).toBe('')
  })

  it('та же задача с ДРУГИМ видом (⌥2) — это другая связь, запрос уходит', async () => {
    await openForm(LINKED)
    await key('2', { code: 'Digit2', altKey: true })
    await enter('13')
    expect(addRelation).toHaveBeenCalledWith(TASK, 13, 'blocks')
  })

  it('уже связанной этим видом задачи нет в списке, с другим видом — есть', async () => {
    await openForm(LINKED)
    expect(shown()).toEqual([11, 12])
    await key('2', { code: 'Digit2', altKey: true })
    expect(shown()).toEqual([11, 12, 13])
  })
})

describe('форма «Связать»: второй Enter по летящему запросу', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    relations.mockResolvedValue({ data: [] })
    wsTasks.mockResolvedValue({ data: WS_TASKS })
  })

  it('шлёт ровно одно создание', async () => {
    const sent = deferred()
    addRelation.mockReturnValue(sent.promise)
    await openForm()
    await type('13')
    await key('Enter')
    await key('Enter') // нетерпение: поле ещё не очищено
    expect(addRelation).toHaveBeenCalledTimes(1)

    sent.resolve({ data: {} })
    await flush()
    await tick()
    addRelation.mockResolvedValue({ data: {} })
    await enter('11')
    expect(addRelation).toHaveBeenCalledTimes(2)
  })
})
