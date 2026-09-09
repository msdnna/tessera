import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve, join } from 'node:path'
import process from 'node:process'

// Regression test for #2863 — thick, high-contrast scrollbars in Firefox.
//
// `.t-hoverscroll` deliberately resets `scrollbar-width`/`scrollbar-color` to
// `auto` so the -webkit- pseudo-elements own the look: in Chrome an explicit
// `scrollbar-color` switches on the NATIVE bar, whose built-in hover highlight
// can't be overridden (that was the "colour jitter" on kanban columns).
//
// Firefox implements none of those pseudo-elements, so for it that reset is not
// "hand control to the pseudo-elements" — it is "fall back to the system bar".
// The fix is a `@supports not selector(::-webkit-scrollbar-thumb)` block restating
// the auto-hiding model in standard properties. This test locks in both halves:
// the fallback exists, and no future reset ships without one.
//
// Sources are read off disk (same trick as cx-doc-editor.spec.js): __dirname
// doesn't exist in ESM and import.meta.url isn't a file: URL under Vite. Vitest
// runs with frontend/ as cwd.
const root = process.cwd()
const css = readFileSync(resolve(root, 'src/styles/main.css'), 'utf8')

// Pull out the body of a top-level at-rule by brace matching — a regex can't
// nest, and this block contains rules of its own.
function atRuleBody(source, header) {
  const start = source.indexOf(header)
  if (start === -1) return null
  const open = source.indexOf('{', start)
  if (open === -1) return null
  let depth = 0
  for (let i = open; i < source.length; i++) {
    if (source[i] === '{') depth++
    else if (source[i] === '}') {
      depth--
      if (depth === 0) return source.slice(open + 1, i)
    }
  }
  return null
}

// Deliberately the -thumb pseudo-element, not the bare ::-webkit-scrollbar:
// Firefox parses the latter as a web-compat alias and reports it as SUPPORTED
// (checked on Firefox 153), so a query on the bare selector is dead code in the
// one browser this block exists for. See the comment in main.css.
const FALLBACK_HEADER = '@supports not selector(::-webkit-scrollbar-thumb)'

describe('scrollbar styling', () => {
  it('gives Firefox a standard-property fallback for .t-hoverscroll', () => {
    const body = atRuleBody(css, FALLBACK_HEADER)
    expect(body, `missing ${FALLBACK_HEADER} block in main.css`).toBeTruthy()
    expect(body).toMatch(/\.t-hoverscroll\s*\{[^}]*scrollbar-width:\s*thin/)
    // Transparent at rest, tinted on hover — the auto-hide the class is named for.
    expect(body).toMatch(/scrollbar-color:\s*transparent transparent/)
    expect(body).toMatch(/\.t-hoverscroll:hover[\s\S]*scrollbar-color:\s*color-mix/)
  })

  it('feature-queries the fallback instead of sniffing the browser', () => {
    // @-moz-document / UA sniffing would also "work" in Firefox but would change
    // Chrome's rendering the day the query stops matching.
    expect(css).not.toMatch(/@-moz-document/)
    // The bare ::-webkit-scrollbar is a trap, not a simplification: Firefox
    // reports it as supported, so `not selector(::-webkit-scrollbar)` never
    // matches there and the fallback silently stops existing.
    expect(css).not.toMatch(/@supports not selector\(::-webkit-scrollbar\)/)
  })

  it('pairs every scrollbar-color:auto reset with a fallback selector', () => {
    const fallback = atRuleBody(css, FALLBACK_HEADER) ?? ''
    // Selectors that hand their scrollbar to the pseudo-elements. Each one is
    // invisible to Firefox and must be restated in the fallback block.
    // Strip comments first — otherwise the selector capture swallows the prose
    // block sitting between the previous rule and this one.
    const bare = css.replace(/\/\*[\s\S]*?\*\//g, '')
    const resets = [...bare.matchAll(/([^{}]+)\{[^}]*scrollbar-color:\s*auto[^}]*\}/g)]
      .map((m) => m[1].trim())
      .filter((sel) => !sel.startsWith('@'))
    expect(resets.length).toBeGreaterThan(0)
    for (const selector of resets) {
      expect(
        fallback,
        `${selector} resets scrollbar-color to auto with no Firefox fallback`,
      ).toContain(selector)
    }
  })

  it('covers every element that carries the class', () => {
    // The fallback is written against .t-hoverscroll, so any carrier is covered
    // by construction — this asserts the carriers still use the shared class
    // rather than open-coding a reset of their own.
    const carriers = []
    const walk = (dir) => {
      for (const entry of readdirSync(dir)) {
        const path = join(dir, entry)
        if (statSync(path).isDirectory()) walk(path)
        else if (entry.endsWith('.vue')) {
          const source = readFileSync(path, 'utf8')
          if (source.includes('t-hoverscroll')) carriers.push(path)
          expect(
            source,
            `${entry} resets scrollbar-color locally — use .t-hoverscroll`,
          ).not.toMatch(/scrollbar-color:\s*auto/)
        }
      }
    }
    walk(resolve(root, 'src'))
    expect(carriers.length).toBeGreaterThan(0)
  })
})
