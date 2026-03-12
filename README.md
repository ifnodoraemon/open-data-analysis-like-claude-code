# Open Data Analysis / 数据分析智能体

Interactive, Claude-Code-style data analysis for tabular files. Upload CSV or Excel data, let the agent inspect and query it, then generate a report with interactive charts.

面向表格数据的交互式智能分析工具，交互方式类似 Claude Code。上传 CSV 或 Excel 文件后，Agent 会自主检查、查询、分析数据，并生成带交互图表的研报。

## Highlights / 功能概览

- Agent-driven workflow with tool calling, not a fixed DAG
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

## Agent Workflow / Agent 工作流

The agent is dynamic. It observes tool results, decides the next step, and stops when it has enough evidence to answer or finalize a report.

Agent 是动态决策的。它会观察工具结果，自主决定下一步要做什么，在证据充分时结束分析或生成最终报告。

Available core tools:

当前核心工具：

- `load_data`
- `list_tables`
- `describe_data`
- `query_data`
- `create_chart`
- `write_section`
- `finalize_report`
- `run_python`

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

### Local development / 本地开发

```bash
# 1. Prepare backend env
cp server/.env.example server/.env

# 2. Fill in your LLM settings
#    配置 LLM_PROVIDER / LLM_API_KEY / LLM_MODEL 等参数

# 3. Start backend
cd server
go run main.go

# 4. Start frontend in another terminal
cd client
npm install
npm run dev

# 5. Open
#    浏览器访问 http://localhost:5173
```

### Docker / Docker 部署

```bash
cp server/.env.example server/.env
docker compose up -d --build
```

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

MIT
