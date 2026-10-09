// The board-name field in «Настроить вид» edits a draft, not the live model (#2932).
//
// It used to bind `v-model:value="boardName"` — the two-way model KanbanBoard keeps
// in sync with the loaded board — and emit `update-board` unconditionally on BOTH
// `@blur` and `@keyup.enter`. Every one of those emits reaches boards.update(), which
// is a full replace of the board's name+icon+colour+mode plus a workspace-wide
// `board.updated` broadcast, so the looseness was not free:
//
//   * Enter and then a click away sent the same PUT twice.
//   * Focusing the field and leaving it untouched sent a PUT that changed nothing.
//   * Esc closes the drawer without blurring the input (a browser fires no blur for a
//     focused node that is merely removed from the DOM), so the abandoned text was
//     never saved — and, because it had been written straight into the model, the
//     drawer went on showing it as the board's name, ready for the next stray blur of
//     that field to commit the rename the reader had cancelled.
//
// The sidebar's inline board rename (ProjectRow.commitBoardRename) has always done
// this properly: a buffer seeded when the edit opens, and `if (!n || n === b.name)
// return` on commit. These pin the same contract for the drawer.
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import BoardCustomizePanel from '@/components/BoardCustomizePanel.vue'

vi.mock('naive-ui', async () => {
  const actual = await vi.importActual('naive-ui')
  return {
    ...actual,
    useMessage: () => ({ error: vi.fn(), success: vi.fn(), info: vi.fn(), warning: vi.fn() }),
  }
})

// n-drawer teleports to <body>; the stub renders it in place so the field is
// reachable from the wrapper. jsdom ships no matchMedia, which useResponsive needs.
const STUBS = { teleport: true, TesseraIcon: true }
beforeEach(() => {
  // The panel prints its due-date preview through useFormat (the theme store).
  setActivePinia(createPinia())
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  })
})

const NAME = 'Доска релиза'

function mountPanel() {
  return mount(BoardCustomizePanel, {
    props: {
      show: true,
      readonly: false,
      defaultOpen: ['board'],
      boardName: NAME,
      boardIcon: '',
      boardColor: '#7c5cff',
      boardIconMode: 'badge',
      fieldVis: {},
      facetChips: [],
      addOptions: [],
    },
    global: { stubs: STUBS },
  })
}

// The `<input>` inside the name field. Selected by `.n-input` rather than by the
// testid the field also carries: the testid arrived with this change, and a spec
// that cannot find the old field at all would "fail" on it for the wrong reason.
// The radio buttons below carry hidden inputs of their own, but none of them sits
// inside an `.n-input` — this panel has exactly one.
const nameInput = (w) => w.find('.n-input input')

describe('BoardCustomizePanel board-name field', () => {
  it('shows the board’s name', () => {
    const w = mountPanel()
    expect(nameInput(w).element.value).toBe(NAME)
    w.unmount()
  })

  it('stays silent when the field is left untouched', async () => {
    const w = mountPanel()
    await nameInput(w).trigger('blur')
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })

  it('stays silent when the text is edited back to the same name', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Доска')
    await nameInput(w).setValue(NAME)
    await nameInput(w).trigger('blur')
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })

  it('emits once for a real rename', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Релиз 2.0')
    await nameInput(w).trigger('keyup.enter')
    expect(w.emitted('update-board')).toHaveLength(1)
    expect(w.emitted('update-board')[0][0]).toEqual({ name: 'Релиз 2.0' })
    w.unmount()
  })

  // The gesture that produced two identical PUTs: commit with Enter, then click away.
  it('does not emit again when the committed field blurs', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Релиз 2.0')
    await nameInput(w).trigger('keyup.enter')
    await nameInput(w).trigger('blur')
    expect(w.emitted('update-board')).toHaveLength(1)
    w.unmount()
  })

  // A commit is published through the model too, so KanbanBoard (and the avatar
  // initials right below the field) see the new name without waiting for the server.
  it('publishes the committed name through the model', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Релиз 2.0')
    await nameInput(w).trigger('keyup.enter')
    expect(w.emitted('update:boardName')).toHaveLength(1)
    expect(w.emitted('update:boardName')[0][0]).toBe('Релиз 2.0')
    w.unmount()
  })

  // Typing alone must not move the model: that is what left an abandoned edit
  // standing in for the board's name.
  it('keeps the model untouched while typing', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Полов')
    expect(w.emitted('update:boardName')).toBeFalsy()
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })

  // Esc / the ✕ / the mask all end with `show: false` and no blur.
  it('throws an uncommitted edit away when the drawer closes', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Полов')
    await w.setProps({ show: false })
    await w.setProps({ show: true })
    expect(nameInput(w).element.value).toBe(NAME)
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })

  // …and the field no longer holds anything that a later blur could commit.
  it('has nothing left to commit after a cancelled edit', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('Полов')
    await w.setProps({ show: false })
    await w.setProps({ show: true })
    await nameInput(w).trigger('blur')
    expect(w.emitted('update-board')).toBeFalsy()
    w.unmount()
  })

  // An emptied field is not a rename — KanbanBoard's updateBoard refuses a blank
  // name; the field now restores the current one instead of asking it to.
  it('restores the name instead of committing an empty field', async () => {
    const w = mountPanel()
    await nameInput(w).setValue('')
    await nameInput(w).trigger('blur')
    expect(w.emitted('update-board')).toBeFalsy()
    expect(nameInput(w).element.value).toBe(NAME)
    w.unmount()
  })

  // A rename from elsewhere (the sidebar's inline edit, or a colleague's, arriving
  // through KanbanBoard's `watch(board)`) has to land in the field.
  it('follows the model when the board is renamed elsewhere', async () => {
    const w = mountPanel()
    await w.setProps({ boardName: 'Переименована' })
    expect(nameInput(w).element.value).toBe('Переименована')
    w.unmount()
  })
})

// The drawer is the one place that edits the live board's branding; KanbanBoard is
// too heavy to mount here (it loads a board over the API on create), so its half of
// the contract is read off the source, as the neighbouring specs do.
const kanban = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('KanbanBoard wiring for the name field', () => {
  it('still resyncs the mirror from the loaded board', () => {
    expect(kanban).toContain('watch(board, (b) => (boardName.value = b?.name')
  })

  it('still restores the mirror when the write fails', () => {
    const fn = kanban.slice(kanban.indexOf('async function updateBoard('))
    expect(fn.slice(0, fn.indexOf('\n}'))).toContain('boardName.value = board.value.name')
  })
})
