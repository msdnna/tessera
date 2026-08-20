---
name: tessera-ship
description: Release hygiene for a finished Tessera change — on a feature branch add a changelog fragment + conventional commit (NO VERSION/CHANGELOG edits); on develop assemble fragments and bump versions. Use after a verified backend/frontend/android fix or feature. Does NOT push or tag (the user does that).
---

# tessera-ship

Финализирует проверенное изменение. **Ключевое правило (во избежание вечных конфликтов
на `CHANGELOG.md`/`VERSION`):** фича-ветка кладёт запись как **фрагмент** и НЕ трогает
`VERSION`/`CHANGELOG.md`; **бамп версий и сборка changelog делаются на `develop`**.
Push/теги — пользователь сам.

Предусловие: изменение проверено, quality-gate зелёный
(`make lint-<comp>` + `make test-<comp>`; backend часто ещё e2e — см. скилл tessera-e2e).

## Модель веток

- **Одна ветка на РОДИТЕЛЬСКУЮ задачу**, а не на подзадачу. Все подзадачи родителя
  коммитятся в её ветку (`feat/<родитель#>-slug`), каждая — отдельным conventional-коммитом
  с `#номером`. Так у ревьюера одна поверхность на родителя, но погранульность по коммитам
  сохраняется. Не плодить ветку на каждую подзадачу — пользователь не хочет прыгать между
  ветками на ревью.
- **Миграции — последовательные** (`golang-migrate` линеен). Номер `NNNN_` занимай
  **перед самым merge**, не в начале работы. Разбирай задачи так, чтобы миграционные шли
  **после** безмиграционных — тогда параллельных номеров почти не возникает. Коллизию
  (два одинаковых номера при merge) чинить renumber'ом более позднего — механический шаг.

## A. На фича-ветке (заканчивая задачу/подзадачу)

### 1. Добавить changelog-фрагмент (вместо правки CHANGELOG.md)

Один файл на задачу в `changelog.d/<component>/<task#>[-slug].md`
(`component` = `backend`|`frontend`|`android`|`desktop`|`mcp`):

```bash
make changelog-add COMP=backend TASK=2620 SLUG=ws-auth   # + опц. BUMP=minor
```

Заполни буллет в стиле существующего CHANGELOG:

```markdown
---
bump: patch            # необязательно; по умолчанию из типа: feat→minor, feat!→major, иначе patch
---
- **fix(api): краткое описание (#2620).**
  Опциональные детали с отступом.
```

- Имя файла завязано на номер задачи → фрагменты разных задач **не конфликтуют** при merge.
- `bump:` указывай явно только чтобы переопределить инференс по типу коммита.
- **VERSION и CHANGELOG.md на ветке НЕ трогаем.** Детали формата — `changelog.d/README.md`.

### 2. Conventional commit (локально, без push/tag)

```bash
git add -A
git commit -m "fix(api): краткое описание (#2620)"
```
Типы: `feat`/`fix`/`docs`/`chore`/`refactor`/`test`/`perf`/`style`. Скоупы: `api`/`backend`,
`web`/`ui`, `android`, `desktop`, `mcp`. Git-identity: `msdnna` / `extracker0mail@gmail.com`
(НЕ harness-email). **Не пушить, не тегать.**

## B. На develop (после merge веток — сборка релиза)

```bash
make changelog-release DRY=1      # показать план, ничего не меняя
make changelog-release            # собрать все компоненты с фрагментами
make changelog-release ONLY=backend,frontend
```

`tools/changelog-release.py` для каждого компонента с фрагментами: поднимает `VERSION`
(уровень = максимум по фрагментам), вставляет `### [x.y.z] — ДАТА` сверху нужной секции
`CHANGELOG.md` (android → `## x.y.z — ДАТА` в `android/CHANGELOG.md`), удаляет фрагменты.
Затем:

### Frontend: обновить «Что нового» (whatsNew.js)

**Если бампнулась web-версия — добавь запись в `frontend/src/data/whatsNew.js`** (новейшая
первой, `version` = новый web `VERSION`). Это НЕ копия CHANGELOG: только то, что **заметит
пользователь** (новые фичи, видимые улучшения, заметные фиксы), человеческим языком, без
деталей для разработчика (внутренние рефакторы, тесты, CI, детали реализации — пропускаем).
Коротко — пара буллетов на релиз. Если пользователю в этом релизе замечать нечего
(только внутренние `chore`/`refactor`/`test`) — записи не делаем.

Форма записи описана в шапке `whatsNew.js` (`version`/`date`/`title`/`items`/`spotlight`).

**Крупную фичу дополни `spotlight`-поповером** (указатель + подсказка на пункт сайдбара):
новый раздел сайдбара, заметно изменённый вид, перенос привычных кнопок/элементов
управления. `spotlight: { navKey, title, body }`, где `navKey` совпадает с nav-пунктом
сайдбара (напр. `'documents'`) — работает **только** для пунктов навигации сайдбара, не для
произвольных элементов. Показывается один раз после закрытия модалки, гасится ключом
`spotlight:<navKey>`.

Highlights бери из собранной секции `CHANGELOG.md` (шаг выше) — переформулируй под
пользователя. `corepack yarn eslint src/data/whatsNew.js` перед коммитом. Правку клади в
тот же `chore(release)`-коммит.

```bash
git diff                          # проверить вставку и бамп
git add -A && git commit -m "chore(release): api <ver> / web <ver>"
```

## Чеклист
- Фича-ветка: [ ] quality-gate зелёный · [ ] фрагмент в `changelog.d/<comp>/` · [ ]
  conventional-commit · [ ] VERSION/CHANGELOG.md НЕ тронуты · [ ] без push/tag ·
  [ ] миграция (если есть) — номер занят перед merge
- develop: [ ] `make changelog-release` · [ ] diff проверен · [ ] если бампнулась web —
  запись в `whatsNew.js` (user-facing; крупная фича → `spotlight`) · [ ] `chore(release)`-коммит
  без push/tag
