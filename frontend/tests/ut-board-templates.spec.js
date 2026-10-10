import { describe, it, expect } from 'vitest'
import { BOARD_TEMPLATES, boardTemplate, firstBoard } from '@/utils/boardTemplates'
import { columnStatusName } from '@/utils/columnStatus'
import ruBoard from '@/locales/ru/board.json'
import enBoard from '@/locales/en/board.json'
import ruProject from '@/locales/ru/project.json'
import enProject from '@/locales/en/project.json'

// Board templates for an empty project (#3047). The presets live on the server;
// the client must caption every column key they seed, in every language.

describe('board templates', () => {
  it('has a caption for every seeded column key, ru and en', () => {
    for (const tpl of BOARD_TEMPLATES) {
      for (const key of tpl.columns) {
        expect(ruBoard.column.defaultName[key], `ru ${key}`).toBeTruthy()
        expect(enBoard.column.defaultName[key], `en ${key}`).toBeTruthy()
      }
      expect(ruProject.boardTemplate[tpl.key].name).toBeTruthy()
      expect(enProject.boardTemplate[tpl.key].name).toBeTruthy()
    }
  })

  it('matches the mockup: sprint kanban, bug tracker, roadmap', () => {
    expect(BOARD_TEMPLATES.map((x) => [x.key, x.glyph])).toEqual([
      ['sprint', 'layout-kanban'],
      ['bugs', 'layout-list'],
      ['roadmap', 'layout-timeline'],
    ])
    expect(boardTemplate('bugs').columns.at(-1)).toBe('closed')
    expect(boardTemplate('nope')).toBeNull()
  })

  it('opens the first live board of a project', () => {
    expect(firstBoard([])).toBeNull()
    expect(firstBoard(null)).toBeNull()
    expect(firstBoard([{ id: 'a', deleted_at: '2026-01-01' }, { id: 'b' }]).id).toBe('b')
  })

  it('draws «Проверка» of the bug tracker as a review column', () => {
    expect(columnStatusName({ nameKey: 'verify' })).toBe('status-review')
    expect(columnStatusName({ nameKey: 'confirmed' })).toBe('status-progress')
  })
})
