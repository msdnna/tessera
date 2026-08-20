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
// Entry shape:
//   version    web VERSION this shipped in (compared against __APP_VERSION__)
//   date       'YYYY-MM-DD'
//   title      short headline
//   items      array of Markdown bullet strings
//   spotlight  optional { navKey, title, body } — a one-shot arrow hint pointing
//              at a sidebar nav item, dismissed via key `spotlight:<navKey>`.
//              navKey matches Sidebar nav entries (e.g. 'documents'). Only works
//              for sidebar nav items — not arbitrary UI elements.
export const WHATS_NEW = [
  {
    version: '0.172.0',
    date: '2026-08-20',
    title: 'Импорт Word и устойчивость сессии',
    items: [
      'Импорт `.docx` сохраняет форматирование: цвета и размеры текста, горизонтальные линии, блоки кода, заливку и рамки таблиц. В тёмной теме цвета из Word остаются читаемыми.',
      'Кратковременный обрыв связи с сервером больше не выкидывает на экран входа — сессия восстанавливается сама.',
      'Уведомление о новой версии появляется сразу, без перезагрузки страницы.',
    ],
  },
  {
    version: '0.171.0',
    date: '2026-08-20',
    title: 'Пошаговое обучение Get Started',
    items: [
      'Новым пользователям — интерактивный путеводитель из 12 шагов: от выбора пространства до создания проекта, доски и первой задачи, а затем знакомство с инструментами доски и разделами. Запустить заново можно в любой момент — пункт «Обучение» внизу боковой панели.',
      'Просмотр PDF в документах снова работает: воркер pdf.js больше не блокируется, а наложение проходов отрисовки устранено.',
    ],
  },
  {
    version: '0.170.0',
    date: '2026-08-18',
    title: 'Версии сервисов и автообновление',
    items: [
      'Версии клиента и сервера теперь видны в правом нижнем углу — наведите, чтобы увидеть коммит и дату сборки.',
      'Приложение само предложит обновиться, когда выйдет новая версия, — без ручной перезагрузки.',
    ],
  },
  {
    version: '0.169.0',
    date: '2026-08-18',
    title: 'GitLab: группировка и статусы подзадач',
    items: [
      'Задачу можно пометить сгруппированной в GitLab, а у подзадач во вкладке «Подзадачи» виден их GitLab-статус со ссылкой на issue.',
    ],
  },
  {
    version: '0.168.0',
    date: '2026-08-18',
    title: 'Описание отдельной вкладкой',
    items: [
      'В режиме боковой панели описание задачи вынесено в первую вкладку — быстрее добраться до комментариев и подзадач.',
    ],
  },
  {
    version: '0.167.0',
    date: '2026-08-17',
    title: 'Карточки упоминаний',
    items: [
      'Наведение на @упоминание в описании и комментариях показывает карточку пользователя: аватар, имя, логин и роль.',
    ],
  },
  {
    version: '0.165.0',
    date: '2026-08-17',
    title: 'Ветки ответов в комментариях',
    items: [
      'Ответы отображаются веткой под комментарием — с отступом и сворачиванием; «Ответить» подставляет упоминание автора.',
    ],
  },
  {
    version: '0.164.0',
    date: '2026-08-17',
    title: 'Прокачанный редактор описаний',
    items: [
      'Панель форматирования: заголовки, списки, чекбоксы, цитаты, спойлеры; автозакрытие скобок и продолжение списков.',
      'Полноэкранный редактор с живым предпросмотром и ссылки `#N` на задачи и вложения прямо в тексте.',
    ],
  },
  {
    version: '0.163.0',
    date: '2026-08-17',
    title: 'Документы и макеты задачи',
    items: [
      'Новый раздел «Документы» (alpha): вики-страницы с блочным редактором — вложенность, автосохранение, drag-ручка блоков и меню по «/».',
      'Выбор макета задачи: модальное окно, полный экран или панель справа (запоминается на устройстве).',
    ],
    spotlight: {
      navKey: 'documents',
      title: 'Загляните в «Документы»',
      body: 'Вики-страницы и документы рабочего пространства доступны уже сейчас.',
    },
  },
]
