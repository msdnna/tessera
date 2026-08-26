// Curated, user-facing "What's New" highlights — newest first, keyed by the web
// VERSION they shipped in.
//
// This is deliberately NOT the raw CHANGELOG (developer-facing and noisy): only
// entries worth interrupting a user for after an update. Keep it short — a
// couple of bullets per release, visible features only. Add an entry here when a
// release ships something a user should notice (the release step is a good place
// to remember — see the tessera-ship skill), and set `version` to the web
// VERSION it actually ships in.
//
// The wording itself lives in the catalogue (`src/locales/<locale>/whatsNew.json`,
// #2800), so the notes speak the interface language like everything else — this
// file holds only the structure and the key of each string. Adding a release
// means adding an entry here AND a block under the same key in both locales; the
// key-parity test (tests/cx-i18n.spec.js) fails on a half-added one.
//
// Entry shape:
//   version    web VERSION this shipped in (compared against __APP_VERSION__)
//   date       'YYYY-MM-DD'
//   titleKey   catalogue key of the headline
//   itemKeys   catalogue keys of the Markdown bullets, in order
//   spotlight  optional { navKey, titleKey, bodyKey } — a one-shot arrow hint
//              pointing at a sidebar nav item, dismissed via key
//              `spotlight:<navKey>`. navKey matches Sidebar nav entries (e.g.
//              'documents'). Only works for sidebar nav items — not arbitrary UI
//              elements.
export const WHATS_NEW = [
  {
    version: '0.177.0',
    date: '2026-08-26',
    titleKey: 'whatsNew.changelogHistory.title',
    itemKeys: ['whatsNew.changelogHistory.item1'],
  },
  {
    version: '0.176.0',
    date: '2026-08-26',
    titleKey: 'whatsNew.i18n.title',
    itemKeys: ['whatsNew.i18n.item1', 'whatsNew.i18n.item2'],
  },
  {
    version: '0.175.0',
    date: '2026-08-24',
    titleKey: 'whatsNew.helpCenter.title',
    itemKeys: [
      'whatsNew.helpCenter.item1',
      'whatsNew.helpCenter.item2',
      'whatsNew.helpCenter.item3',
    ],
    spotlight: {
      navKey: 'help',
      titleKey: 'whatsNew.helpCenter.spotlightTitle',
      bodyKey: 'whatsNew.helpCenter.spotlightBody',
    },
  },
  {
    version: '0.173.0',
    date: '2026-08-20',
    titleKey: 'whatsNew.tourDnd.title',
    itemKeys: ['whatsNew.tourDnd.item1', 'whatsNew.tourDnd.item2'],
  },
  {
    version: '0.172.0',
    date: '2026-08-20',
    titleKey: 'whatsNew.docxImport.title',
    itemKeys: [
      'whatsNew.docxImport.item1',
      'whatsNew.docxImport.item2',
      'whatsNew.docxImport.item3',
    ],
  },
  {
    version: '0.171.0',
    date: '2026-08-20',
    titleKey: 'whatsNew.getStarted.title',
    itemKeys: ['whatsNew.getStarted.item1', 'whatsNew.getStarted.item2'],
  },
  {
    version: '0.170.0',
    date: '2026-08-18',
    titleKey: 'whatsNew.versions.title',
    itemKeys: ['whatsNew.versions.item1', 'whatsNew.versions.item2'],
  },
  {
    version: '0.169.0',
    date: '2026-08-18',
    titleKey: 'whatsNew.gitlabGrouping.title',
    itemKeys: ['whatsNew.gitlabGrouping.item1'],
  },
  {
    version: '0.168.0',
    date: '2026-08-18',
    titleKey: 'whatsNew.descriptionTab.title',
    itemKeys: ['whatsNew.descriptionTab.item1'],
  },
  {
    version: '0.167.0',
    date: '2026-08-17',
    titleKey: 'whatsNew.mentionCards.title',
    itemKeys: ['whatsNew.mentionCards.item1'],
  },
  {
    version: '0.165.0',
    date: '2026-08-17',
    titleKey: 'whatsNew.commentThreads.title',
    itemKeys: ['whatsNew.commentThreads.item1'],
  },
  {
    version: '0.164.0',
    date: '2026-08-17',
    titleKey: 'whatsNew.editor.title',
    itemKeys: ['whatsNew.editor.item1', 'whatsNew.editor.item2'],
  },
  {
    version: '0.163.0',
    date: '2026-08-17',
    titleKey: 'whatsNew.documents.title',
    itemKeys: ['whatsNew.documents.item1', 'whatsNew.documents.item2'],
    spotlight: {
      navKey: 'documents',
      titleKey: 'whatsNew.documents.spotlightTitle',
      bodyKey: 'whatsNew.documents.spotlightBody',
    },
  },
]
