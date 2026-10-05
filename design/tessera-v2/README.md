# Tessera v2 — дизайн-handoff (#2951)

Экспорт из Claude Design (проект Tessera-v1-DS, `templates/board-v2/`), 2026-10-05.
Итоговое направление — **F**: вариант C «Цветные дорожки» + приоритет полосой из A,
каскад аватаров, клавиатурное управление.

- `handoff/README.md` — спецификация экранов, поведение, «что заменило каждую линию».
- `handoff/tokens-v2.css` — токены light/dark (`--v-*`; в коде перенесены в `--t-*`).
- `*.dc.html` — прототипы (React-обёртки ДС). Вне проекта Claude Design не открываются:
  `ds-base.js` грузит `_ds_bundle.js` проекта. Читать как исходник, не рендерить.
  `DirectionF` / `Board` / `Column*` / `Card` — доска; `Screens3`, `TaskModal`,
  `Gantt`, `Home`, `SearchPalette`, `Shortcuts`, `Mobile` — остальные экраны.
  `DirectionA/B/C`, `TokensA/B/C` — отклонённые направления, для справки.
- `shots/` — скриншоты макетов (тёмная тема) для сверки.
