import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { readFileSync } from 'fs'
import { iconCatalog } from './build/iconCatalog.js'

const webVersion = readFileSync('./VERSION', 'utf-8').trim()

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
  },
  plugins: [vue(), iconCatalog()],
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
