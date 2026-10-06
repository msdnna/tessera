import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// A board's toolbar state is written down twice, by hand, in KanbanBoard.vue:
//   • snapshotToolbar/loadToolbar — the per-board, per-layout localStorage slot;
//   • currentViewConfig/applyViewConfig — the blob a *saved view* carries to the
//     server and restores from.
// Both claim to hold the whole toolbar, and neither is derived from the other, so
// a knob added to one silently stays out of the other. That is not a cosmetic
// drift: a key missing from the view blob means loading a saved view leaves that
// knob at whatever the board happens to be showing — the view does not restore
// what was saved, and the state it does keep leaks into every other view.
//
// Source-level like dt-get-started.spec.js (see vitest.config.js: cwd is
// frontend/): the two shapes live inside the SFC's setup block, where no unit
// test can call them, and it is the *lists* that have to agree.
const root = process.cwd()
const kanban = readFileSync(resolve(root, 'src/components/KanbanBoard.vue'), 'utf8')

// `layout` is the one key a view carries that the localStorage slot cannot: that
// slot is keyed *by* layout, so the value is in the key rather than the blob.
const VIEW_ONLY = ['layout']

function fnBody(src, name) {
  const at = src.indexOf(`function ${name}(`)
  expect(at, `${name} not found in KanbanBoard.vue`).toBeGreaterThan(-1)
  const open = src.indexOf('{', src.indexOf(')', at))
  let depth = 0
  for (let i = open; i < src.length; i++) {
    if (src[i] === '{') depth++
    else if (src[i] === '}' && --depth === 0) return src.slice(open + 1, i)
  }
  throw new Error(`${name}: unbalanced braces`)
}

// Top-level keys of the object literal a `return { … }` hands back. Nested
// literals (`fieldVis: { ...fieldVis }`) raise the depth and are skipped.
function returnedKeys(body) {
  const open = body.indexOf('{', body.indexOf('return'))
  const keys = []
  let depth = 0
  for (let i = open; i < body.length; i++) {
    const ch = body[i]
    if (ch === '{') {
      depth++
      continue
    }
    if (ch === '}') {
      if (--depth === 0) break
      continue
    }
    if (depth !== 1) continue
    const m = /^([A-Za-z_$][\w$]*)\s*:/.exec(body.slice(i))
    if (m && !/[\w$.]/.test(body[i - 1] || '')) {
      keys.push(m[1])
      i += m[0].length - 1
    }
  }
  return keys
}

describe('board toolbar state: localStorage slot vs saved view', () => {
  const snapshot = returnedKeys(fnBody(kanban, 'snapshotToolbar'))
  const view = returnedKeys(fnBody(kanban, 'currentViewConfig'))

  it('reads both shapes out of the component', () => {
    expect(snapshot.length).toBeGreaterThan(5)
    expect(view.length).toBeGreaterThan(5)
    expect(snapshot).toContain('filters')
    expect(view).toContain('filters')
  })

  it('a saved view carries every knob the localStorage slot keeps', () => {
    expect([...snapshot].sort()).toEqual([...view].filter((k) => !VIEW_ONLY.includes(k)).sort())
  })

  it('applying a view reads back every key it saved', () => {
    const reader = fnBody(kanban, 'applyViewConfig') + fnBody(kanban, 'loadCustomize')
    const unread = view.filter((k) => !reader.includes(`c.${k}`) && !reader.includes(`s.${k}`))
    expect(unread).toEqual([])
  })

  it('the per-layout defaults name every key too, so an older blob is complete', () => {
    const defaults = returnedKeys(fnBody(kanban, 'defaultToolbar'))
    expect([...defaults].sort()).toEqual([...snapshot].sort())
  })
})
