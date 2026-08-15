import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// This is the regression test for what actually killed the first TipTap attempt
// in this project (ed63159, "dark-theme breakage"). The cause was not the
// library: RichEditor.vue used four CSS variables that do not exist
// (--t-card, --t-fill, --t-text, --t-text-3) and gave each a hardcoded *light*
// colour as the var() fallback. In the light theme it looked fine, so the
// mistake was invisible until someone switched to dark.
//
// ProseMirror renders plain DOM, so naive-ui's themeOverrides never reach the
// editing surface — CSS custom properties are the only channel the theme has.
// That makes this failure mode structural, not carelessness, and worth a test.

// This file reads sources off disk, so it needs the package root. __dirname does
// not exist (ESM) and import.meta.url is not a file: URL here — Vite serves test
// modules over its own scheme, so fileURLToPath throws. Vitest sets its root as
// the process cwd, and vitest.config.js keeps the default, so cwd is frontend/.
const root = process.cwd()

const DOC_FILES = [
  'src/components/documents/DocEditor.vue',
  'src/components/documents/DocToolbar.vue',
  'src/components/documents/DocComments.vue',
  'src/views/DocumentsView.vue',
]

function styleBlock(source) {
  const blocks = [...source.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1])
  return blocks.join('\n')
}

function definedTokens() {
  const css = readFileSync(resolve(root, 'src/styles/main.css'), 'utf8')
  const store = readFileSync(resolve(root, 'src/stores/theme.js'), 'utf8')
  return new Set([
    ...[...css.matchAll(/(--t-[a-z0-9-]+)\s*:/g)].map((m) => m[1]),
    ...[...store.matchAll(/'(--t-[a-z0-9-]+)'/g)].map((m) => m[1]),
  ])
}

describe.each(DOC_FILES)('%s theming', (file) => {
  const source = readFileSync(resolve(root, file), 'utf8')
  const css = styleBlock(source)

  it('only references theme tokens that exist', () => {
    const tokens = definedTokens()
    const used = [...css.matchAll(/var\((--t-[a-z0-9-]+)/g)].map((m) => m[1])
    expect(used.length).toBeGreaterThan(0)
    const unknown = [...new Set(used)].filter((t) => !tokens.has(t))
    expect(unknown).toEqual([])
  })

  // Stricter than checking the names, and deliberately so: it was the
  // hardcoded fallback that hid the typo. A colour written out in full cannot
  // follow the theme, so there is no correct use of one here.
  it('contains no literal colours', () => {
    const literals = [
      ...css.matchAll(/#[0-9a-fA-F]{3,8}\b/g),
      ...css.matchAll(/\b(?:rgba?|hsla?)\(/g),
    ].map((m) => m[0])
    expect(literals).toEqual([])
  })

  it('uses no var() fallback at all', () => {
    const withFallback = [...css.matchAll(/var\(--t-[a-z0-9-]+\s*,/g)].map((m) => m[0])
    expect(withFallback).toEqual([])
  })
})

describe('document toolbar skin', () => {
  const source = readFileSync(resolve(root, 'src/components/documents/DocToolbar.vue'), 'utf8')

  // The selects and tabs are naive-ui components, so they inherit
  // themeOverrides; writing them by hand is how a third-party-looking panel
  // creeps in.
  it('builds its controls from naive-ui components', () => {
    for (const tag of ['n-tabs', 'n-tab-pane', 'n-select']) {
      expect(source).toContain(`<${tag}`)
    }
  })

  // The accent is user-configurable at runtime, so the active state has to read
  // the token rather than a fixed purple.
  it('paints the active button with the accent token', () => {
    expect(source).toMatch(/\.doc-tbtn\.on\s*{[^}]*var\(--t-primary\)/)
  })
})
