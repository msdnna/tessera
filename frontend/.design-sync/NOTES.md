# Claude Design sync — notes (#2950)

Project: **Tessera-v1-DS** (`projectId` in `config.json`). Config home = `frontend/`
(run every command below from `frontend/`).

## How this DS is built (not a stock React DS)

- Tessera's web UI is **Vue 3 + Naive UI**; Claude Design consumes React. The DS
  package `frontend/design-system/` is a thin React layer: each export mounts the
  **real Vue SFC from `frontend/src/components`** as its own small Vue app (an
  "island", `design-system/src/runtime/island.js`) with Pinia, vue-i18n and Naive's
  config/message/dialog providers. React children reach Vue slots through portals
  into a placeholder `<span class="tds-slot">`; two-way values (`value`/`onChange`,
  uncontrolled `defaultValue`) go through the island's `model` option.
- Types are **hand-written** in `design-system/types/index.d.ts` (no TSX source to
  derive them from). Keep them in step with the Vue `defineProps` when a component
  changes. Object shapes are inlined per prop — the converter copies each
  `<Name>Props` body verbatim, so a shared named type would dangle.
- Build: `corepack yarn build:ds` (Vite lib mode, `design-system/vite.config.js`) →
  `design-system/dist/index.js` (+ `style.css`, which is `cfg.cssEntry`). Everything
  but react/react-dom is inlined; minified (~3.2 MB / ~0.8 MB gzip).
- `mermaid` is aliased to `design-system/src/stubs/mermaid.js` in the DS build: it
  was ~8 MB of a 10.7 MB bundle. RichContent catches the stub's throw, so ```mermaid
  blocks stay as code in MarkdownEditor's preview.

## Environment

- Converter deps (esbuild, ts-morph, @types/react, react@19, react-dom@19,
  playwright@1.62.0) live in `.ds-sync/node_modules` (gitignored, staged per run).
  `--node-modules .ds-sync/node_modules`, `--entry ./design-system/dist/index.js`.
- `design-system/node_modules` must be a symlink to `../.ds-sync/node_modules`
  (gitignored; recreate per clone: `ln -sfn ../.ds-sync/node_modules design-system/node_modules`).
  Without it ts-morph can't find `@types/react` from the package dir → `[DTS_REACT]`
  and ReactNode/CSSProperties collapse to `any`.
- playwright **1.62.0** matches the cached `~/.cache/ms-playwright/chromium-1234`.
  Never run `playwright install` here (see CLAUDE.md).
- A full `package-capture.mjs` run takes ~10+ minutes (every cell boots Vue islands);
  run it in the background.

## Gotchas found while making previews render

- **One provider owns `:root`.** `TesseraProvider` writes `--t-*` on its own wrapper
  (`.tds-root`, a plain block) and only the OUTERMOST one writes `:root` (body
  background, teleported popovers). Each island's copy of the app theme store also
  writes `:root` on theme change; islands call `reapplyRoot()` in a post-flush
  watcher so a nested dark provider can't repaint the page.
- `.tds-root` was `display: contents` at first → validator measured 0px height for
  every preview (`[RENDER_THIN]`). It is a normal block now.
- `--t-accent-grad*` are declared on `:root` in terms of `--t-primary`; a custom
  property is computed where declared and inherits as the finished gradient, so a
  nested accent never reached them. `.tds-root` re-declares the three (same
  formulas as main.css — keep in sync). SVG gradient icons (`url(#t-accent-grad-svg)`)
  still follow the root provider only: one id, defs live in the outermost provider.
- BrandLogo's masks point at `url(/mark-white.svg)` / `url(/wordmark-white.svg)` —
  files in `frontend/public/` the web server serves. The DS Vite config inlines any
  `url(/<public file>)` in CSS as a data: URI (`inlinePublicUrls` plugin); without
  it the logo rendered empty.
- Dates on cards are formatted by the theme store's `language`, not vue-i18n's
  locale; islands set both (headless Chrome is en-US → "Sun", "24 May" otherwise).
- Popover triggers (Tooltip) need a real box: slot spec `{ name, box: true }` renders
  `.tds-slot-box` (inline-flex) instead of a `display: contents` placeholder, which
  `getBoundingClientRect` reports as 0×0.
- Call-site styling the app applies around a component is NOT part of the SFC:
  TagPill (padding/radius/size), UserAvatar (24px gradient circle) get the board
  card's metrics in `design-system/src/styles/island.css`. ProjectIcon's 22px badge
  box and the kanban column chrome stay recipes (conventions header + previews).
- Scoped-tag friendly names are keyed WITH the separator: `{ 'app::': 'App' }`.
- ColumnHeader shows the status glyph and menu only with `editable` (every real
  board column is editable). The collapsed strip is drawn by KanbanBoard, not by
  ColumnHeader — no preview for it.
- A scoped tag (`scope::name`) always renders two-tone; TagPill `variant` only
  changes unscoped tags.
- KanbanBoard's scoped `.col-head` rule (8px bottom margin) reaches ColumnHeader's
  root only inside the board; compositions add it on the host (`style`).
- Avatar images: members passed with `user_id` make UserAvatar request
  `/api/users/<id>/avatar` (404 outside the app) and fall back to initials — benign.

## Claude Design token check (#2951)

- Claude Design lints `_ds_bundle.css` custom properties. It flagged two (fixed in
  `src/styles/main.css`): `--t-error-grad` as a gradient of bare literals (minified
  to plain hex stops it can't classify), and `--bgrad`, a helper colour property
  declared on a global selector (`.n-button--ghost`). Build theme gradients on a
  colour token (`var(--t-error)`, `var(--t-primary)`) and don't add helper colour
  properties on global selectors — spell the value out. A `/* @kind color */`
  annotation would not help: the DS build minifies CSS and strips comments.
- Scoped helpers inside SFCs (`--card-fill`, `--sub-bg`, `--ghost`…) pass on a
  plain class rule, but Claude Design flags ANY custom property re-declared under a
  state selector — colour or not (2026-10-06/07, #2966): first
  `.card:hover { --card-fill }`, then a numeric `--_hov: 0|1` flag (both the base
  and the :hover declaration). Under `:hover`/`:focus`… set the real property
  (`background`), never a variable; descendants that read the variable get their
  own plain hover rule (TaskCard: avatar ring via `.card:hover :deep(.avatar)`).
  `/* @kind other */` hints don't survive — the DS build strips comments.

## Known render warns

- `[FONT_MISSING] "JetBrains Mono"` — it is the 3rd entry of a monospace stack
  (`ui-monospace, SFMono-Regular, 'JetBrains Mono', Menlo, Consolas, monospace`) in
  MarkdownEditor/WorkspaceCommandsModal; the app itself never ships it either.
  The system monospace substitute was accepted by the user (2026-10-02, #2950);
  revisit only if a v2 design picks a brand mono font.
- `[RENDER_THIN] Icon / TesseraIcon` ("paint nothing" heuristic) — glyph-only
  previews; the sheets show every glyph.
- `[RENDER_THIN] Modal` ("rendered height is 0px") — the NModal teleports to `body`
  with fixed positioning, so the card root measures 0px; the screenshot shows the
  whole dialog (checked 2026-10-05, #2951).

## Re-sync risks

- The project also holds user-authored files the sync does not own:
  `templates/board-v2/**` (v2 redesign mock-ups, #2951), `shots/**`, `uploads/**`.
  The standard writes globs never touch them and anchored deletes come only from
  `upload.deletePaths`; never add these paths to a plan's `deletes`, and on an
  un-anchored re-sync leave them out of the "files this build doesn't produce" review.
- `types/index.d.ts` is hand-maintained: a prop added/renamed in a Vue SFC is
  invisible to the design agent until mirrored there.
- `island.css` duplicates call-site metrics (TagPill chip, avatar) from
  `TaskCardPills.vue`; if the card's pill/avatar CSS changes, update it.
- The kanban column recipe in `conventions.md` and the `Board` / `ColumnHeader`
  previews copy KanbanBoard.vue `.col` CSS — v2 lanes (#2954): 256px, 8px padding,
  18px radius, 8px gap, `color-mix(<col> var(--t-col-tone), var(--t-col))`, no
  border. The status palette (teal/blue/orange/green) mirrors the backend's
  `defaultColumns` (handlers/boards.go).
- The v2 surface/shadow tokens (`--t-col`, `--t-card`, `--t-pop`, `--t-sh-*`…) are
  published at runtime by `themeVars()` (styles/tokens.js), not by a stylesheet:
  they are in `_ds_bundle.js`, not `_ds_bundle.css`. Validate conventions names
  against the bundle text for them.
- The interaction kit (#2969: InlineEdit, PopoverForm, FormField, ActionMenu,
  UndoToast, ColorIconPicker) renders in its DS-only modes: PopoverForm/ActionMenu
  `inline` (no anchor, no teleport), InlineEdit `teleportHint: false` (hint
  absolutely under the field, not on `body`). A change to the anchored/teleported
  path in the SFC is invisible in the cards; check the app too. ActionMenu reacts
  to `arm` with `immediate` — the card mounts it already open.
- `UndoToast`, `InlineEdit`, `ColorIconPicker`, `FormField` are `cardMode: column`
  (their stories are wider than a grid cell — validate flags `GRID_OVERFLOW`).
- `design-system/dist/` is NOT rebuilt by the driver: run `corepack yarn build:ds`
  after any SFC change, or the driver happily re-bundles the stale dist.
- The mermaid stub relies on RichContent wrapping `initialize()` in try/catch.
- Converter deps are network-installed from npm (react@19 latest at sync time).

## Environment quirk: slow loopback

- In this WSL (mirrored networking) loopback HTTP moves ~330 KB/s regardless of
  server (node or python) and proxy settings: the 4 MB `_ds_bundle.js` takes ~12 s,
  so a preview's `load` sits right under `package-validate`'s 15 s `page.goto`
  timeout. One random card fails with `[RENDER] page.goto: Timeout 15000ms exceeded`
  in most validate runs (2026-10-06: TesseraProvider, ColumnHeader, Card in three
  consecutive runs) — a different card each time, so it is not a render failure.
  The driver's verdict then reads `validate: false`; re-run `package-validate.mjs`
  standalone on the same build until it exits 0 (1–3 tries), then capture/upload.
  Since 2026-10-07 (#2969 re-sync) the staged copies are patched locally instead:
  `sed -i 's/timeout: 15000 }/timeout: 45000 }/' .ds-sync/package-validate.mjs`
  and `s/timeout: 15_000 }/timeout: 45_000 }/` in `.ds-sync/package-capture.mjs`
  (sheet pages). Re-apply after every re-copy of the scripts — with it the driver
  ran clean three times in a row.
  Keep the bundle small (mermaid stub, minify) for this reason too.
- The same link slows the upload: `write_files` of the 3.2 MB `_ds_bundle.js` can
  hit the tool's 60 s client timeout (2026-10-06). Upload it alone in its own call
  and simply retry — writes are idempotent; the retry went through.
