import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { NButton, NInput, NCheckbox, NDropdown } from 'naive-ui'

import { GET_STARTED } from '@/data/getStarted'
import ru from '@/locales/ru'
import en from '@/locales/en'

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
    // `cut` is in here too: it selects real elements (the modal's «Создать», the
    // target group's row), so a rename strands it exactly like an anchor —
    // silently, since a cut that matches nothing just leaves the mask solid.
    keys.push(s.anchor, ...(s.extra || []), ...(s.cut || []), s.advanceOn?.count, s.advanceOn?.set)
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

  it('gives every step an anchor, and a title and body in every locale', () => {
    // The wording moved into the catalogue (#2799), keyed by step id, so the
    // failure this guards against changed shape: a step is no longer missing a
    // field — it renders the raw key path at the user. Both locales are checked,
    // since `ru` alone would let an English run fall back to Russian silently.
    for (const s of GET_STARTED) {
      expect(s.anchor, s.id).toBeTruthy()
      for (const [locale, bundle] of [
        ['ru', ru],
        ['en', en],
      ]) {
        const entry = bundle.tour.steps[s.id]
        expect(entry, `${s.id} has no ${locale} entry`).toBeTruthy()
        expect(entry.title, `${s.id}.title (${locale})`).toBeTruthy()
        expect(entry.body, `${s.id}.body (${locale})`).toBeTruthy()
      }
    }
  })

  it('has no catalogue entry left over from a removed step', () => {
    const ids = new Set(GET_STARTED.map((s) => s.id))
    expect(Object.keys(ru.tour.steps).filter((id) => !ids.has(id))).toEqual([])
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

  it('has someone to fill every {token} its steps are scoped with', () => {
    // Steps scoped to an entity the user creates mid-guide carry a token the
    // store expands from the context a component reported. Miss either half and
    // nothing fails loudly: the selector keeps its literal `{token}`, matches
    // nothing, and the step falls back to a timeout — or, when it was written
    // unscoped, points confidently at the first row in the tree, which is how
    // the mask ended up on someone else's group (#2778 rework).
    const store = readFileSync(resolve(root, 'src/stores/tour.js'), 'utf8')
    const tokens = new Set()
    for (const key of anchorKeys()) {
      for (const [, t] of key.matchAll(/\{(\w+)\}/g)) tokens.add(t)
    }
    expect(tokens).toContain('group')
    for (const t of tokens) {
      expect(store, `resolve() leaves {${t}} unexpanded`).toContain(`{${t}}`)
      expect(markup, `nothing reports ${t}Id to the guide`).toMatch(
        new RegExp(`noteCreated\\(\\{\\s*${t}Id`),
      )
    }
  })

  it('anchors the dropdown item through node-props', () => {
    // `menu-project` is never written out literally: naive builds it per option
    // from the option key, so the literal check above would miss a rename on
    // either side.
    expect(markup).toContain("'data-tour': `menu-${o.key}`")
    // The label moved into the catalogue (#2799); the anchor rides on the key,
    // which is what has to stay put — a renamed key silently unhooks the step.
    expect(markup).toMatch(/label: t\('shell\.tree\.addProject'\), key: 'project'/)
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
    //
    // The guide addresses columns by their seeded Russian names, so the attribute
    // has to carry the column's server-side name and not its caption — those parted
    // ways when seeded columns started being translated (#2800).
    expect(kanban).toContain(':data-column-name="dcol.rawName"')
    expect(kanban).toContain('data-testid="add-task-button"')
    expect(readFileSync(resolve(root, 'src/components/TaskCard.vue'), 'utf8')).toContain(
      'data-testid="task-card"',
    )
  })
})
