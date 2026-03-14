# 数据分析智能体

[English](README.md)

面向表格数据的交互式智能分析工具，交互方式类似 Claude Code。上传 CSV 或 Excel 文件后，Agent 会自主检查、查询、分析数据，并生成带交互图表的研报。

![数据分析智能体界面](docs/images/screenshot.png)

## 功能概览

- Agent 自主决定工具调用顺序，不是硬编码 DAG
- 已支持工作区、会话、任务、文件归属和鉴权
- 通过 WebSocket 实时推送执行过程，并支持恢复最近一次 run/report
- 已抽象对象存储接口，当前默认本地存储，后续可切 S3 兼容实现
- 前端使用 Vue 3，后端使用 Go，当前运行时采用 SQLite 元数据库 + SQLite 分析工作库

## 当前架构

### 运行时数据分层

1. 元数据 SQLite  
   存储用户、工作区、成员关系、会话、文件元数据和分析任务。

2. 会话分析 SQLite  
   每个 session 都有自己的分析工作库，用于导入 CSV/Excel 并执行 SQL 查询。

目前运行时还没有接入 PostgreSQL。仓库已经保留了未来迁移所需的领域边界和 schema 方向，但当前产品代码路径仍然使用 SQLite。

### 存储

应用没有把业务逻辑绑定到 MinIO。文件对外只暴露 `file_id`，后端通过存储抽象解析实际对象位置。

当前默认实现：

- Provider：本地文件系统
- 上传文件对象 key：`workspaces/{workspace_id}/files/{file_id}/source/{filename}`
- 报告对象 key：`workspaces/{workspace_id}/runs/{run_id}/report/report.html`

### 本地 Docker 调试

通过 `docker compose` 启动本地环境时，服务端默认开启 `LLM_DEBUG=true`，并将模型输入输出写入独立的 trace 调试目录：

- 路径：`data/llm-debug/`
- 格式：`YYYY-MM-DD/<trace_id>/request.json + response.json + index.jsonl`
- 隔离：不与程序标准日志混写

## 技术栈

| 层 | 技术 |
|---|---|
| Frontend | Vue 3, Vite, Pinia |
| Backend | Go, Chi, Gorilla WebSocket |
| Agent | Tool-calling ReAct loop |
| Data ingestion | CSV / Excel -> SQLite |
| Charts | ECharts 5 |
| Storage | Local object storage abstraction |
| Deployment | Docker, Docker Compose |

## Agentic 方向

这个项目把后端视为 agent runtime，而不是隐藏的 workflow engine。系统负责暴露目标、工具、状态和薄 guardrail；下一步做什么由模型自己裁决。

参考：

- `docs/agentic-principles.md`

非目标：

- 写死的步骤顺序
- 隐式阶段切换
- 由 runtime 替模型写行动建议

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

说明：

- `goal_manage` 是可选 scratchpad 状态，不是强制规划阶段
- 各类 state inspect 工具只暴露事实，不替模型做判断
- 稳定项目约束放在 `AGENTS.md`，不放在 runtime prompt 里

## 鉴权

后端当前运行在鉴权模式下。除 `/api/auth/login` 和 `/api/health` 外，其余接口都要求携带有效 token。

默认管理员账号通过环境变量配置，不写死在业务逻辑中。

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

## 快速开始

### 本地只支持 Docker Compose

```bash
# 1. 准备环境变量
cp server/.env.example server/.env

# 2. 填写 LLM 配置
#    配置 LLM_PROVIDER / LLM_API_KEY / LLM_MODEL 等参数

# 3. 启动全部服务
docker compose up -d --build

# 4. 打开页面
#    浏览器访问 http://localhost
```

本地调试统一只走 `docker compose`。仓库不再把 `go run main.go` 或 `npm run dev` 视为主支持路径。

### 重建与日志

```bash
# 全量重建服务
docker compose up -d --build --force-recreate

# 查看后端日志
docker compose logs -f server

# 查看前端日志
docker compose logs -f client

# 查看 python executor 日志
docker compose logs -f python-executor

# 停止所有服务
docker compose down
```

### 运行期目录

Docker 模式下，运行产物统一收敛到挂载的 `data/` 目录：

- `data/metadata/`：metadata SQLite
- `data/cache/`：每个 session 的分析 SQLite 工作文件
- `data/storage/`：上传源文件和生成的报告对象
- `data/tmp/`：物化出来的临时文件
- `data/llm-debug/`：LLM 请求/响应 trace

LLM 调试日志按日期和 trace ID 落盘：

- `data/llm-debug/YYYY-MM-DD/index.jsonl`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/request.json`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/response.json` 或 `response.error.txt`
- `data/llm-debug/YYYY-MM-DD/<trace_id>/index.jsonl`

## 主要接口

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

## 界面行为

- 切换工作区时会重新签发 token 并重连 WebSocket
- 启动后会恢复最近的 session 和 runs
- 刷新页面后仍可重新打开最终报告 HTML
- 上传的源文件保持 session 作用域，不会和生成的报告产物混在一起

## 项目结构

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
├── README.md
└── README.zh-CN.md
```

## 产品方向

这个仓库对应的是新产品，因此当前优先级是减少未来技术债，而不是保留历史兼容逻辑。当前实现重点收敛在这些边界：

- 认证与工作区归属
- 会话与任务生命周期
- 文件身份与存储抽象
- 报告持久化与恢复

## 许可证

MIT，详见 [LICENSE](LICENSE)。
