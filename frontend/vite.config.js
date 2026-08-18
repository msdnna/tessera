import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'
import { resolve } from 'path'
import { readFileSync } from 'fs'
import { execSync } from 'child_process'
import { iconCatalog } from './build/iconCatalog.js'

const webVersion = readFileSync('./VERSION', 'utf-8').trim()

// Build metadata for the sidebar version tooltip (#2747). The Docker build
// context is ./frontend (no .git), so CI/deploy passes GIT_COMMIT / BUILD_DATE
// as env; a local build reads git directly and stamps "now". Empty commit is
// tolerated (the tooltip just omits it).
function gitCommit() {
  if (process.env.GIT_COMMIT) return process.env.GIT_COMMIT
  try {
    return execSync('git rev-parse --short HEAD').toString().trim()
  } catch {
    return ''
  }
}
const webCommit = gitCommit()
const buildDate = process.env.BUILD_DATE || new Date().toISOString()

// Backend dev server runs on :8090 (8080 is taken by the budget app on this box).
const backend = process.env.TESSERA_API_TARGET || 'http://localhost:8090'

// Shared by `server` and `preview`: `vite preview` does NOT inherit `server.proxy`,
// and the e2e suite (#2710) runs against the built bundle, where a missing proxy
// means 404 on every /api call and a failed /api/ws upgrade.
const apiProxy = {
  '/api': {
    target: backend,
    changeOrigin: true,
    ws: true, // proxy the /api/ws WebSocket upgrade too
  },
}

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(webVersion),
    __APP_COMMIT__: JSON.stringify(webCommit),
    __BUILD_DATE__: JSON.stringify(buildDate),
  },
  plugins: [
    vue(),
    iconCatalog(),
    // Service worker for auto-update on deploy (#2748). Prompt mode: the SW
    // installs in the background and waits; the app shows an "update ready"
    // toast and only reloads when the user confirms (registration + the
    // needRefresh flag are driven from src/composables/useAppUpdate.js via the
    // virtual:pwa-register/vue module). We deliberately do NOT cache the API or
    // the WebSocket — navigateFallbackDenylist keeps /api on the network, so
    // refresh-on-401 and realtime are untouched.
    VitePWA({
      registerType: 'prompt',
      injectRegister: null,
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,ico,woff2}'],
        // Keep the precache to the app shell + board. The heavy, lazily-loaded
        // feature/vendor bundles (documents, PDF, diagrams, the icon catalog)
        // are hashed and cache fine over HTTP on demand — precaching them would
        // push ~7 MB into every first visit for no benefit to update detection.
        globIgnores: [
          '**/pdf-*.js',
          '**/katex-*.js',
          '**/cytoscape*.js',
          '**/mermaid*.js',
          '**/DocumentsView-*.js',
          '**/_virtual_icon-catalog-*.js',
        ],
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api/],
        cleanupOutdatedCaches: true,
      },
      manifest: {
        name: 'Tessera',
        short_name: 'Tessera',
        description: 'Tessera — таск-трекер экосистемы msdnna',
        theme_color: '#7c5cff',
        background_color: '#ffffff',
        display: 'standalone',
        icons: [{ src: '/icon-192.png', sizes: '192x192', type: 'image/png' }],
      },
    }),
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5174,
    proxy: apiProxy,
  },
  preview: {
    host: '0.0.0.0',
    port: Number(process.env.TESSERA_PREVIEW_PORT) || 4174,
    proxy: apiProxy,
  },
})
