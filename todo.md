# 当前 Todo

更新日期：2026-03-22

这份清单基于当前仓库实际状态重写，只保留"还没闭环"的事项，不再重复已经落地的能力。

本次盘点依据：

- 已检查核心实现：`server/agent`、`server/data`、`server/session`、`server/tools`
- 已检查产品与设计文档：`README.md`、`README.zh-CN.md`、`docs/agentic-principles.md`、`docs/benchmark.md`、`docs/database-source-mvp.md`
- 已检查回归资产：`samples/coverage_scenarios/`、`scripts/run_scenario.js`、`tmp/scenario-runs/`
- 已验证当前基本工程状态：
  - `GOCACHE=/tmp/go-build go test ./...` 通过
  - `npm run build` 通过
  - `.github/workflows/ci.yml` 已存在

## 已从旧清单移除或降级的事项

这些能力仓库已经具备基础实现，不应继续作为"从 0 开始建设"的 todo：

- Agent runtime 已有 `goal_manage`、`memory_save_fact`、`state_*_inspect`、`task_delegate`
- `user_request_input` 已可把 run 挂起为 `waiting_user_input`，并在同一 run 中恢复
- session 已限制同一时刻只有一个 active run
- CSV 导入已经是流式批量插入，不是全量读入内存
- Excel 已有 10 万行硬上限，并在超限时返回明确错误
- 导入后已经会做表统计与索引生成
- 对象存储已经抽象为 interface，当前默认本地实现
- coverage scenario 与最小 runner 已存在，不需要再写"先建 benchmark 目录/runner"
- 基础 CI 已存在，不需要再写"先把 CI 从 0 搭起来"

## P0 ✅ 全部完成

> 已完成并从清单移除：
> 1. 确定性元数据与 LLM 语义预分析解耦
> 2. user_request_input 结构化确认协议
> 3. evidence model
> 4. scenario 回归门禁
> 5. data_query_sql 大结果上下文压缩

## P1 ✅ 全部完成

> 已完成并从清单移除：
> 6. 补齐产品边界文档（中英文 README 添加 Product Boundaries 表）
> 7. 生命周期清理与留存策略（`cleanup.go` + `SESSION_TTL_HOURS` / `TRACE_RETENTION_DAYS` / `TEMP_CLEANUP_ON_START`）
> 8. 状态串扰回归测试（8 个新测试：expired session、trace 清理、temp 清理、快速开始/停止/重启、旧 run cancel 隔离）

## P2

### 9. 数据库数据源 MVP

现状：

- `docs/database-source-mvp.md` 已写得比较完整
- 代码里还没有 `data_source`、数据库连接仓储、快照导入 API 和前端入口

待做：

- 先只做 workspace 级只读 PostgreSQL snapshot import
- 新增领域对象与仓储：
  - `data_source`
  - `database_connection`
  - `session_source_binding`
  - `imported_snapshot`
- 增加连接测试、allowlist、浏览 schema/table、导入到 session SQLite 的最小链路
- 导入后走上面的 schema metadata + semantic profile 流程

完成标准：

- 用户可以保存一个只读 PostgreSQL 连接，并把指定表快照导入当前 session

### 10. 存储 provider 扩展

现状：

- 存储抽象已经有了，但当前只有 local provider

待做：

- 在当前文件身份模型不变的前提下补一个 S3-compatible provider
- 保持业务层只认 `file_id` / `storage_key`

完成标准：

- 切对象存储实现时不需要改业务协议和 agent tool 语义

### ~~11. Agent Harness 可靠性补齐~~ ✅ 已完成（2026-03-22）

> 子代理上下文压缩（`compactWorkerMessages`）、幂等工具重试（`retryableToolExec` 指数退避）、子代理 token 追踪（`UpdateChildRunTokens` 接口 + child_run_tokens 事件）、Evaluation 管道（`scripts/eval_report.js` + CI 步骤）、报告 HTML Guardrail（`html_sanitize.go` + `applyReportHTMLGuardrail`）均已落地并通过测试。

## 暂不进入近期开发

这些方向不是不做，而是不应排在当前缺口前面：

- PostgreSQL 元数据仓储迁移
- 独立 worker/queue 化执行
- MySQL 支持
- live query
- 过早微服务化
