import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { NButton, NInput, NCheckbox, NDropdown } from 'naive-ui'

import { GET_STARTED } from '@/data/getStarted'

// The scenario is declarative data walked by stores/tour.js, so nothing in the
// app breaks loudly when a step is malformed — it just quietly does nothing, or
// hangs on an arrow to an element that was renamed away. Hence these checks.
//
// See vitest.config.js: cwd is frontend/ (cx-doc-editor.spec.js explains why
// __dirname/import.meta.url are unusable here).
const root = process.cwd()

// Where each anchor key is expected to live. Keys not listed here are raw CSS
// selectors, checked separately.
const MARKUP = [
  'src/components/Sidebar.vue',
  'src/components/ProjectRow.vue',
  'src/components/ProjectCreateModal.vue',
]
const markup = MARKUP.map((f) => readFileSync(resolve(root, f), 'utf8')).join('\n')
const kanban = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

const isRawSelector = (key) => /[[\].#\s>]/.test(key)

function anchorKeys() {
  const keys = []
  for (const s of GET_STARTED) keys.push(s.anchor, ...(s.extra || []), s.advanceOn?.count)
  return [...new Set(keys.filter(Boolean))]
}

describe('Get Started scenario', () => {
  it('has unique step ids', () => {
    const ids = GET_STARTED.map((s) => s.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('gives every step an anchor, a title and a body', () => {
    for (const s of GET_STARTED) {
      expect(s.anchor, s.id).toBeTruthy()
      expect(s.title, s.id).toBeTruthy()
      expect(s.body, s.id).toBeTruthy()
    }
  })

  it('gives every action step a way to advance, and info steps none', () => {
    for (const s of GET_STARTED) {
      if (s.mode === 'action') {
        const a = s.advanceOn || {}
        expect(a.click || a.count || a.when, `${s.id} can never advance`).toBeTruthy()
      } else {
        // An info step advances on «Понятно»; an advanceOn there would be dead
        // config, since the store only consults it for action steps.
        expect(s.advanceOn, s.id).toBeUndefined()
      }
    }
  })

  it('starts on the workspace switcher and ends on the first task', () => {
    expect(GET_STARTED[0].id).toBe('workspaces')
    expect(GET_STARTED[0].mode).toBe('info')
    expect(GET_STARTED.at(-1).id).toBe('task-create')
  })

  it('waits for the entity, not the click, wherever one is created', () => {
    // Points 3–5 of the scenario: the author asked for advancement by the
    // project/board/task actually appearing — a cancelled modal must not let
    // the guide run ahead.
    const byId = Object.fromEntries(GET_STARTED.map((s) => [s.id, s]))
    for (const id of ['project-create', 'board-create', 'task-create']) {
      expect(byId[id].advanceOn.count, id).toBeTruthy()
      expect(byId[id].advanceOn.click, id).toBeUndefined()
    }
  })

  it('names only anchors that exist in the markup', () => {
    for (const key of anchorKeys()) {
      if (isRawSelector(key) || key.startsWith('menu-')) continue
      expect(markup, `data-tour="${key}" is not in the markup`).toContain(`"${key}"`)
    }
  })

  it('anchors the dropdown item through node-props', () => {
    // `menu-project` is never written out literally: naive builds it per option
    // from the option key, so the literal check above would miss a rename on
    // either side.
    expect(markup).toContain("'data-tour': `menu-${o.key}`")
    expect(markup).toMatch(/label: 'Проект', key: 'project'/)
  })

  it('survives naive-ui attribute fallthrough', async () => {
    // Half the anchors sit on naive components rather than plain elements. The
    // guide resolves them with document.querySelector, so an implicit attrs
    // fallthrough that lands somewhere unqueryable (or nowhere) would break the
    // step silently.
    for (const [name, Comp] of [
      ['button', NButton],
      ['input', NInput],
      ['checkbox', NCheckbox],
    ]) {
      const w = mount(Comp, { attrs: { 'data-tour': `probe-${name}` } })
      expect(w.find(`[data-tour="probe-${name}"]`).exists(), name).toBe(true)
      w.unmount()
    }
  })

  it('tags dropdown options through node-props', async () => {
    // The «Проект» step anchors on a menu item, which only exists while the
    // dropdown is open and is rendered by naive from an options array.
    const w = mount(NDropdown, {
      props: {
        show: true,
        options: [{ label: 'Проект', key: 'project' }],
        nodeProps: (o) => ({ 'data-tour': `menu-${o.key}` }),
      },
      slots: { default: () => h('button', 'Добавить') },
    })
    await new Promise((r) => setTimeout(r, 0))
    expect(document.querySelector('[data-tour="menu-project"]')).not.toBe(null)
    w.unmount()
  })

  it('keeps its raw kanban selectors in sync with KanbanBoard', () => {
    // These two ride on attributes the board already had; a rename there would
    // silently strand the last step of the guide.
    expect(kanban).toContain(':data-column-name="dcol.name"')
    expect(kanban).toContain('data-testid="add-task-button"')
    expect(readFileSync(resolve(root, 'src/components/TaskCard.vue'), 'utf8')).toContain(
      'data-testid="task-card"',
    )
  })
})
