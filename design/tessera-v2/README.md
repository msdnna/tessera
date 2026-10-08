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

## Пакеты A–H — v2 на всё приложение (эпик #2966), экспорт 2026-10-07

Брифы: `frontend/.design-sync/brief-v2-app.md` (A–G), `brief-v2-tour-spotlight.md` (H),
`brief-v2-toasts.md` (I) и `brief-v2-props.md` (J).
Начинать с `Package<X>.dc.html`: там кадры, спецификация под ними и раздел
«Отступления от handoff» (где макет сознательно расходится с текущим кодом).
`Catalog<X>.dc.html` — лист состояний примитивов пакета. Остальные файлы —
компоненты, которые импортируют пакеты.

| Пакет | Файлы | Задача |
|---|---|---|
| A. Модель взаимодействия | `PackageA`, `CatalogA`, `PopForm`, `PaletteForm`, `UndoToast`, `MenuA`, `SidebarA`, `ShellA`, `FrameA`, `FragA` | #2969 |
| B. «Входящие» | `PackageB`, `CatalogB`, `CapturePop`, `DockPanel`, `InboxItem`, `SnoozeMenu`, `HomeB`, `TaskModalB`, `MobileB` | #2972 |
| C. Проект, теги, этапы | `PackageC`, `CatalogC`, `ProjectWin`, `TagPicker`, `StagePage`, `StageStrip`, `StageMenu`, `MobileStage` | #2971 |
| D. Доска | `PackageD`, `CatalogD`, `CustomPanel`, `ViewMenu`, `ListView`, `CalendarView`, `MatrixView`, `MobileD` | #2970, #2973 |
| E. Экраны | `PackageE`, `CatalogE`, `DocsPage`, `SettingsPage`, `AdminPage`, `MeetPage`, `CallScreen`, `MiniCall`, `AuthPage`, `AuthSheet`, `HelpWin`, `WhatsNew`, `NotifPop`, `TemplatesWin` | #2974 |
| F. Крупные окна | `PackageF`, `CatalogF`, `GitlabWin`, `ConflictWin`, `JobsWin`, `MembersWin`, `CmdsWin` | #2975 |
| G. Мобильный | `PackageG`, `CatalogG`, `MobileG`, `MG*` (`MGNavB` — нижняя навигация, `MGDrawer` — дровер) | #2976 |
| H. Тур и spotlight | `PackageH`, `CatalogH`, `TourPop`, `TourLayer`, `TourChecklist`, `SpotCard`, `HintsPane`, `WhatsNewV2` | #3000 |
| I. Тосты | бриф `brief-v2-toasts.md`, кадры текущего состояния `current-toasts/` — макетов ещё нет | #3117 |
| J. Поповеры свойств задачи и редакторы | бриф `brief-v2-props.md`, кадры текущего состояния `current-props/` — макетов ещё нет | #3118 |

`HANDOFF-BUNDLE.md` — README самого экспорта. Полный архив (с бандлом ДС и рабочими
кадрами Design) — `design/Tessera-v1-DS-handoff.zip`, в git не кладём.
