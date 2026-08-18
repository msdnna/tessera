// Curated, user-facing "What's New" highlights — newest first, keyed by the web
// VERSION they shipped in.
//
// This is deliberately NOT the raw CHANGELOG (developer-facing and noisy): only
// entries worth interrupting a user for after an update. Keep it short; add an
// entry here when a release ships something a user should notice (the release
// step is a good place to remember — see the tessera-ship skill).
//
// Entry shape:
//   version    web VERSION this shipped in (compared against __APP_VERSION__)
//   date       'YYYY-MM-DD'
//   title      short headline
//   items      array of Markdown bullet strings
//   spotlight  optional { navKey, title, body } — a one-shot arrow hint pointing
//              at a sidebar nav item, dismissed via key `spotlight:<navKey>`.
//              navKey matches Sidebar nav entries (e.g. 'documents').
export const WHATS_NEW = [
  {
    version: '0.170.0',
    date: '2026-08-18',
    title: 'Интерактивное версионирование',
    items: [
      'Версии клиента и сервера теперь видны в подвале сайдбара — наведите на строку, чтобы увидеть коммит и дату сборки.',
      'Приложение само предложит обновиться, когда выйдет новая версия — без ручной перезагрузки.',
    ],
    spotlight: {
      navKey: 'documents',
      title: 'Загляните в «Документы»',
      body: 'Вики-страницы и документы рабочего пространства — прямо в сайдбаре.',
    },
  },
]
