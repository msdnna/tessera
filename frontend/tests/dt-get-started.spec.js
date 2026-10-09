import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import { NButton, NInput, NCheckbox, NDropdown } from 'naive-ui'

import { GET_STARTED, CHAPTERS, ITEMS, OPTIONAL_CHAPTERS, stepOfItem } from '@/data/getStarted'
import { HINTS } from '@/data/hints'
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
  'src/components/rail/AppRail.vue',
  'src/components/SidebarNode.vue',
  'src/components/ProjectRow.vue',
  // The project form and the new-group row took over from the modal and the
  // create-then-rename group (#2969).
  'src/components/sidebar/ProjectCreatePopover.vue',
  'src/components/sidebar/NewGroupRow.vue',
  'src/components/task/TaskCardPills.vue',
  'src/components/TaskModal.vue',
  'src/components/KanbanBoard.vue',
  'src/components/BoardLayoutSwitch.vue',
  'src/components/SearchBar.vue',
  'src/components/SidebarFooter.vue',
  'src/components/NotificationsBell.vue',
  'src/components/task/TaskModalHeader.vue',
  // v2 chapters (#3000): the top bar, the board's root, the stage pieces.
  'src/components/Topbar.vue',
  'src/views/BoardView.vue',
]
const markup = MARKUP.map((f) => readFileSync(resolve(root, f), 'utf8')).join('\n')
const kanban = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

const isRawSelector = (key) => /[[\].#\s>]/.test(key)

// Every step of every chapter (#3000) — the checks below hold for all of them.
const ALL_STEPS = CHAPTERS.flatMap((c) => c.steps)

function anchorKeys() {
  const keys = []
  for (const s of ALL_STEPS) {
    // `cut` is in here too: it selects real elements (the modal's «Создать», the
    // target group's row), so a rename strands it exactly like an anchor —
    // silently, since a cut that matches nothing just leaves the mask solid.
    keys.push(s.anchor, ...(s.extra || []), ...(s.cut || []), s.advanceOn?.count, s.advanceOn?.set)
    keys.push(s.advanceOn?.appear, s.advanceOn?.vanish, s.skipIf)
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
  it('has unique step ids across all chapters', () => {
    const ids = ALL_STEPS.map((s) => s.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('gives every step an anchor, and a title and body in every locale', () => {
    // The wording lives in the catalogue (#2799), keyed by step id: a missing
    // entry renders the raw key path at the user. Both locales are checked,
    // since `ru` alone would let an English run fall back to Russian silently.
    for (const s of ALL_STEPS) {
      expect(s.anchor, s.id).toBeTruthy()
      for (const [locale, bundle] of [
        ['ru', ru],
        ['en', en],
      ]) {
        const entry = bundle.tour.steps[s.id]
        expect(entry, `${s.id} has no ${locale} entry`).toBeTruthy()
        expect(entry.title, `${s.id}.title (${locale})`).toBeTruthy()
        expect(entry.body, `${s.id}.body (${locale})`).toBeTruthy()
        // An action step says what will count it (the plate under the text).
        if (s.mode === 'action') expect(entry.wait, `${s.id}.wait (${locale})`).toBeTruthy()
      }
    }
  })

  it('names every chapter and every checklist result in every locale', () => {
    for (const bundle of [ru, en]) {
      for (const c of CHAPTERS) expect(bundle.tour.chapters[c.id]?.title, c.id).toBeTruthy()
      for (const item of Object.values(ITEMS).flat()) {
        const [ch, key] = item.split('.')
        expect(bundle.tour.items[ch]?.[key], item).toBeTruthy()
      }
    }
  })

  it('has no catalogue entry left over from a removed step', () => {
    const ids = new Set(ALL_STEPS.map((s) => s.id))
    expect(Object.keys(ru.tour.steps).filter((id) => !ids.has(id))).toEqual([])
    expect(Object.keys(en.tour.steps).filter((id) => !ids.has(id))).toEqual([])
  })

  it('gives every action step a way to advance, and info steps none', () => {
    for (const s of ALL_STEPS) {
      if (s.mode === 'action') {
        const a = s.advanceOn || {}
        expect(
          a.click || a.count || a.set || a.when || a.moved || a.appear || a.vanish,
          `${s.id} can never advance`,
        ).toBeTruthy()
      } else {
        // An info step advances on «Понятно»; an advanceOn there would be dead
        // config, since the store only consults it for action steps.
        expect(s.advanceOn, s.id).toBeUndefined()
      }
    }
  })

  it('ends every chapter on a closing info step', () => {
    // The last step has to be an info one: «Готово» on it calls finish() and
    // writes the chapter's ack. An action step there would leave the chapter
    // with no way to end but «Прервать».
    for (const c of CHAPTERS) {
      const last = c.steps.at(-1)
      expect(last.mode, `${c.id}: ${last.id}`).toBe('info')
      expect(last.final, `${c.id}: ${last.id}`).toBe(true)
    }
  })

  it('walks «Основы» in the order of the mockup — ten steps', () => {
    expect(GET_STARTED.map((s) => s.id)).toEqual([
      'workspaces',
      'tree-add',
      'project-create',
      'board-add',
      'task-create',
      'card-fields',
      'dnd-card',
      'card-open',
      'tm-close',
      'done',
    ])
  })

  it('waits for the entity, not the click, wherever one is created', () => {
    // The author's call (#2753): advancement by the project/board/task actually
    // appearing — a cancelled form must not let the guide run ahead.
    const byId = Object.fromEntries(ALL_STEPS.map((s) => [s.id, s]))
    for (const id of ['project-create', 'task-create', 'inbox-capture', 'group-add']) {
      expect(byId[id].advanceOn.count, id).toBeTruthy()
      expect(byId[id].advanceOn.click, id).toBeUndefined()
    }
    // The board: not when it exists, but when it is open — the next step is on it.
    expect(byId['board-add'].advanceOn.appear).toContain('{board}')
  })

  it('maps every checklist result to the step that counts it', () => {
    for (const item of Object.values(ITEMS).flat()) {
      const at = stepOfItem(item)
      expect(at, `${item} has no step`).toBeTruthy()
      expect(item.startsWith(at.chapter + '.'), item).toBe(true)
    }
    for (const c of OPTIONAL_CHAPTERS) expect(ITEMS[c.id]?.length, c.id).toBeGreaterThan(0)
  })

  it('names, for every v2 hint, its wording in every locale', () => {
    for (const bundle of [ru, en]) {
      for (const h of HINTS) {
        const e = bundle.hints.items[h.key]
        expect(e?.name && e?.title && e?.body, h.key).toBeTruthy()
        if (h.kind === 'ctx') expect(e.meta && e.wait, h.key).toBeTruthy()
      }
    }
  })

  it('has every data-testid a step or a hint names in the source', () => {
    // Raw selectors over data-testid are not in the MARKUP list (they live all
    // over the app); a renamed testid would strand the step silently.
    const ids = new Set()
    const sels = [...anchorKeys(), ...HINTS.flatMap((h) => [h.anchor, h.mobile])]
    for (const k of sels.filter(Boolean)) {
      for (const [, id] of k.matchAll(/data-testid="([^"]+)"/g)) ids.add(id)
    }
    const src = [
      'src/components/Topbar.vue',
      'src/components/QuickActionsFab.vue',
      'src/components/inbox/InboxPanel.vue',
      'src/components/inbox/InboxItem.vue',
      'src/components/inbox/CapturePop.vue',
      'src/components/milestone/StageSelect.vue',
      'src/components/milestone/StageStrip.vue',
      'src/components/palette/CommandPalette.vue',
      'src/components/ShortcutsModal.vue',
      'src/components/KanbanBoard.vue',
      'src/components/TaskCard.vue',
      'src/components/boardViews/ViewChip.vue',
      'src/components/kit/UndoToast.vue',
      'src/components/mobile/MobileTabBar.vue',
    ]
      .map((f) => readFileSync(resolve(root, f), 'utf8'))
      .join('\n')
    for (const id of ids) {
      // The phone's tabs are rendered from a list: `mobile-tab-${tab.key}`.
      const tab = id.match(/^mobile-tab-(\w+)$/)
      if (tab) {
        expect(src).toContain('`mobile-tab-${tab.key}`')
        expect(src).toContain(`key: '${tab[1]}'`)
        continue
      }
      // A kit panel takes it as a prop (`testid="inbox-panel"`).
      expect(src, `data-testid="${id}"`).toMatch(new RegExp(`(data-)?testid="${id}"`))
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
    for (const s of ALL_STEPS) {
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
    for (const s of ALL_STEPS) {
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
