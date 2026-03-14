# Open Data Analysis / 数据分析智能体

Interactive, Claude-Code-style data analysis for tabular files. Upload CSV or Excel data, let the agent inspect and query it, then generate a report with interactive charts.

面向表格数据的交互式智能分析工具，交互方式类似 Claude Code。上传 CSV 或 Excel 文件后，Agent 会自主检查、查询、分析数据，并生成带交互图表的研报。

![数据分析智能体界面](docs/images/screenshot.png)

## Highlights / 功能概览

- Agent runtime with tool calling and self-directed planning, not a fixed DAG
- Workspace-aware auth, sessions, runs, and file ownership
- Real-time WebSocket execution stream plus resumable run/report state
- Local object storage abstraction with clean migration path to S3-compatible backends
- Vue 3 frontend, Go backend, SQLite metadata + SQLite analysis scratch DB

- Agent 自主决定工具调用顺序，不是硬编码 DAG
- 已支持工作区、会话、任务、文件归属和鉴权
- 通过 WebSocket 实时推送执行过程，并支持恢复最近一次 run/report
- 已抽象对象存储接口，当前默认本地存储，后续可切 S3 兼容实现
- 前端使用 Vue 3，后端使用 Go，当前运行时采用 SQLite 元数据库 + SQLite 分析工作库

## Current Architecture / 当前架构

### Runtime data layers / 运行时数据分层

1. Metadata SQLite
   Stores users, workspaces, memberships, sessions, files, and analysis runs.

1. 元数据 SQLite
   存储用户、工作区、成员关系、会话、文件元数据和分析任务。

2. Session analysis SQLite
   Each session gets its own scratch database for imported CSV/Excel data and SQL analysis.

2. 会话分析 SQLite
   每个 session 都有自己的分析工作库，用于导入 CSV/Excel 并执行 SQL 查询。

PostgreSQL is not enabled in runtime yet. The repository keeps the domain boundaries and schema direction ready for a future migration, but the current product uses SQLite in production code paths.

目前运行时还没有接入 PostgreSQL。仓库已经保留了未来迁移所需的领域边界和 schema 方向，但当前产品代码路径仍然使用 SQLite。

### Storage / 存储

The application does not bind business logic to MinIO. Files are addressed by `file_id`, and the backend resolves them through the storage abstraction.

应用没有把业务逻辑绑定到 MinIO。文件对外只暴露 `file_id`，后端通过存储抽象解析实际对象位置。

Current default implementation:

当前默认实现：

- Provider: local filesystem
- Upload object key: `workspaces/{workspace_id}/files/{file_id}/source/{filename}`
- Report object key: `workspaces/{workspace_id}/runs/{run_id}/report/report.html`

- Provider：本地文件系统
- 上传文件对象 key：`workspaces/{workspace_id}/files/{file_id}/source/{filename}`
- 报告对象 key：`workspaces/{workspace_id}/runs/{run_id}/report/report.html`

### Local Docker debugging / 本地 Docker 调试

When started with `docker compose`, the server enables `LLM_DEBUG=true` by default and writes model request/response traces to a separate trace directory:

通过 `docker compose` 启动本地环境时，服务端默认开启 `LLM_DEBUG=true`，并将模型输入输出写入独立的 trace 调试目录：

- Path / 路径: `data/llm-debug/`
- Format / 格式: `YYYY-MM-DD/<trace_id>/request.json + response.json + index.jsonl`
- Separation / 隔离: kept outside normal app stdout logs / 不与程序标准日志混写

## Tech Stack / 技术栈

| Layer | Stack |
|---|---|
| Frontend | Vue 3, Vite, Pinia |
| Backend | Go, Chi, Gorilla WebSocket |
| Agent | Tool-calling ReAct loop |
| Data ingestion | CSV / Excel -> SQLite |
| Charts | ECharts 5 |
| Storage | Local object storage abstraction |
| Deployment | Docker, Docker Compose |

## Agentic Direction / Agentic 方向

The project treats the backend as an agent runtime, not a hidden workflow engine. The system should expose goals, tools, state, and thin guardrails; the model should judge what to do next.

这个项目把后端视为 agent runtime，而不是隐藏的 workflow engine。系统负责暴露目标、工具、状态和薄 guardrail；下一步做什么由模型自己裁决。

Reference:

- `docs/agentic-principles.md`

Non-goals:

- hardcoded step order
- implicit phase transitions
- runtime-written advice that tells the model how to act

非目标：

- 写死的步骤顺序
- 隐式阶段切换
- 由 runtime 替模型写行动建议

Available core tools:

当前核心工具：

- `data_load_file`
- `data_list_tables`
- `data_describe_table`
- `data_query_sql`
- `report_create_chart`
- `report_manage_blocks`
- `report_configure_layout`
- `report_finalize`
- `code_run_python`
- `memory_save_fact`
- `state_memory_inspect`
- `state_goal_inspect`
- `state_report_inspect`
- `goal_manage`
- `task_delegate`
- `user_request_input`

Notes:

- `goal_manage` is optional scratchpad state, not a required planning phase
- state inspect tools expose facts only; the model decides how to use them
- durable project guidance lives in `AGENTS.md`, not in the runtime prompt

## Authentication / 鉴权

The backend now runs in authenticated mode. Except for `/api/auth/login` and `/api/health`, APIs require a valid token.

后端当前运行在鉴权模式下。除 `/api/auth/login` 和 `/api/health` 外，其余接口都要求携带有效 token。

Default admin credentials are configured through environment variables, not hardcoded in business logic.

默认管理员账号通过环境变量配置，不写死在业务逻辑中。

Example defaults in `server/.env.example`:

`server/.env.example` 中的默认示例：

```env
DEFAULT_USER_ID=admin
DEFAULT_USER_EMAIL=admin
DEFAULT_USER_NAME=Administrator
DEFAULT_USER_PASSWORD=admin@123
DEFAULT_WORKSPACE_ID=default
DEFAULT_WORKSPACE_NAME=Default Workspace
AUTH_SECRET=change-me
```

## Quick Start / 快速开始

### Docker Compose Only / 本地只支持 Docker Compose

```bash
# 1. Prepare env
cp server/.env.example server/.env

# 2. Fill in your LLM settings
#    配置 LLM_PROVIDER / LLM_API_KEY / LLM_MODEL 等参数

# 3. Start all services
docker compose up -d --build

# 4. Open
#    浏览器访问 http://localhost
```

Local setup is intentionally standardized on `docker compose`. The repository does not treat `go run main.go` or `npm run dev` as the primary supported path anymore.

本地调试统一只走 `docker compose`。仓库不再把 `go run main.go` 或 `npm run dev` 视为主支持路径。

### Rebuild And Logs / 重建与日志

```bash
# Rebuild all services from scratch
docker compose up -d --build --force-recreate

# Follow backend logs
docker compose logs -f server

# Follow frontend logs
docker compose logs -f client

# Follow python executor logs
docker compose logs -f python-executor

# Stop all services
docker compose down
```

### Runtime Directories / 运行期目录

All runtime data stays under the mounted `data/` directory inside Docker:

Docker 模式下，运行产物统一收敛到挂载的 `data/` 目录：

- `data/metadata/`: metadata SQLite
- `data/cache/`: per-session analysis SQLite scratch files
- `data/storage/`: uploaded source files and generated report objects
- `data/tmp/`: materialized temp files
- `data/llm-debug/`: LLM request/response traces

LLM debug traces are organized by date and trace ID:

LLM 调试日志按日期和 trace ID 落盘：

- `data/llm-debug/YYYY-MM-DD/index.jsonl`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/request.json`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/response.json` or `response.error.txt`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/index.jsonl`

## Main API Surface / 主要接口

Authenticated endpoints currently include:

当前受保护接口包括：

- `POST /api/auth/switch-workspace`
- `GET /api/bootstrap`
- `GET /api/sessions`
- `GET /api/sessions/{sessionID}`
- `GET /api/runs`
- `GET /api/runs/{runID}`
- `GET /api/runs/{runID}/report`
- `POST /api/upload?session_id=...`
- `GET /ws?token=...&session_id=...`

## UI Behavior / 界面行为

- Workspace switch issues a new token and reconnects the WebSocket
- Recent sessions and runs are restored on bootstrap
- Final report HTML can be reopened after refresh
- Uploaded source files remain session-scoped and are not mixed with generated report artifacts

- 切换工作区时会重新签发 token 并重连 WebSocket
- 启动后会恢复最近的 session 和 runs
- 刷新页面后仍可重新打开最终报告 HTML
- 上传的源文件保持 session 作用域，不会和生成的报告产物混在一起

## Project Structure / 项目结构

```text
.
├── client/
│   ├── src/
│   │   ├── components/
│   │   ├── composables/
│   │   └── stores/
│   ├── Dockerfile
│   └── nginx.conf
├── server/
│   ├── agent/
│   ├── auth/
│   ├── config/
│   ├── data/
│   ├── domain/
│   ├── handler/
│   ├── metadata/
│   ├── migrations/
│   ├── repository/
│   ├── service/
│   ├── session/
│   ├── storage/
│   ├── tools/
│   ├── Dockerfile
│   └── main.go
├── data/
├── docker-compose.yml
└── README.md
```

## Product Direction / 产品方向

This repository is being built as a new product, so the priority is reducing future technical debt rather than preserving backward compatibility. Current implementation choices favor explicit boundaries:

这个仓库对应的是新产品，因此当前优先级是减少未来技术债，而不是保留历史兼容逻辑。当前实现重点收敛在这些边界：

- auth and workspace ownership
- session and run lifecycle
- file identity and storage abstraction
- report persistence and recovery

- 认证与工作区归属
- 会话与任务生命周期
- 文件身份与存储抽象
- 报告持久化与恢复

## License / 许可证

Apache License 2.0 with Commons Clause — see [LICENSE](LICENSE) for details.

This means you can freely use, modify, and distribute the code, but you may not sell it as a commercial product or service.

本项目采用 Apache License 2.0 + Commons Clause 许可证，详见 [LICENSE](LICENSE)。

你可以自由使用、修改和分发代码，但不得将其作为商业产品或服务进行销售。
