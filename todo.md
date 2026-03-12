# 下一步实施清单

这份清单只保留当前还没有完成的事项，不重复已经收口的能力。

默认原则：

- 不做历史兼容包袱
- 本地开发和演示一律走 Docker
- 优先减少后续技术债，而不是先堆功能

## 当前重点

当前主线已经从 P0 收口转到 P1/P2，优先做三类事情：

- Agent 上下文组织与长任务稳定性
- benchmark、测试、回归能力
- 数据导入、生命周期、执行架构的后续演进

## P1：Agent 与产品可用性

### 1. 前端状态机继续收紧

- 重新检查登录、bootstrap、reconnect、switch workspace、open session、open run 的状态切换
- 避免多个入口重复调用 `bootstrap/connect`
- 统一 `activeRunId`、`selectedRunId`、`sessionId` 的语义
- 补前端错误态和空态，减少“黑屏但其实有报错”的情况

验收标准：

- 状态切换清晰，没有重复初始化和错乱回放

### 2. 报告导出与恢复闭环继续完善

- 最终报告的下载、重新打开、权限校验继续走统一接口
- 补报告元数据，而不是只存 HTML
- 为后续 PDF / DOCX 导出预留 report snapshot 结构

验收标准：

- 任何已完成 run 都可以稳定重新打开报告

### 3. LLM Trace 结构继续标准化

- 从“调试文件”进一步收敛成统一结构：
  - `trace`
  - `span`
  - `event`
- 至少挂上这些关联字段：
  - `workspace_id`
  - `session_id`
  - `run_id`
  - `tool_call_id`
  - `trace_id`
- 为后续做 trace 查询 UI 和诊断能力打基础

验收标准：

- 一次 run 内的模型调用和工具调用可以串起来看

### 4. Agent 上下文按阶段拆分

- 把长任务从单一消息历史拆成至少两个阶段：
  - analysis phase
  - report phase
- 避免分析期的大查询结果、图表参数、章节正文持续污染后续上下文
- 明确阶段切换时保留什么摘要，丢弃什么中间细节

验收标准：

- 长任务后半程的 prompt 体积和响应延迟明显下降
- 报告阶段不再反复携带大段分析中间结果

## P1：后端稳定性

### 5. 运行链路串行化和边界收紧

- 明确一个 session 同时只能有一个 active run
- 进一步检查 `cancel/reset/new run` 的竞争条件
- 避免旧 run 的 emitter 污染新 run
- 明确 handler、session、engine 的状态归属

验收标准：

- 快速点击“开始/停止/重新开始”不会把状态搞乱

### 6. 生命周期清理

- session 过期清理
- 元数据 SQLite 清理策略
- 分析 SQLite 清理策略
- 调试日志清理策略
- 临时文件和中间产物清理

验收标准：

- 本地运行一段时间后，磁盘不会无边界增长

## P2：数据与存储演进

### 7. 大文件导入改造

- CSV 改流式导入，不再全量读入内存
- Excel 明确上限或改流式读取
- 导入过程记录元数据：
  - 表名
  - 行数
  - 来源文件
  - 导入时间

验收标准：

- 文件导入能力和产品定位一致，不再“文档说能处理大文件，代码其实全量读内存”

### 8. 存储 provider 继续抽象

- 保持业务层只认 `file_id`
- 保持元数据层只认 `provider + storage_key`
- 在本地实现稳定后，补 `S3-compatible` provider
- 不把 MinIO 当成领域前提

验收标准：

- 后续切 MinIO / S3 / R2 / GCS 时，不需要改前端协议和工具层

## P2：benchmark、测试与工程质量

### 9. Benchmark 基线

- 建内部 benchmark 分层，至少覆盖：
  - SQL
  - Spreadsheet
  - Python
  - 图表
  - 报告
  - Agent 编排
- 建最小 case 目录结构和评分规则
- 支持把 benchmark 跑进本地 Docker 回归链路

验收标准：

- 每次核心 agent 改动后，都能看到任务成功率、错误恢复率、成本和耗时变化

### 10. 自动化测试补齐

- 后端仓储测试
- service 层单元测试
- upload / run / report 的接口测试
- Docker 端到端冒烟测试
- 至少覆盖：
  - 登录
  - bootstrap
  - 上传文件
  - 开始分析
  - 保存报告
  - 刷新恢复

验收标准：

- 每次改动后可以自动验证核心链路

### 11. CI 基础能力

- `go test ./...`
- 前端构建
- Docker 构建
- 冒烟测试
- 后续再考虑更严格的 lint / e2e

验收标准：

- 主干分支改动不会靠手工回归兜底

## P3：中期架构演进

### 12. 元数据仓储保留 PostgreSQL 迁移接口

- 当前运行时继续用 SQLite
- repository interface 保持不变
- 等业务稳定后，再补 PostgreSQL repository 实现
- 迁移目标只放在元数据层，不影响分析 SQLite

验收标准：

- 迁移 PG 时不需要推翻现有领域模型和 handler/service 边界

### 13. 长任务拆到 worker

- 当前 run 仍在 WebSocket 进程内执行
- 后续把 run 调度和执行拆开
- API / WS 负责交互
- worker 负责分析执行
- Redis 或队列负责调度和取消信号

验收标准：

- 能支撑更长、更重、更并发的分析任务

## 建议执行顺序

### 第一批

- 前端状态机收紧
- 运行链路串行化
- 生命周期清理
- benchmark 基线

### 第二批

- Agent 上下文按阶段拆分
- trace 结构标准化
- 自动化测试补齐
- CI 基础能力

### 第三批

- 大文件导入改造
- 存储 provider 演进
- worker 化准备

### 第四批

- PostgreSQL repository 实现
- worker 化落地

## 不建议现在做的事

- 不要现在就把分析工作库迁到 PostgreSQL
- 不要为了“未来可扩展”提前引入过重的微服务拆分
- 不要在 benchmark 和 trace 都没成形之前继续堆普通文本日志
- 不要把 MinIO 写死进业务层
