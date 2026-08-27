<p align="right"><b>Русский</b> · <a href="README.en.md">English</a></p>

# MCP-сервер Tessera

Сервер [Model Context Protocol](https://modelcontextprotocol.io), который отдаёт
рабочее пространство Tessera AI-агенту (например, Claude Code) как **ранжированную
по приоритету очередь задач** — для автоматизированной агентной разработки прямо из
Tessera.

Это тонкий **REST-клиент** API Tessera (в базу он не ходит), общается по MCP через
**stdio**, поэтому MCP-клиент запускает его как подпроцесс.

## Инструменты (tools)

### Чтение

| Инструмент | Что делает |
|------------|------------|
| `tessera_list_workspaces` | Рабочие пространства, к которым принадлежит аккаунт |
| `tessera_list_projects` | Проекты в рабочем пространстве |
| `tessera_list_boards` | Доски в проекте |
| `tessera_list_columns` | Колонки доски (id, имя, позиция, флаг done) |
| `tessera_resolve_board` | `/project/<slug>/board/<slug>` → доска |
| `tessera_list_tasks` | Задачи доски как **ранжированная** очередь (опц. с привязкой к этапу) |
| `tessera_my_tasks` | Задачи, назначенные на вас в пространстве, ранжированные |
| `tessera_next_task` | Единственная самая приоритетная выполнимая задача (пропускает заблокированные на доске) |
| `tessera_get_task` | Полные детали задачи: описание, теги, исполнители, подзадачи, комментарии (с автором + `is_agent`), связи, вложения, ссылки на изображения, GitLab-линк |
| `tessera_view_image` | Забрать и вернуть изображение(я) задачи (инлайн-картинки + вложения), чтобы агент реально видел скриншоты/схемы |

### Запись

| Инструмент | Что делает |
|------------|------------|
| `tessera_add_comment` | Оставить Markdown-комментарий к задаче/подзадаче; опц. `image_paths` — загрузить и встроить локальные скриншоты инлайн |
| `tessera_update_description` | Дописать Markdown в описание — `append` (по умолчанию, под опц. заголовком) или `replace`; остальные поля сохраняются |
| `tessera_move_task` | Перенести задачу в колонку по **имени** (`В процессе`, `На рассмотрении`, …) или UUID; done-колонка доски авто-завершает задачу |
| `tessera_assign_task` | Назначить исполнителей по email / имени / UUID / `author` / `me`; `replace=true` возвращает задачу (напр. `['author']` вернёт её создателю и снимет агента) |
| `tessera_create_task` | Создать задачу (или подзадачу через `parent_number`/`parent_id`) с описанием, приоритетом, датами, оценкой, исполнителями и тегами; колонка по умолчанию — крайняя левая |
| `tessera_create_subtasks` | Создать много подзадач под одним родителем за один вызов; сбой элемента не откатывает остальные (`created` / `failed`) |
| `tessera_update_task` | Частичная правка title / description / priority / дат / estimate / completed; `clear: ["due_date"]` очищает дату или оценку |
| `tessera_set_parent` | Сделать задачу подзадачей другой или `detach: true` — вернуть её на верхний уровень |
| `tessera_move_description` | Перенести (или скопировать) описание между задачами; `cut: true` очищает источник **после** записи в цель |
| `tessera_set_tags` | Добавить/убрать теги по имени в рамках проекта задачи; неизвестные имена дают ошибку, если не `create_missing: true` |
| `tessera_link_tasks` / `tessera_unlink_tasks` | Связать две задачи (`relates` \| `blocks` \| `blocked_by` \| `duplicates`) по `#number`; unlink убирает связь с **обоих** концов |
| `tessera_upload_attachment` / `tessera_download_attachment` | Приложить локальные файлы к задаче (≤ 25 МиБ каждый) / скачать вложения задачи в локальный каталог |

Задачи адресуются по `task_id` (id подзадачи тоже подходит) или по `workspace_id` +
`number` (например, `#252`). Инструменты записи действуют от имени владельца токена —
выпускайте токен для **выделенного агент-пользователя**, чтобы его комментарии
отличались от ваших (`is_agent` в комментариях `get_task` завязан именно на это).

**Ранжирование** (в `internal/rank`): сначала незавершённые → сначала просроченные →
выше приоритет (4=срочный … 0=нет) → ближе срок → позиция на доске. Архивные задачи
исключаются; завершённые исключаются, если не задан `include_completed: true`.

## Настройка

### 1. Выпустить Personal Access Token

Headless-клиенты аутентифицируются **Personal Access Token** (`tsra_…`) Tessera, а не
браузерной сессией. Выпустите его локально против своей БД:

```bash
cd backend
go run ./cmd/token -email you@example.com -name mcp        # без срока
go run ./cmd/token -email you@example.com -name ci -days 90 # истекает через 90 дней
```

Открытый текст токена печатается **один раз** — сохраните сразу. Токены отзываются
через `DELETE /api/auth/tokens/:id` (список — `GET /api/auth/tokens`).

### 2. Сборка

```bash
make build-mcp        # → mcp/tessera-mcp
```

### 3. Регистрация в Claude Code

```bash
claude mcp add tessera \
  --env TESSERA_BASE_URL=http://localhost:8090/api \
  --env TESSERA_TOKEN=tsra_xxxxxxxxxxxxxxxx \
  -- /absolute/path/to/tessera/mcp/tessera-mcp
```

или добавьте в `.mcp.json`:

```json
{
  "mcpServers": {
    "tessera": {
      "command": "/absolute/path/to/tessera/mcp/tessera-mcp",
      "env": {
        "TESSERA_BASE_URL": "http://localhost:8090/api",
        "TESSERA_TOKEN": "tsra_xxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

## Конфигурация

| Переменная окружения | По умолчанию | |
|----------------------|--------------|-|
| `TESSERA_BASE_URL` | `http://localhost:8090/api` | Базовый URL API |
| `TESSERA_TOKEN` | — | Personal access token (`tsra_…`), **обязателен** |

## Область применения

Чтение **и** запись: агент может забрать ранжированную очередь, прочитать задачу
(включая скриншоты через `tessera_view_image`), взять её в работу, комментировать
(прикладывая доказательства тестов), отправить на проверку и задать уточняющие
вопросы — полный агентный цикл. `image_paths` в `tessera_add_comment` читаются из
**собственной файловой системы MCP-сервера**, поэтому скриншоты должны быть доступны
этому процессу (нормально, когда сервер работает на том же хосте, что и агент).
