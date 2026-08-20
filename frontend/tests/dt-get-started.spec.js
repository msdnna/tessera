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
  'src/components/SidebarNode.vue',
  'src/components/ProjectRow.vue',
  'src/components/ProjectCreateModal.vue',
  'src/components/task/TaskCardPills.vue',
  'src/components/TaskModal.vue',
  'src/components/KanbanBoard.vue',
  'src/components/BoardLayoutSwitch.vue',
  'src/components/BoardActions.vue',
  'src/components/SearchBar.vue',
  'src/components/SidebarFooter.vue',
  'src/components/WorkspaceTools.vue',
]
const markup = MARKUP.map((f) => readFileSync(resolve(root, f), 'utf8')).join('\n')
const kanban = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

const isRawSelector = (key) => /[[\].#\s>]/.test(key)

function anchorKeys() {
  const keys = []
  for (const s of GET_STARTED) {
    keys.push(s.anchor, ...(s.extra || []), s.advanceOn?.count, s.advanceOn?.set)
    keys.push(s.advanceOn?.moved?.el, s.advanceOn?.moved?.within)
    if (typeof s.advanceOn?.click === 'string') keys.push(s.advanceOn.click)
  }
  return [...new Set(keys.filter((k) => k && typeof k === 'string'))]
}

// Bare keys, plus the ones buried inside a composite selector — the card steps
// scope their anchor to the «К работе» column, so `card-priority` never appears
// on its own.
function tourKeys() {
  const out = new Set()
  for (const key of anchorKeys()) {
    if (!isRawSelector(key)) {
      out.add(key)
      continue
    }
    for (const m of key.matchAll(/data-tour="([^"]+)"/g)) out.add(m[1])
  }
  return [...out]
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
        expect(
          a.click || a.count || a.set || a.when || a.moved,
          `${s.id} can never advance`,
        ).toBeTruthy()
      } else {
        // An info step advances on «Понятно»; an advanceOn there would be dead
        // config, since the store only consults it for action steps.
        expect(s.advanceOn, s.id).toBeUndefined()
      }
    }
  })

  it('starts on the workspace switcher and ends on the closing step', () => {
    expect(GET_STARTED[0].id).toBe('workspaces')
    expect(GET_STARTED[0].mode).toBe('info')
    // The last step has to be an info one: «Понятно» on it is what calls
    // finish() and writes the getstarted:done ack. An action step there would
    // leave the guide with no way to end other than «Пропустить».
    expect(GET_STARTED.at(-1).id).toBe('done')
    expect(GET_STARTED.at(-1).mode).toBe('info')
  })

  it('walks the scenario in the order the task describes', () => {
    const ids = GET_STARTED.map((s) => s.id)
    const spine = [
      'workspaces',
      'project-create',
      'board-create',
      'task-create',
      'card-fields',
      'card-open',
      'tm-due',
      'tm-save',
      'board-tools',
      'nav-sections',
      'done',
    ]
    expect(spine.map((id) => ids.indexOf(id))).toEqual(
      [...spine.map((id) => ids.indexOf(id))].sort((a, b) => a - b),
    )
    expect(ids).toEqual(expect.arrayContaining(spine))
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
    for (const key of tourKeys()) {
      if (key.startsWith('menu-')) continue
      expect(markup, `data-tour="${key}" is not in the markup`).toContain(`"${key}"`)
    }
  })

  it('keeps its borrowed selectors (tabs, nav) in sync with the markup', () => {
    // These ride on attributes naive/the sidebar already render, so nothing in
    // the guide would fail loudly if a tab or a nav route were renamed.
    const modal = readFileSync(resolve(root, 'src/components/TaskModal.vue'), 'utf8')
    const sidebar = readFileSync(resolve(root, 'src/components/Sidebar.vue'), 'utf8')
    for (const key of anchorKeys()) {
      for (const [, name] of key.matchAll(/\[data-name="([^"]+)"\]/g)) {
        expect(modal, `tab "${name}"`).toContain(`<n-tab-pane name="${name}"`)
      }
      for (const [, nav] of key.matchAll(/\[data-nav="([^"]+)"\]/g)) {
        expect(sidebar, `nav "${nav}"`).toContain(`data-nav="${nav}"`)
      }
    }
  })

  it('marks a filled field for every set-based step', () => {
    // advanceOn.set matches on a data-tour-set marker the field renders only
    // while it holds a value. A step whose marker was never wired up would look
    // fine here and hang in the app.
    const modal = readFileSync(resolve(root, 'src/components/TaskModal.vue'), 'utf8')
    for (const s of GET_STARTED) {
      const sel = s.advanceOn?.set
      if (!sel) continue
      expect(sel, s.id).toContain('[data-tour-set]')
      const key = sel.match(/data-tour="([^"]+)"/)?.[1]
      expect(key, s.id).toBe(s.anchor)
      expect(modal, `${s.id}: no :data-tour-set beside data-tour="${key}"`).toMatch(
        new RegExp(`data-tour="${key}"[\\s\\S]{0,120}:data-tour-set=`),
      )
    }
  })

  it('wires every moved-step to a container that really carries an address', () => {
    // advanceOn.moved reads `by` off the nearest `within` container (#2778). Two
    // ways to get that wrong silently: name a container that doesn't carry the
    // attribute (closest() finds it, getAttribute returns null → the step never
    // ends), or point at an attribute nothing renders any more.
    for (const s of GET_STARTED) {
      const m = s.advanceOn?.moved
      if (!m) continue
      expect(m.el && m.within && m.by, `${s.id}: incomplete moved spec`).toBeTruthy()
      expect(m.within, `${s.id}: within does not select on ${m.by}`).toContain(m.by)
      expect(markup, `${s.id}: nothing renders ${m.by}`).toMatch(new RegExp(`:?${m.by}=`))
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
