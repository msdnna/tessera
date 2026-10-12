// The kind of a new board (#3017): the inline row in the sidebar switches
// Канбан / Список with Tab (or a click on its chip) and creates the board with
// that kind; a board opened for the first time on this device, with no default
// view, opens in it. ProjectRow and KanbanBoard are too heavy to mount here, so
// the wiring is read off their sources (as cx-archive-customize-board does).
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { isStub } from '@/utils/featureStubs'

const row = readFileSync(resolve(process.cwd(), 'src/components/ProjectRow.vue'), 'utf8')
const board = readFileSync(resolve(process.cwd(), 'src/components/KanbanBoard.vue'), 'utf8')

describe('new board kind', () => {
  it('is no longer a stub', () => {
    expect(isStub('board.newKind')).toBe(false)
    expect(row).not.toContain("stubTask('board.newKind')")
  })

  it('switches on Tab and keeps the field open', () => {
    const fn = row.slice(row.indexOf('async function commitNewBoard('))
    const head = fn.slice(0, fn.indexOf('const n = value.trim()'))
    expect(head).toContain("if (via === 'tab') {")
    expect(head).toContain('toggleNewBoardKind()')
  })

  it('sends the kind with the create', () => {
    expect(row).toMatch(
      /createBoard\(props\.project\.id, \{\s*name: n,\s*default_layout: newBoardKind\.value,/,
    )
  })

  it('opens a never-seen board in its kind unless a default view applied', () => {
    const fn = board.slice(board.indexOf('async function openBoard('))
    const body = fn.slice(0, fn.indexOf('\n}'))
    expect(body).toContain('const applied = bv.onOpen({ applyDefault: untouched })')
    expect(body).toContain('board.value?.default_layout')
    expect(body).toContain(
      'if (untouched && !applied && !seen && start && start !== layout.value) layout.value = start',
    )
  })
})
