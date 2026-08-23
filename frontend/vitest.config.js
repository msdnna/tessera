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
    // Installs vue-i18n into every mount (#2799). Without it, the first
    // component that renders `$t` takes down every spec that mounts it.
    setupFiles: ['./tests/setup.js'],
    coverage: {
      provider: 'v8',
      reporter: ['text-summary', 'html', 'lcov'],
      // Coverage now spans components/views too (they were whitelisted out
      // under #2580), but only the logic layer is a *gate*. A single global
      // number over 30k+ lines of mostly-untested .vue would sit in the single
      // digits and be useless as a regression signal, so the threshold below
      // applies to the logic layer only; components/views stay visible in the
      // html report and grow via component tests (#2670, #2669).
      include: ['src/**'],
      exclude: ['src/main.js', 'src/**/*.d.ts'],
      thresholds: {
        // Measured floor for the logic layer (see #2669); keep at/under the
        // current figure so a real regression trips it, not normal churn.
        'src/{utils,stores,composables,api,router}/**': {
          statements: 76,
          branches: 73,
          functions: 63,
          lines: 77,
        },
      },
    },
  },
})
