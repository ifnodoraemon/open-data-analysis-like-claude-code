# 下一步实施清单

这份清单只记录“接下来要做的事情”，不重复已经完成的能力。优先级以当前新产品落地为准，默认原则：

- 不做历史兼容包袱
- 本地开发和演示一律走 Docker
- 优先减少后续技术债，而不是先堆功能

## 当前基线

当前系统已经具备这些基础边界：

- 账号、工作区、session、run、file 的基础模型已经建立
- 默认存储已经抽象到 `ObjectStorage`，当前实现为本地文件系统
- 运行时使用两套 SQLite
  - 元数据 SQLite：用户、工作区、会话、文件、任务
  - 分析 SQLite：每个 session 的导表与 SQL 查询工作库
- WebSocket 支持实时执行流
- run/report 支持最近状态恢复
- 默认管理员账号通过环境变量注入

当前还没有完成的关键部分，主要集中在：

- 本地 Docker 调试链路的收口
- LLM 调试留痕体系
- 测试体系
- 数据导入与存储演进
- 后端执行架构的进一步稳定化

## P0：当前这轮必须收口

### 1. 收口本地运行方式

- 统一 README、脚本、配置说明，明确本地只支持 `docker compose`
- 去掉文档里所有 `go run` / `npm run dev` 作为主路径的表述
- 清理与宿主机直启耦合的默认值，避免出现“本地能跑但 Docker 不一致”
- 补一个一键重建与日志查看说明，降低排障成本

验收标准：

- 新同事只按 README 的 Docker 步骤即可启动
- 不需要额外手工改代码或切换本地直启模式

### 2. 收口 LLM 调试日志

- 本地 Docker 默认开启 `LLM_DEBUG=true`
- 模型输入、输出、错误单独落盘，不与应用 stdout/stderr 混写
- 调试目录按日期和 trace 组织，至少包含：
  - `request.json`
  - `response.json` 或错误响应
  - `index.jsonl`
- 给每次 LLM 调用分配稳定 `trace_id`
- 后续把同一个 run 下的多次 LLM 调用串起来，形成最小 trace 链路

验收标准：

- 能从磁盘直接定位某次 run 的模型输入输出
- 程序日志仍然保持简洁，不被 prompt / response 污染

### 3. 收口登录后的 bootstrap / session 建立链路

- 确认登录成功后一定会执行 `bootstrap -> connect`
- 确认上传文件前 session 一定已经建立
- 避免前端状态变化导致初始化事件丢失
- 补一轮实际回归：登录、上传 Excel、分析、刷新恢复

验收标准：

- 不再出现“会话尚未建立，请稍后再上传”
- 刷新后仍能恢复最近 session 和报告

### 4. 收口 Python 工具可用性判断

- 本地 Docker 启动时默认启用 `python-executor`
- session 创建时按健康检查决定是否注册 `run_python`
- system prompt 根据工具可用性动态生成
- 工具不可用时，不让模型继续规划 `run_python`

验收标准：

- Docker 模式下 `run_python` 可正常使用
- 工具不可用时，模型不会反复错误调用

### 5. 清理运行产物与仓库边界

- 把运行期产物目录统一加入 `.gitignore`
- 区分代码目录、元数据目录、临时文件目录、调试目录
- 避免把本地调试数据误提交到仓库

验收标准：

- `git status` 只出现真正的代码改动

## P1：产品可用性增强

### 6. 历史记录体验补齐

- session 列表支持明确切换，不只自动恢复最近一次
- run 列表支持查看历史报告与基础摘要
- 增加“当前正在执行”和“当前查看的历史 run”的显式区分
- 补一个最小下载入口，至少能下载最终 HTML 报告

验收标准：

- 用户可以稳定回看历史分析
- 不会把历史 run 和当前实时 run 状态混在一起

### 7. LLM Trace 结构化

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

### 8. 前端状态机继续收紧

- 重新检查登录、bootstrap、reconnect、switch workspace、open session、open run 的状态切换
- 避免多个入口重复调用 `bootstrap/connect`
- 统一 `activeRunId`、`selectedRunId`、`sessionId` 的语义
- 补前端错误态和空态，减少“黑屏但其实有报错”的情况

验收标准：

- 状态切换清晰，没有重复初始化和错乱回放

### 9. 报告导出与恢复闭环

- 最终报告的下载、重新打开、权限校验走统一接口
- 补报告元数据而不是只存 HTML
- 为后续 PDF / DOCX 导出预留 report snapshot 结构

验收标准：

- 任何已完成 run 都可以稳定重新打开报告

## P1：后端稳定性

### 10. 运行链路串行化和边界收紧

- 明确一个 session 同时只能有一个 active run
- 进一步检查 `cancel/reset/new run` 的竞争条件
- 避免旧 run 的 emitter 污染新 run
- 明确 handler、session、engine 的状态归属

验收标准：

- 快速点击“开始/停止/重新开始”不会把状态搞乱

### 11. file / report 的仓储边界补齐

- 当前上传文件和报告文件已经分离，但还需要进一步明确类型
- 增加文件类型字段或用途字段，例如：
  - source
  - report
  - artifact
- 补基础清理策略，避免长期堆积

验收标准：

- 业务层可以明确区分源文件和生成产物

### 12. SQL 执行边界加固

- 强制单语句
- 强制只读
- 强制超时
- 强制返回行数限制
- 降低 LLM 生成异常 SQL 时的破坏面

验收标准：

- `query_data` 对错误或危险 SQL 有稳定保护

## P2：数据与存储演进

### 13. 大文件导入改造

- CSV 改流式导入，不再全量读入内存
- Excel 明确上限或改流式读取
- 导入过程记录元数据：
  - 表名
  - 行数
  - 来源文件
  - 导入时间

验收标准：

- 文件导入能力和产品定位一致，不再“文档说能处理大文件，代码其实全量读内存”

### 14. 存储 provider 继续抽象

- 保持业务层只认 `file_id`
- 保持元数据层只认 `provider + storage_key`
- 在本地实现稳定后，补 `S3-compatible` provider
- 不把 MinIO 当成领域前提

验收标准：

- 后续切 MinIO / S3 / R2 / GCS 时，不需要改前端协议和工具层

### 15. 生命周期清理

- session 过期清理
- 元数据 SQLite 清理策略
- 分析 SQLite 清理策略
- 调试日志清理策略
- 临时文件和中间产物清理

验收标准：

- 本地运行一段时间后，磁盘不会无边界增长

## P2：测试与工程质量

### 16. 自动化测试补齐

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

### 17. CI 基础能力

- `go test ./...`
- 前端构建
- Docker 构建
- 冒烟测试
- 后续再考虑更严格的 lint / e2e

验收标准：

- 主干分支改动不会靠手工回归兜底

## P3：中期架构演进

### 18. 元数据仓储保留 PostgreSQL 迁移接口

- 当前运行时继续用 SQLite
- repository interface 保持不变
- 等业务稳定后，再补 PostgreSQL repository 实现
- 迁移目标只放在元数据层，不影响分析 SQLite

验收标准：

- 迁移 PG 时不需要推翻现有领域模型和 handler/service 边界

### 19. 长任务拆到 worker

- 当前 run 仍在 WebSocket 进程内执行
- 后续把 run 调度和执行拆开
- API / WS 负责交互
- worker 负责分析执行
- Redis 或队列负责调度和取消信号

验收标准：

- 能支撑更长、更重、更并发的分析任务

## 建议执行顺序

### 第一批

- 收口 Docker-only 运行方式
- 收口 LLM 调试日志
- 修完登录后 bootstrap / session 建立链路
- 收口 `run_python` 动态启用
- 清理运行产物目录

### 第二批

- 历史记录体验补齐
- 前端状态机收紧
- 报告导出与恢复闭环
- 运行链路串行化与 SQL 保护

### 第三批

- 大文件导入改造
- 存储 provider 演进
- 生命周期清理
- 测试与 CI

### 第四批

- PostgreSQL repository 实现
- worker 化

## 不建议现在做的事

- 不要现在就把分析工作库迁到 PostgreSQL
- 不要为了“未来可扩展”提前引入过重的微服务拆分
- 不要在没有 trace 结构之前继续堆普通文本日志
- 不要把 MinIO 写死进业务层

