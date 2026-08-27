import base from './playwright.config.js'

// The locale visual pass (#2800, step 4) reuses the whole e2e harness — same
// throwaway backend, same global setup, same per-run seeded account — and only
// swaps the test directory. It is kept out of `e2e/specs` deliberately: the pass
// is run twice on purpose (`E2E_LANG=ru`, then `E2E_LANG=en`) to produce two
// comparable sets of frames, which is not what `make test-e2e-frontend` should
// spend its time on.
export default {
  ...base,
  testDir: './e2e/locale',
  reporter: [['list']],
}
