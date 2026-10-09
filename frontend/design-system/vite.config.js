// Library build of the Tessera design system for Claude Design (#2950).
// Run from frontend/: `corepack yarn build:ds`. Produces one self-contained ESM
// file (Vue, Naive UI, Pinia, vue-i18n and the app's components inlined; only
// react/react-dom stay external — Claude Design provides them) plus one CSS file
// with every component's scoped styles and the global tokens from main.css.
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { existsSync, readFileSync } from 'fs'

const root = resolve(import.meta.dirname, '..')
const webVersion = readFileSync(resolve(root, 'VERSION'), 'utf-8').trim()

// The app references some art from public/ by absolute URL in its CSS (BrandLogo's
// masks: url(/mark-white.svg)). The web server serves those; a design has no such
// server, so inline every public/ file a stylesheet points at as a data: URI.
const MIME = { svg: 'image/svg+xml', png: 'image/png', webp: 'image/webp', jpg: 'image/jpeg' }
function inlinePublicUrls() {
  const pub = resolve(root, 'public')
  return {
    name: 'tessera-ds-inline-public-urls',
    enforce: 'pre',
    transform(code, id) {
      if (!/\.(css|vue)(\?|$)/.test(id) || !code.includes('url(/')) return null
      const out = code.replace(/url\(\/([\w.-]+\.(svg|png|webp|jpg))\)/g, (m, file, ext) => {
        const abs = resolve(pub, file)
        if (!existsSync(abs)) return m
        const b64 = readFileSync(abs).toString('base64')
        return `url("data:${MIME[ext]};base64,${b64}")`
      })
      return out === code ? null : { code: out, map: null }
    },
  }
}

export default defineConfig({
  root: import.meta.dirname,
  plugins: [inlinePublicUrls(), vue()],
  resolve: {
    alias: [
      // See src/stubs/docImport.js — keeps the document editor and pdf.js (~3 MB)
      // out of the bundle. Before the generic `@/` alias, which would win otherwise.
      {
        find: /^@\/utils\/docImport(\.js)?$/,
        replacement: resolve(import.meta.dirname, 'src/stubs/docImport.js'),
      },
      { find: /^@\//, replacement: resolve(root, 'src') + '/' },
      // See src/stubs/mermaid.js — keeps ~8 MB of diagram engine out of the bundle.
      { find: /^mermaid$/, replacement: resolve(import.meta.dirname, 'src/stubs/mermaid.js') },
    ],
  },
  define: {
    __APP_VERSION__: JSON.stringify(webVersion),
    __APP_COMMIT__: JSON.stringify(''),
    __BUILD_DATE__: JSON.stringify(''),
    'process.env.NODE_ENV': JSON.stringify('production'),
    __VUE_OPTIONS_API__: 'true',
    __VUE_PROD_DEVTOOLS__: 'false',
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: 'false',
  },
  build: {
    outDir: resolve(import.meta.dirname, 'dist'),
    emptyOutDir: true,
    sourcemap: false,
    minify: true,
    cssCodeSplit: false,
    // Fonts, the brand masks and icons all ride inside the bundle.
    assetsInlineLimit: () => true,
    lib: {
      entry: resolve(import.meta.dirname, 'src/index.js'),
      formats: ['es'],
      fileName: () => 'index.js',
      cssFileName: 'style',
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react/jsx-runtime', 'react-dom/client'],
      output: { codeSplitting: false },
    },
  },
})
