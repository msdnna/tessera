import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'

// Regression guard for #2752: PDFs opened in a document, but only in the docker
// build — the viewer died with "Setting up fake worker failed: Failed to fetch
// dynamically imported module".
//
// The cause was nginx, not pdf.js. Vite emits the worker as an .mjs asset, and
// nginx 1.27's stock mime.types maps only .js, so the file went out as
// application/octet-stream and the browser refused to execute it as a module.
// Neither `vite dev` nor `vite preview` can reproduce it — both serve .mjs
// correctly — which is why the Playwright suite cannot cover this and a cheap
// static check of the deployed config earns its keep instead.
//
// Same on-disk-sources trick as cx-doc-editor.spec.js: __dirname does not exist
// (ESM) and import.meta.url is not a file: URL under Vite, but vitest's root is
// the process cwd, i.e. frontend/.
const root = process.cwd()

const conf = readFileSync(resolve(root, 'nginx.conf'), 'utf8')

// Strip comments so a mention of "mjs" in prose cannot satisfy the assertions.
const live = conf.replace(/#[^\n]*/g, '')

describe('frontend/nginx.conf', () => {
  it('maps .mjs to a JavaScript MIME type', () => {
    const types = [...live.matchAll(/types\s*\{([\s\S]*?)\}/g)].map((m) => m[1])
    const mapsMjs = types.some((body) => /application\/javascript[^;]*\bmjs\b[^;]*;/.test(body))
    expect(mapsMjs, 'no types{} block maps mjs → application/javascript').toBe(true)
  })

  it('declares that block at http level, so the inherited types stay', () => {
    // A types{} block inside server{} REPLACES the hash inherited from
    // mime.types instead of topping it up — css, svg and images would all turn
    // into octet-stream. conf.d is included inside http, so the block has to sit
    // before the server{} it would otherwise land in.
    const typesAt = live.indexOf('types')
    const serverAt = live.search(/\bserver\s*\{/)
    expect(typesAt).toBeGreaterThan(-1)
    expect(serverAt).toBeGreaterThan(-1)
    expect(typesAt, 'types{} must precede server{}').toBeLessThan(serverAt)
  })

  it('is needed because the pdf.js worker is an .mjs asset', () => {
    // The pairing this guard exists for. If the viewer ever stops pulling an
    // .mjs worker, this test says so rather than leaving the nginx rule looking
    // arbitrary.
    const viewer = readFileSync(resolve(root, 'src/components/documents/DocPdf.vue'), 'utf8')
    expect(viewer).toMatch(/pdf\.worker\.min\.mjs\?url/)
  })
})
