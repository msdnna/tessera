// Стрелки зависимостей в гантте против УСТАРЕВШЕГО ответа (#2932).
//
// Список рёбер — отдельный запрос (/boards/:id/dependencies), и его переспрашивают
// на каждое изменение состава задач (а состав меняет ЛЮБОЙ фильтр доски: набор в
// поиске, чип исполнителя, «только мои») и на каждое событие о связи. То есть
// несколько запросов в полёте — обычный режим работы вью, а не редкость.
//
// loadDeps присваивал deps.value после await без единой сверки «мой ли это ответ»,
// а его catch — и вовсе стирал список целиком. Отсюда три наблюдаемых поломки:
// снимок покинутого фильтра перерисовывал стрелки, флап сети на одном из двух
// запросов убирал с экрана уже приехавшие стрелки, а ответ, снятый ДО удаления
// связи, возвращал удалённую стрелку — и она залипала до следующего события.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const dependencies = vi.fn()
vi.mock('@/api', () => ({
  tasks: { update: vi.fn(), addRelation: vi.fn(), removeRelation: vi.fn() },
  boards: { dependencies: (...a) => dependencies(...a) },
}))

import BoardGanttView from '@/components/BoardGanttView.vue'

const stubs = {
  NIcon: { template: '<i />' },
  NTooltip: { template: '<div><slot name="trigger" /><slot /></div>' },
  NDropdown: { template: '<div><slot /></div>' },
  NPopover: { template: '<div><slot name="trigger" /><slot /></div>' },
}

const task = (id, number) => ({
  id,
  number,
  title: 'Задача ' + number,
  priority: 2,
  start_date: '2026-01-05T00:00:00Z',
  due_date: '2026-01-09T00:00:00Z',
})

// Ребро t1 → t2 в том виде, в каком его отдаёт бэкенд.
const EDGE = [{ id: 'r1', kind: 'blocks', task_id: 't1', related_task_id: 't2' }]

// Запросы отдаём вручную, по одному: только так видно, чей ответ лёг последним.
let pending = []
function nextRequest() {
  return pending[pending.length - 1]
}
function defer() {
  let settle
  const p = new Promise((resolve, reject) => {
    settle = { resolve, reject }
  })
  return { p, ...settle }
}

// Микрозадач между вызовом и отрисовкой несколько (await в loadDeps + вотчеры).
async function flush() {
  for (let i = 0; i < 4; i++) await nextTick()
}

function arrows(w) {
  return w.findAll('path.g-line').length
}

function mountGantt(tasks) {
  return mount(BoardGanttView, {
    props: {
      boardId: 'b1',
      tasks,
      statusColumns: [{ id: 'c1', name: 'К работе' }],
    },
    global: { stubs },
  })
}

describe('Gantt dependency edges vs a stale answer', () => {
  beforeEach(() => {
    // The v2 Gantt reads the viewport through useResponsive; jsdom has no
    // matchMedia at all. Desktop width — these cases are about the edges.
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    })
    setActivePinia(createPinia())
    pending = []
    dependencies.mockReset()
    dependencies.mockImplementation(() => {
      const d = defer()
      pending.push(d)
      return d.p
    })
  })

  it('снимок покинутого фильтра не перерисовывает стрелки', async () => {
    const w = mountGantt([task('t1', 1), task('t2', 2), task('t3', 3)])
    await flush()
    const stale = nextRequest()

    // Фильтр доски скрыл часть задач → состав сменился, вью переспрашивает рёбра.
    await w.setProps({ tasks: [task('t1', 1), task('t2', 2)] })
    await flush()
    const fresh = nextRequest()
    expect(fresh).not.toBe(stale)

    fresh.resolve({ data: [] })
    await flush()
    expect(arrows(w)).toBe(0)

    stale.resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(0)
    w.unmount()
  })

  it('провал устаревшего запроса не стирает уже приехавшие стрелки', async () => {
    const w = mountGantt([task('t1', 1), task('t2', 2), task('t3', 3)])
    await flush()
    const stale = nextRequest()

    await w.setProps({ tasks: [task('t1', 1), task('t2', 2)] })
    await flush()
    const fresh = nextRequest()

    fresh.resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(1)

    // Связь оборвалась на запросе, который уже никому не нужен. Своего ответа
    // вью не ждёт — стрелки на экране актуальны и гаснуть им не от чего.
    stale.reject(new Error('Нет соединения'))
    await flush()
    expect(arrows(w)).toBe(1)
    w.unmount()
  })

  it('удалённая стрелка не возвращается ответом, снятым до удаления', async () => {
    const w = mountGantt([task('t1', 1), task('t2', 2)])
    await flush()
    nextRequest().resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(1)

    // Событие о связи → перезапрос (снимок ещё со стрелкой, отвечает медленно).
    await w.setProps({ depsToken: 1 })
    await flush()
    const beforeDelete = nextRequest()

    // Связь удалили, пришло второе событие → перезапрос, и он отвечает первым.
    await w.setProps({ depsToken: 2 })
    await flush()
    const afterDelete = nextRequest()
    afterDelete.resolve({ data: [] })
    await flush()
    expect(arrows(w)).toBe(0)

    beforeDelete.resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(0)
    w.unmount()
  })

  it('свежий ответ по-прежнему рисует стрелки', async () => {
    const w = mountGantt([task('t1', 1), task('t2', 2)])
    await flush()
    nextRequest().resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(1)

    // …и следующий, уже после смены фильтра, тоже: гвард отбрасывает только то,
    // что заказывал кто-то другой.
    await w.setProps({ tasks: [task('t1', 1), task('t2', 2), task('t3', 3)] })
    await flush()
    nextRequest().resolve({
      data: [...EDGE, { id: 'r2', kind: 'blocked_by', task_id: 't3', related_task_id: 't1' }],
    })
    await flush()
    expect(arrows(w)).toBe(2)
    w.unmount()
  })

  it('провал свежего запроса убирает стрелки, которых никто не подтвердил', async () => {
    const w = mountGantt([task('t1', 1), task('t2', 2)])
    await flush()
    nextRequest().resolve({ data: EDGE })
    await flush()
    expect(arrows(w)).toBe(1)

    await w.setProps({ depsToken: 1 })
    await flush()
    nextRequest().reject(new Error('Нет соединения'))
    await flush()
    // Рисовать старый снимок дальше — врать про связи, которых мы не проверяли.
    expect(arrows(w)).toBe(0)
    w.unmount()
  })
})
