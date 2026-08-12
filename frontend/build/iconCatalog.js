import { readFileSync } from 'fs'
import { createRequire } from 'module'
import { dirname, join } from 'path'

const VIRTUAL_ID = 'virtual:icon-catalog'
const RESOLVED_ID = '\0' + VIRTUAL_ID

// The icon picker needs every ionicons5 icon at once, but `import('@vicons/ionicons5')`
// asks for the whole namespace of the same barrel module that ~50 components import
// statically. That pins the barrel unshakeable and drags all 1334 icons (~1.2 MB) into
// the preloaded entry chunk. This module re-exports the icons via their deep paths, so
// the dynamic side never touches the barrel and lands in its own lazy chunk.
export function iconCatalog() {
  return {
    name: 'tessera-icon-catalog',
    resolveId(id) {
      return id === VIRTUAL_ID ? RESOLVED_ID : null
    },
    load(id) {
      if (id !== RESOLVED_ID) return null
      const require = createRequire(import.meta.url)
      const esDir = join(dirname(require.resolve('@vicons/ionicons5/package.json')), 'es')
      // Take the names from the barrel itself rather than from a directory listing:
      // that keeps the catalog exactly in step with the package's public API (`es/`
      // also holds internal files such as `async-index.js`).
      const barrel = readFileSync(join(esDir, 'index.js'), 'utf-8')
      const names = [...barrel.matchAll(/export \{ default as (\w+) \}/g)].map((m) => m[1])
      if (!names.length) {
        this.error('icon catalog: no icons found in @vicons/ionicons5/es/index.js')
      }
      return names
        .map((n) => `export { default as ${n} } from '@vicons/ionicons5/es/${n}'`)
        .join('\n')
    },
  }
}
