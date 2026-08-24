import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import configPrettier from 'eslint-config-prettier'
import globals from 'globals'

export default [
  // Generated output only. playwright-report/ and test-results/ are .gitignored
  // like the rest, but were missing here — so a local e2e run left hundreds of
  // lint errors from bundled vendor code in its trace viewer.
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'coverage/**',
      'playwright-report/**',
      'test-results/**',
    ],
  },
  js.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  configPrettier,
  {
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        ...globals.browser,
        __APP_VERSION__: 'readonly',
        __APP_COMMIT__: 'readonly',
        __BUILD_DATE__: 'readonly',
      },
    },
    rules: {
      'vue/multi-word-component-names': 'off',
    },
  },
  {
    // Build/config files run under Node, not the browser. scripts/ holds build
    // tooling (help-index generator) that reads the filesystem and sets an exit
    // code, so it needs the Node globals too.
    files: ['*.config.js', 'vite.config.js', 'scripts/**/*.{js,mjs}'],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    // Playwright e2e: these run in Node (fs, process, fetch), not the browser.
    // `page.evaluate` callbacks execute in the page, so browser globals stay in
    // scope too — hence both sets.
    files: ['e2e/**/*.js', 'playwright.config.js'],
    languageOptions: { globals: { ...globals.node, ...globals.browser } },
    rules: {
      // Playwright derives fixture dependencies from the destructuring pattern
      // and refuses a plain parameter, so a dependency-free fixture has to be
      // declared as `async ({}, use)`.
      'no-empty-pattern': 'off',
    },
  },
  {
    // Vitest test files.
    files: ['tests/**/*.js'],
    languageOptions: {
      globals: {
        describe: 'readonly',
        it: 'readonly',
        expect: 'readonly',
        beforeEach: 'readonly',
        afterEach: 'readonly',
        vi: 'readonly',
      },
    },
  },
]
