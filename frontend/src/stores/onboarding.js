import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useWhatsNewStore } from '@/stores/whatsNew'
import { useTourStore, TOUR_DONE, TOUR_CHAPTER_PREFIX, TOUR_PREFIX } from '@/stores/tour'
import { CHAPTERS, ITEMS, OPTIONAL_CHAPTERS } from '@/data/getStarted'

// The «Начало работы» checklist (#3000, #3102): which results of which chapter
// the user has, with the tour or without it.
//
// A result is counted by the same event as its tour step — the user created a
// project, moved a card, opened the inbox — whatever screen, key or menu they
// did it from. The events come from two places (AppLayout wires both):
//   - the workspace socket: the server tags the user's own actions with their
//     id (`actor`, #3102), so a project created from ⌘K counts like one
//     created in the tree;
//   - the client itself, for what never reaches the server: a window opened,
//     the palette, the cheat sheet, a key pressed on a card.
//
// Kept in the acks like everything else of the tour:
//   getstarted:item:<id>        done while a tour chapter was running
//   getstarted:item:<id>:self   done on the user's own — «сделано без тура»
//   getstarted:checklist:hidden «Скрыть насовсем» / the switch in Settings
// A v1 account that walked or skipped the old guide (getstarted:done) has its
// «Основы» counted as passed — those are the same six things.

export const ITEM_PREFIX = TOUR_PREFIX + 'item:'
export const CHECKLIST_HIDDEN = TOUR_PREFIX + 'checklist:hidden'
const SELF = ':self'

// Which results an event stands for.
const EVENT_ITEMS = {
  'project.created': ['basics.project'],
  'board.created': ['basics.board'],
  'task.created': ['basics.task'],
  'task.fields': ['basics.fields'],
  'task.moved': ['basics.move'],
  'task.opened': ['basics.open'],
  'task.due': ['task.due', 'basics.fields'],
  'task.assigned': ['task.assignees', 'basics.fields'],
  'task.priority': ['task.priority', 'basics.fields'],
  'task.tagged': ['task.tags', 'basics.fields'],
  'task.described': ['task.description'],
  'inbox.opened': ['inbox.open'],
  'inbox.captured': ['inbox.capture'],
  'inbox.handled': ['inbox.handle'],
  'stage.selected': ['stages.select'],
  'stage.strip': ['stages.strip'],
  'stage.page': ['stages.page'],
  'palette.opened': ['keys.palette'],
  'sheet.opened': ['keys.sheet'],
  'card.keys': ['keys.card'],
  'board.grouped': ['tags.group'],
  'tags.opened': ['tags.manage'],
  'group.created': ['projects.group'],
  'project.moved': ['projects.move'],
  'project.window': ['projects.window'],
  'gitlab.opened': ['gitlab.open'],
  'gitlab.synced': ['gitlab.sync'],
}

// Socket event type → onboarding event, for events whose actor is the user.
const SOCKET_EVENTS = {
  'project.created': 'project.created',
  'board.created': 'board.created',
  'task.created': 'task.created',
  'task.moved': 'task.moved',
  'task.tagged': 'task.tagged',
  'task.assigned': 'task.assigned',
  'group.created': 'group.created',
  'project.moved': 'project.moved',
}

export function itemsOfEvent(name) {
  return EVENT_ITEMS[name] || []
}

export const useOnboardingStore = defineStore('onboarding', () => {
  const acks = useWhatsNewStore()
  const tour = useTourStore()
  // Whether GitLab is bound anywhere in the space — the GitLab chapter waits
  // for it (AppLayout asks the server once per workspace).
  const gitlabBound = ref(false)

  const legacyDone = computed(() => acks.has(TOUR_DONE))

  function isDone(item) {
    if (acks.has(ITEM_PREFIX + item) || acks.has(ITEM_PREFIX + item + SELF)) return true
    return legacyDone.value && item.startsWith('basics.')
  }
  // Done only on the user's own — the row says «сделано без тура».
  function isSelf(item) {
    return acks.has(ITEM_PREFIX + item + SELF) && !acks.has(ITEM_PREFIX + item)
  }

  // { done, total, complete } of a chapter. A chapter walked to the end is
  // complete even if a result was skipped on the way (an info step has none).
  function progress(chapterId) {
    const items = ITEMS[chapterId] || []
    const done = items.filter(isDone).length
    const walked =
      acks.has(TOUR_CHAPTER_PREFIX + chapterId) || (chapterId === 'basics' && legacyDone.value)
    return {
      done,
      total: items.length,
      complete: walked || (items.length > 0 && done === items.length),
    }
  }

  const basics = computed(() => progress('basics'))
  const optional = computed(() =>
    OPTIONAL_CHAPTERS.map((c) => ({
      ...c,
      ...progress(c.id),
      locked: c.locked === 'gitlab' && !gitlabBound.value,
    })),
  )
  // Optional chapters passed / available (a locked one isn't counted as missing).
  const chaptersDone = computed(() => optional.value.filter((c) => c.complete).length)
  const chaptersOpen = computed(() => optional.value.filter((c) => !c.locked).length)
  const allDone = computed(
    () => basics.value.complete && optional.value.every((c) => c.complete || c.locked),
  )
  const hidden = computed(() => acks.has(CHECKLIST_HIDDEN))

  // The next thing to do: the first result of «Основы» not done, or null.
  const nextBasic = computed(() => (ITEMS.basics || []).find((i) => !isDone(i)) || null)

  async function mark(item, { tour: inTour = tour.active } = {}) {
    if (!item || isDone(item)) {
      // Done on its own earlier, now walked in the tour: keep the tour's mark
      // too, so the row stops saying «без тура».
      if (item && inTour && isSelf(item)) await acks.ack(ITEM_PREFIX + item)
      return
    }
    await acks.ack(ITEM_PREFIX + item + (inTour ? '' : SELF))
  }

  // An onboarding event from anywhere in the app.
  function note(name) {
    for (const item of itemsOfEvent(name)) mark(item)
  }

  // A socket event: counted only when the user did it themselves.
  function onSocket(ev, selfId) {
    if (!ev || !selfId || ev.actor !== selfId) return
    const name = SOCKET_EVENTS[ev.type]
    if (name) note(name)
  }

  async function hide() {
    await acks.ack(CHECKLIST_HIDDEN)
  }
  async function unhide() {
    await acks.forget(CHECKLIST_HIDDEN)
  }

  // «Начать с начала»: every mark of the tour and the checklist is cleared and
  // «Основы» start over. The card's own visibility is the user's choice and
  // survives — only the progress goes.
  async function restart({ mobile = false } = {}) {
    const keepHidden = hidden.value
    await acks.forgetPrefix(TOUR_PREFIX)
    if (keepHidden) await acks.ack(CHECKLIST_HIDDEN)
    tour.startGuide({ mobile })
  }

  return {
    gitlabBound,
    isDone,
    isSelf,
    progress,
    basics,
    optional,
    chaptersDone,
    chaptersOpen,
    allDone,
    hidden,
    nextBasic,
    mark,
    note,
    onSocket,
    hide,
    unhide,
    restart,
  }
})

export { CHAPTERS }
