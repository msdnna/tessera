// Property pickers of a task must be operable from the keyboard (#2932).
//
// Every field of a card/task — priority, estimate, milestone, assignees, tags,
// column, parent, move-to-board — is edited through an `n-popover` whose rows
// are plain `<div class="menu-item">`: no role, no tabindex, no key handler. The
// trigger is a real `<button>`, so Enter opens the picker — and then there is
// nothing to choose with. Tabbing in is impossible by construction, too: the
// popover body is teleported to <body>, and inside the task modal vueuc's
// FocusTrap sends any focus landing there back to the modal's first element.
//
// So the fix can't be «add tabindex»: focus stays on the trigger and a roving
// highlight is driven from a document listener (components/PopMenu.vue). These
// specs pin that from both ends — the real pickers of a board card, and the
// Esc/modal interplay the naive stack gives us (an `n-popover` only answers Esc
// when it traps focus, which these don't, so an un-eaten Esc closes the task
// modal instead of the menu).
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { NModal, NPopover } from 'naive-ui'
import { seedBoardStore } from './helpers/boardStore'
import PopMenu from '@/components/PopMenu.vue'
import TaskCardPills from '@/components/task/TaskCardPills.vue'

const update = vi.fn(() => Promise.resolve({ data: {} }))
const addAssignee = vi.fn(() => Promise.resolve({ data: {} }))
vi.mock('@/api', () => ({
  tasks: {
    update: (...a) => update(...a),
    addAssignee: (...a) => addAssignee(...a),
    removeAssignee: vi.fn(() => Promise.resolve({ data: {} })),
    description: vi.fn(() => Promise.resolve({ data: { description: '' } })),
    addTag: vi.fn(() => Promise.resolve({ data: {} })),
  },
  projects: { createTag: vi.fn(() => Promise.resolve({ data: {} })) },
}))

const TASK = {
  id: 't1',
  board_id: 'b1',
  column_id: 'c1',
  title: 'Задача',
  priority: 0,
  assignee_ids: [],
  tag_ids: [],
}
const MEMBERS = [
  { user_id: 'u1', name: 'Алиса' },
  { user_id: 'u2', name: 'Борис' },
  { user_id: 'u3', name: 'Виктор' },
]

// The picker lives in a teleported popover, so it is queried off document.body
// rather than off the wrapper.
const menuRows = () => Array.from(document.body.querySelectorAll('.menu .menu-item'))
const highlighted = () => document.body.querySelector('.menu .kb-on')

function press(key, target = document.body) {
  const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
  target.dispatchEvent(e)
  return e
}

async function settle() {
  for (let i = 0; i < 4; i++) await nextTick()
}

let wrapper = null

describe('клавиатура в выпадающих меню свойств задачи', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    seedBoardStore({ columns: [{ id: 'c1', name: 'К работе', position: 1 }], members: MEMBERS })
    update.mockClear()
    addAssignee.mockClear()
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    document.body.innerHTML = ''
  })

  async function openPills(selector) {
    wrapper = mount(TaskCardPills, { props: { task: TASK }, attachTo: document.body })
    await wrapper.find(selector).trigger('click')
    await settle()
  }

  it('стрелка + Enter выбирают приоритет в меню карточки', async () => {
    await openPills('[data-tour="card-priority"]')
    expect(menuRows().length).toBe(5) // нет … срочный

    press('ArrowDown')
    press('ArrowDown')
    press('ArrowDown')
    await settle()
    expect(highlighted()?.textContent.trim()).toBe(menuRows()[2].textContent.trim())

    press('Enter')
    await settle()
    expect(update).toHaveBeenCalledTimes(1)
    expect(update.mock.calls[0][1]).toMatchObject({ priority: 2 })
  })

  it('стрелка вверх с нетронутого меню берёт последний пункт', async () => {
    await openPills('[data-tour="card-priority"]')
    press('ArrowUp')
    press('Enter')
    await settle()
    expect(update.mock.calls[0][1]).toMatchObject({ priority: 4 })
  })

  it('в поле фильтра исполнителей стрелка водит по списку, а пробел печатается', async () => {
    await openPills('.assignee-pill')
    const input = document.body.querySelector('.assignee-menu input')
    expect(input).toBeTruthy()
    expect(menuRows().length).toBe(3)

    // Пробел в поле ввода — это текст, а не «выбрать»: меню его не трогает.
    const space = press(' ', input)
    expect(space.defaultPrevented).toBe(false)
    expect(addAssignee).not.toHaveBeenCalled()

    // Стрелка в поле ввода — наоборот, ведёт по списку (иначе фильтровать незачем).
    const down = press('ArrowDown', input)
    expect(down.defaultPrevented).toBe(true)
    press('Enter', input)
    await settle()
    expect(addAssignee).toHaveBeenCalledWith('t1', 'u1')
  })
})

// The host reproduces the real nesting: a task modal (with its focus trap) and a
// property popover inside it.
const Host = {
  components: { NModal, NPopover, PopMenu },
  setup() {
    const picked = ref(null)
    const show = ref(true)
    return { picked, show }
  },
  template: `
    <n-modal v-model:show="show">
      <div class="modal-body">
        <n-popover trigger="click" :show="true">
          <template #trigger><button class="trig">приоритет</button></template>
          <PopMenu>
            <div class="menu-item" @click="picked = 'a'">Первый</div>
            <div class="menu-item" @click="picked = 'b'">Второй</div>
          </PopMenu>
        </n-popover>
      </div>
    </n-modal>`,
}

describe('Esc в открытом меню', () => {
  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    document.body.innerHTML = ''
  })

  it('гасится на меню и не доходит до модалки задачи', async () => {
    wrapper = mount(Host, { attachTo: document.body })
    await settle()
    expect(menuRows().length).toBe(2)

    const esc = press('Escape')
    await settle()
    // Ключевое: Esc потрачен на меню. Без этого naive закрывает саму модалку —
    // n-popover отвечает на Esc только когда сам держит фокус (Popover.mjs).
    expect(esc.defaultPrevented).toBe(true)
    expect(wrapper.vm.show).toBe(true)
  })

  it('Enter в соседнем поле «создать» достаётся полю, а не подсветке', async () => {
    // Поповер этапа/тегов держит список и поле создания рядом: поле лежит ВНЕ
    // списка, и его @keydown.enter не должен съедаться подсветкой в списке.
    const WithBox = {
      components: { PopMenu },
      data: () => ({ picked: null, created: false }),
      template: `
        <div>
          <PopMenu><div class="menu-item" @click="picked = 'a'">Этап</div></PopMenu>
          <input class="newbox" @keydown.enter="created = true" />
        </div>`,
    }
    wrapper = mount(WithBox, { attachTo: document.body })
    await settle()
    press('ArrowDown')
    await settle()
    expect(document.body.querySelector('.kb-on')).toBeTruthy()

    press('Enter', document.body.querySelector('.newbox'))
    await settle()
    expect(wrapper.vm.created).toBe(true)
    expect(wrapper.vm.picked).toBe(null)
  })

  it('на стрелку отвечает только последнее открытое меню', async () => {
    // Оба меню слушают document; без стека одна стрелка водила бы по обоим.
    const Nested = {
      components: { PopMenu },
      template: `
        <div>
          <PopMenu class="outer"><div class="menu-item">Внешний</div></PopMenu>
          <PopMenu class="inner"><div class="menu-item">Внутренний</div></PopMenu>
        </div>`,
    }
    wrapper = mount(Nested, { attachTo: document.body })
    await settle()
    press('ArrowDown')
    await settle()
    expect(document.body.querySelectorAll('.kb-on').length).toBe(1)
    expect(document.body.querySelector('.inner .kb-on')).toBeTruthy()
  })

  it('после закрытия меню клавиши ему больше не отвечают', async () => {
    wrapper = mount(Host, { attachTo: document.body })
    await settle()
    wrapper.unmount()
    wrapper = null
    await settle()
    const e = press('ArrowDown')
    expect(e.defaultPrevented).toBe(false)
  })
})
