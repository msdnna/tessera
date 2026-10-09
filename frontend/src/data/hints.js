// Spotlight hints of v2 (#3000, mockup PackageH «Подсказки на элементах v2»).
// Pure data; the queue lives in stores/hints.js, the card in
// components/tour/SpotlightHint.vue. Wording: `hints.items.<key>.{title,body}`
// (+ `meta` — the reason a contextual hint shows, `wait` — what it waits for).
//
// Two kinds:
//   new — something that changed in v2. Offered to a v1 account only after
//         «Позже» on the v2 card (`mini`: the same point is a step of the «Что
//         изменилось» mini-tour, and walking that tour marks it shown), or once
//         the v2 card was answered at all (`after: 'v2'`).
//   ctx — a hint at first encounter: it waits until its element first shows up
//         on screen (the first board with a stage, the first undo toast).
//
// A dismissed hint is the ack `spotlight:<key>` — shared with Android, like the
// release spotlights (`spotlight:<navKey>`, data/whatsNew.js), which ride the
// same queue as `new` hints anchored on the sidebar item.
//
// `anchor` is a CSS selector; `mobile` is the anchor on a phone (the bottom
// navigation) — a hint without one waits for the desktop.
export const HINTS = [
  {
    key: 'inbox',
    kind: 'new',
    mini: true,
    anchor: '[data-testid="inbox-button"]',
    mobile: '[data-testid="mobile-tab-inbox"]',
  },
  {
    key: 'capture',
    kind: 'new',
    mini: true,
    anchor: '[data-testid="quick-fab-button"]',
    mobile: '[data-testid="quick-fab-button"]',
  },
  { key: 'stage-menu', kind: 'new', mini: true, anchor: '[data-testid="stage-select"]' },
  { key: 'stage-strip', kind: 'ctx', anchor: '[data-testid="stage-strip"]' },
  { key: 'view-options', kind: 'new', mini: true, anchor: '[data-testid="board-customize-btn"]' },
  { key: 'view-chip', kind: 'ctx', anchor: '[data-testid="view-chip"]' },
  { key: 'project-settings', kind: 'new', mini: true, anchor: '[data-tour="crumb-project"]' },
  { key: 'palette', kind: 'new', mini: true, anchor: '[data-tour="ws-search"]' },
  { key: 'undo', kind: 'ctx', anchor: '[data-testid="undo-toast"]' },
  { key: 'rail', kind: 'new', after: 'v2', anchor: '[data-tour="ws-switch"]' },
]

export const hintByKey = (key) => HINTS.find((h) => h.key === key) || null
