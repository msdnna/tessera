import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import { iconCatalog } from './build/iconCatalog.js'

export default defineConfig({
  plugins: [vue(), iconCatalog()],
  resolve: {
    alias: { '@': resolve(__dirname, 'src') },
  },
  test: {
    environment: 'jsdom',
    include: ['tests/**/*.spec.js'],
    coverage: {
      provider: 'v8',
      reporter: ['text-summary', 'html', 'lcov'],
      // Coverage counts the logic layer only (utils/stores/composables/api/
      // router). Components/views are excluded by agreement (task #2580):
      // they get smoke tests here and full coverage via e2e later.
      include: [
        'src/utils/**',
        'src/stores/**',
        'src/composables/**',
        'src/api/**',
        'src/router/**',
      ],
    },
  },
})
