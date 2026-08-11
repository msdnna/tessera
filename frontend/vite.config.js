import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { readFileSync } from 'fs'
import { iconCatalog } from './build/iconCatalog.js'

const webVersion = readFileSync('./VERSION', 'utf-8').trim()

// Backend dev server runs on :8090 (8080 is taken by the budget app on this box).
const backend = process.env.TESSERA_API_TARGET || 'http://localhost:8090'

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
    proxy: {
      '/api': {
        target: backend,
        changeOrigin: true,
        ws: true, // proxy the /api/ws WebSocket upgrade too
      },
    },
  },
})
