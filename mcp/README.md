# Tessera MCP server

A [Model Context Protocol](https://modelcontextprotocol.io) server that exposes a
Tessera workspace to an AI agent (e.g. Claude Code) as a **priority-ranked queue
of tasks** — for automated, agentic development driven straight from Tessera.

It is a thin **REST client** of the Tessera API (it never touches the database)
and speaks MCP over **stdio**, so the MCP client launches it as a subprocess.

## Tools

### Read

| Tool | What it does |
|------|--------------|
| `tessera_list_workspaces` | Workspaces the account belongs to |
| `tessera_list_projects` | Projects in a workspace |
| `tessera_list_boards` | Boards in a project |
| `tessera_list_columns` | A board's columns (id, name, position, done flag) |
| `tessera_resolve_board` | `/project/<slug>/board/<slug>` → board |
| `tessera_list_tasks` | A board's tasks as a **ranked** queue (optionally scoped to a milestone) |
| `tessera_my_tasks` | Tasks assigned to you across a workspace, ranked |
| `tessera_next_task` | The single highest-priority actionable task (skips blocked ones on a board) |
| `tessera_get_task` | Full task detail: description, tags, assignees, subtasks, comments (with author + `is_agent`), image refs, GitLab link |
| `tessera_view_image` | Fetch & return image(s) referenced by a task (inline images + attachments) so the agent can actually see screenshots/diagrams |

### Write

| Tool | What it does |
|------|--------------|
| `tessera_add_comment` | Post a Markdown comment on a task/subtask; optional `image_paths` upload & embed local screenshots inline |
| `tessera_update_description` | Attach Markdown to the description — `append` (default, under an optional heading) or `replace`; other fields preserved |
| `tessera_move_task` | Move a task to a column by **name** (`В процессе`, `На рассмотрении`, …) or UUID; the board's done column auto-completes |
| `tessera_assign_task` | Set assignees by email / name / UUID / `author` / `me`; `replace=true` hands a task back (e.g. `['author']` returns it to its creator and drops the agent) |

Tasks are addressed by `task_id` (a subtask id works too) or by `workspace_id` +
`number` (e.g. `#252`). Write tools author as the token's owner — mint the token
for a **dedicated agent user** so its comments are distinguishable from yours
(`is_agent` on `get_task` comments keys off this).

**Ranking** (in `internal/rank`): incomplete first → overdue first → higher
priority (4=urgent … 0=none) → earlier due date → board position. Archived tasks
are excluded; completed tasks are excluded unless `include_completed: true`.

## Setup

### 1. Mint a Personal Access Token

Headless clients authenticate with a Tessera **Personal Access Token** (`tsra_…`),
not a browser session. Mint one locally against your database:

```bash
cd backend
go run ./cmd/token -email you@example.com -name mcp        # never expires
go run ./cmd/token -email you@example.com -name ci -days 90 # expires in 90 days
```

The plaintext token is printed **once** — store it now. Tokens are revocable via
`DELETE /api/auth/tokens/:id` (or listed with `GET /api/auth/tokens`).

### 2. Build

```bash
make build-mcp        # → mcp/tessera-mcp
```

### 3. Register with Claude Code

```bash
claude mcp add tessera \
  --env TESSERA_BASE_URL=http://localhost:8090/api \
  --env TESSERA_TOKEN=tsra_xxxxxxxxxxxxxxxx \
  -- /absolute/path/to/tessera/mcp/tessera-mcp
```

or add it to `.mcp.json`:

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

## Configuration

| Env var | Default | |
|---------|---------|-|
| `TESSERA_BASE_URL` | `http://localhost:8090/api` | API base URL |
| `TESSERA_TOKEN` | — | Personal access token (`tsra_…`), **required** |

## Scope

Read **and** write: the agent can pull a ranked queue, read a task (including
screenshots via `tessera_view_image`), take it into progress, comment (attaching
test evidence), send it for review, and ask clarifying questions — a full agentic
loop. `image_paths` on `tessera_add_comment` are read from the **MCP server's own
filesystem**, so screenshots must be reachable by that process (fine when the
server runs on the same host as the agent).
