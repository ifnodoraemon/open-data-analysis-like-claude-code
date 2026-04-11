# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Open Data Analysis — an interactive, agent-driven data analysis platform for tabular files. Users upload CSV/Excel data, an LLM agent inspects and queries it via tool calls, and generates reports with interactive charts. The agent runtime follows a self-directed ReAct loop (not a fixed DAG).

## Quick Start (Docker Compose only)

```bash
cp server/.env.example server/.env   # then configure LLM_PROVIDER / LLM_API_KEY / LLM_MODEL
docker compose up -d --build
# Visit http://localhost
```

## Build & Test Commands

### Go Backend (server/)

```bash
cd server
go build ./...                          # compile
go test ./...                           # all tests
go test ./agent/...                     # single package
go test -run TestEngineSingle ./agent/  # single test
go vet ./...                            # static analysis
```

### Vue Frontend (client/)

```bash
cd client
npm install
npm run dev                  # dev server on :5173, proxies /api and /ws to :8080
npm run build                # production build
npm run test                 # vitest
npm run format               # prettier write
npm run format:check         # prettier check
```

### Python Executor (python-executor/)

```bash
cd python-executor
python -m pytest test_sandbox.py       # sandbox tests
python main.py                         # start on :8081
```

### Docker

```bash
docker compose up -d --build --force-recreate   # full rebuild
docker compose logs -f server                   # backend logs
docker compose logs -f python-executor          # executor logs
docker compose down                             # stop
```

## Architecture

### Three-Service Model

| Service | Tech | Port | Role |
|---------|------|------|------|
| **server** | Go (Chi + Gorilla WS) | 8080 | REST API, WebSocket, agent engine, tool execution |
| **client** | Vue 3 (Vite + Pinia) | 80 | SPA, nginx reverse-proxy to server |
| **python-executor** | FastAPI | 8081 | Sandboxed Python execution (AST + process isolation) |

### Request Flow

1. Client connects via WebSocket (`/ws?token=...&session_id=...`)
2. User sends message through `InputBar` -> handler (`handler/ws.go`) starts a run
3. Agent engine (`agent/engine.go`) runs ReAct loop: call LLM -> execute tool calls -> repeat until `stop_run`
4. Each step streams as a WebSocket event to the client in real time
5. Events are asynchronously persisted to SQLite via `eventPersistQueue`

### Key Backend Packages

- **`agent/`** — Engine (ReAct loop), LLM client (OpenAI + Anthropic), tool worker, memory/goals
- **`handler/`** — HTTP handlers (auth, upload, bootstrap, sessions, runs) + WebSocket
- **`tools/`** — Tool registry + implementations (SQL, charts, reports, Python exec, goals/memory)
- **`data/`** — CSV/Excel ingestion into per-session SQLite, schema inference
- **`repository/sqlite/`** — Data access layer for metadata SQLite
- **`session/`** — Per-session analysis DB manager (TTL-based cleanup)
- **`storage/local/`** — File storage abstraction (local FS, S3 migration-ready)
- **`auth/`** — JWT tokens, bcrypt passwords, workspace identity

### Frontend Structure

- **`stores/agent.js`** (Pinia) — Global state: user, sessions, runs, files, report, memory, goals
- **`composables/useWebSocket.js`** — WebSocket lifecycle, reconnection, event dispatch to store
- **`components/agent/`** — AgentPanel (chat), SubgoalTree, WorkingMemoryPanel
- **`components/report/`** — Live report preview (resizable split pane)

### Data Layers

1. **Metadata SQLite** (`data/metadata/app.db`) — users, workspaces, sessions, files, runs, reports, messages
2. **Session analysis SQLite** (`data/cache/{session_id}.db`) — per-session scratch DB for imported data and SQL analysis
3. **File storage** — `data/storage/workspaces/{workspace_id}/files/{file_id}/source/{filename}`

### Python Executor Sandbox

Defense in depth: AST static analysis (import whitelist, attribute blocklist) -> process isolation (multiprocessing) -> resource limits (120s timeout, 512MB memory, 0 subprocess).

## Agentic Design

The agent runtime is self-directed. Key constraints from `AGENTS.md`:

- Runtime provides tools, state, and guardrails; the model decides the path
- No hardcoded step order, no implicit phase transitions, no `next_action` advice from tools
- Tool descriptions are factual and contract-oriented, not prescriptive
- Goals and report blocks are optional scaffolds, not mandatory
- State inspection tools (`state_*`) expose facts only
- Four-layer prompt model: Policy (system) -> Task (user) -> Runtime Context -> History

## Configuration

All config via `server/.env` (see `server/.env.example`). Key variables:

- `LLM_PROVIDER` — `openai` or `anthropic`
- `LLM_API_KEY`, `LLM_MODEL`, `LLM_BASE_URL`
- `AUTH_SECRET` — JWT signing secret
- `PROXY_TOKEN` — python-executor auth token
- `PYTHON_MCP_URL` — executor endpoint (default `http://python-executor:8081`)

LLM debug traces: set `LLM_DEBUG=true` -> traces written to `data/llm-debug/YYYY-MM-DD/`.

## Conventions

- Go module: `github.com/ifnodoraemon/openDataAnalysis`
- Go tests: standard `testing` package, table-driven style
- Frontend tests: Vitest
- Auth: JWT (7-day TTL), bcrypt passwords, workspace-scoped
- One active run per session at a time
- CSV streaming with no row limit; Excel hard-capped at 100K rows/sheet
