// The activity toast credited a robot's action to a living person (#2932).
//
// `Event.Actor` is set for user-initiated actions and deliberately EMPTY for
// system/worker ones — the backend says so in so many words (internal/realtime/
// hub.go: "absent for system/worker actions (GitLab sync, recurrence)"). The board
// filled that hole with the task's `created_by`, which is a different fact
// entirely: `cloneRecurringTask` copies `CreatedBy: src.CreatedBy`, so every
// occurrence of a recurring task carries the author of the FIRST one.
//
// So the schedule worker advanced a task at 03:00 and the toast announced «Вы
// переместили задачу #N» over your own avatar — or named a colleague who was
// asleep. A toast is a claim about who did something; it has no business guessing.
//
// Checked here: the actor rule itself (utils/boardEvents toastActor), that
// KanbanBoard no longer reaches for `created_by`, and that an actor-less toast
// actually renders as «Кто-то» with no avatar — the fallback the fix relies on.
import { describe, it, expect, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ru from '@/locales/ru'
import { toastActor } from '@/utils/boardEvents'
import BoardActivityToasts from '@/components/BoardActivityToasts.vue'

const root = process.cwd()
const ME = '11111111-1111-1111-1111-111111111111'
const COLLEAGUE = '22222222-2222-2222-2222-222222222222'
const MEMBERS = { [ME]: { name: 'Я' }, [COLLEAGUE]: { name: 'Коллега' } }

describe('toastActor', () => {
  it('names nobody for a worker event, whoever created the task', () => {
    // The clone of a recurring task: created_by is the author of occurrence #1.
    const ev = { type: 'task.created', actor: '', data: { created_by: COLLEAGUE } }
    expect(toastActor(ev, { selfId: ME, members: MEMBERS })).toEqual({
      actorId: null,
      actorName: '',
      self: false,
    })
  })

  it('does not mistake the reader for the actor of an anonymous event', () => {
    // The nastiest shape of the bug: my own recurring task, advanced by the
    // scheduler while I was away, read as "you did this".
    const ev = { type: 'task.moved', actor: '', data: { created_by: ME } }
    expect(toastActor(ev, { selfId: ME, members: MEMBERS }).self).toBe(false)
  })

  it('credits the acting user when the event names one', () => {
    const ev = { type: 'task.moved', actor: COLLEAGUE, data: { created_by: ME } }
    expect(toastActor(ev, { selfId: ME, members: MEMBERS })).toEqual({
      actorId: COLLEAGUE,
      actorName: 'Коллега',
      self: false,
    })
  })

  it('marks the reader as self when they are the actor', () => {
    const ev = { type: 'task.created', actor: ME, data: { created_by: COLLEAGUE } }
    const got = toastActor(ev, { selfId: ME, members: MEMBERS })
    expect(got.self).toBe(true)
    expect(got.actorId).toBe(ME)
  })

  it('shows no face for an actor missing from the member map', () => {
    const gone = '33333333-3333-3333-3333-333333333333'
    expect(toastActor({ actor: gone }, { selfId: ME, members: MEMBERS })).toEqual({
      actorId: null,
      actorName: '',
      self: false,
    })
  })

  it('survives an empty call (no options, no event)', () => {
    expect(toastActor(undefined)).toEqual({ actorId: null, actorName: '', self: false })
  })
})

describe('KanbanBoard.pushActivity', () => {
  const board = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')
  const fn = board.slice(
    board.indexOf('function pushActivity('),
    board.indexOf('\n}', board.indexOf('function pushActivity(')),
  )

  it('resolves the actor through the shared rule', () => {
    expect(fn).toContain('toastActor(ev')
  })

  it('never falls back to the task author', () => {
    expect(fn).not.toContain('created_by')
  })
})

describe('BoardActivityToasts', () => {
  let wrapper = null
  // A mount left open in this suite keeps its scheduler around and the next
  // test's re-render never lands (see memory: leaked mounts).
  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
  })

  function mountStack() {
    const i18n = createI18n({ legacy: false, locale: 'ru', messages: { ru } })
    wrapper = mount(BoardActivityToasts, { global: { plugins: [i18n] } })
    return wrapper
  }

  it('says «Кто-то» and draws no avatar when there is no actor', async () => {
    const w = mountStack()
    w.vm.push({ id: 'a', number: 7, title: 'Полить цветы', verb: 'moved', ...toastActor({}) })
    await w.vm.$nextTick()
    expect(w.find('.t-who').text()).toBe(ru.board.activity.someone)
    expect(w.find('.ava').exists()).toBe(false)
  })

  it('names the person when the event carries one', async () => {
    const w = mountStack()
    w.vm.push({
      id: 'a',
      number: 7,
      title: 'Полить цветы',
      verb: 'moved',
      ...toastActor({ actor: COLLEAGUE }, { selfId: ME, members: MEMBERS }),
    })
    await w.vm.$nextTick()
    expect(w.find('.t-who').text()).toBe('Коллега')
  })
})
