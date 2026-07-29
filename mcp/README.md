# Tessera MCP server

A [Model Context Protocol](https://modelcontextprotocol.io) server that exposes a
Tessera workspace to an AI agent (e.g. Claude Code) as a **priority-ranked queue
of tasks** — for automated, agentic development driven straight from Tessera.

It is a thin, **read-only REST client** of the Tessera API (it never touches the
database) and speaks MCP over **stdio**, so the MCP client launches it as a
subprocess.

## Tools

| Tool | What it does |
|------|--------------|
| `tessera_list_workspaces` | Workspaces the account belongs to |
| `tessera_list_projects` | Projects in a workspace |
| `tessera_list_boards` | Boards in a project |
| `tessera_resolve_board` | `/project/<slug>/board/<slug>` → board |
| `tessera_list_tasks` | A board's tasks as a **ranked** queue (optionally scoped to a milestone) |
| `tessera_my_tasks` | Tasks assigned to you across a workspace, ranked |
| `tessera_next_task` | The single highest-priority actionable task (skips blocked ones on a board) |
| `tessera_get_task` | Full task detail: description, tags, assignees, subtasks, comments, GitLab link |

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

Read-only by design. Write-back tools (move a task to In Progress, comment,
mark done) for a full agentic loop are a planned Phase 2.
