import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import configPrettier from 'eslint-config-prettier'
import globals from 'globals'
import { readFileSync } from 'node:fs'

// Files still holding hardcoded Russian, maintained by the ratchet in
// tests/ut-no-ru-literals.spec.js (#2799). Read here — rather than duplicated —
// so ticking a file off the baseline is what switches the bare-string rule on
// for it: one list, one edit per file, no way for the two to drift.
const i18nTodo = JSON.parse(readFileSync(new URL('./tests/i18n-baseline.json', import.meta.url)))

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
    // Bare interface text in templates, for the parts of src/ the extraction
    // waves have already been through. `error`, not `warn`: the point is that a
    // finished screen cannot quietly regain a literal, and warnings scroll past.
    // The rule reads templates only — `<script>` text is covered by the ratchet
    // test, which is also where this ignore list comes from.
    files: ['src/**/*.vue'],
    ignores: i18nTodo.filter((f) => f.endsWith('.vue')),
    rules: {
      'vue/no-bare-strings-in-template': [
        'error',
        {
          // The rule's default allowlist (punctuation and dashes) plus the
          // product name, which is a brand and stays as it is in every locale.
          allowlist: [
            ...'(),.&+-=*/#%!?:[]{}<>|',
            '·',
            '•',
            '‐',
            '–',
            '—',
            '−',
            '→',
            // Glyphs used as icons in their own right — a close cross, the
            // month-stepper chevrons and the full-width plus on "add" buttons.
            // They are drawn, not read, and stay the same in every locale.
            '×',
            '✕',
            '✎',
            '‹',
            '›',
            '＋',
            // A lone ellipsis standing in for a spinner in the panel headers —
            // it says "working", and there is nothing in it to translate.
            '…',
            // Σ prefixes a rolled-up subtask estimate; @ prefixes a login.
            'Σ',
            '@',
            // The created/updated counter shorthand shared by the sync journal
            // and the background-jobs panel: "+3 / ~7". Signs, not words.
            '~',
            '/ ~',
            // A URL path shown verbatim next to the slug input — it is the
            // address the browser will open, not prose, and never translates.
            '/project/',
            'Tessera',
            'tessera',
            // Another brand name that is spelled the same in every locale (it
            // labels integration rows and chips, not prose about them).
            'GitLab',
            // GitLab's own identifiers, shown verbatim in the integration modal:
            // the two PAT scope names, the shape of a project path and of a token,
            // the example instance URL, and the path issue templates live at. They
            // are what the user types or what GitLab prints — translating any of
            // them would make the hint wrong, not localised.
            'api',
            'read_api',
            'group/project',
            'glpat-…',
            'https://gitlab.example.com',
            '.gitlab/issue_templates/*.md',
            // The same kind of verbatim identifier elsewhere in the shell: the
            // example server address on the sign-in screen, the key of a
            // built-in editor command (what the user types after the slash),
            // and the release-stage marker on the Documents nav item.
            'https://tessera.msdnna.website',
            'approve',
            'alpha',
          ],
          // Defaults, plus `placeholder` on any element: the UI is built on
          // Naive UI, so nearly every placeholder sits on `<n-input>` rather
          // than a bare `<input>` and the default rule would walk past it.
          attributes: {
            '/.+/': [
              'title',
              'placeholder',
              'aria-label',
              'aria-placeholder',
              'aria-roledescription',
              'aria-valuetext',
            ],
            img: ['alt'],
          },
        },
      ],
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
    // Vitest test files. Node globals are in scope alongside the browser ones
    // from the base config: specs run in jsdom, but helpers reach for `fs` and
    // `process.cwd()` to walk the source tree (tests/helpers/ruLiterals.js).
    files: ['tests/**/*.js'],
    languageOptions: {
      globals: {
        ...globals.node,
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
