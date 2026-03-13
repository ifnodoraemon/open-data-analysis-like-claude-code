# 数据库连接与语义确认 MVP 设计

本文档定义当前仓库的数据库连接 MVP 方案，以及当上传文件或导入表结构语义不清时的 `AI 小样本预分析 + 人工确认` 流程。

日期：2026-03-13

## 1. 目标

当前系统已经具备：

- 工作区、session、run、file 基础模型
- 上传 CSV / Excel 到对象存储
- 导入到 session analysis SQLite
- 基于工具调用的 SQL / Python 分析和报告生成

但还缺两块关键能力：

1. 把数据库连接做成一等数据源
2. 当表名、列名、口径、join 关系不明确时，不让模型直接硬猜

本 MVP 的目标是：

- 支持工作区级只读 PostgreSQL 连接
- 支持从数据库中选择表/视图导入当前 session analysis SQLite
- 在上传或导入完成后，自动做一次小样本语义预分析
- 对低置信度语义和关系进入人工确认，再继续分析

## 2. 为什么先做 snapshot import

当前架构里，分析执行中心仍然是每个 session 的 SQLite 工作库，见：

- [README.md](/home/ifnodoraemon/myagent/data-analysis/README.md#L28)
- [server/session/types.go](/home/ifnodoraemon/myagent/data-analysis/server/session/types.go#L18)

因此数据库连接第一阶段不直接做 live query，原因如下：

- 更符合当前 session analysis SQLite 的执行架构
- 更利于报告复现，因为分析基于固定快照
- 更利于 trace 审计，因为每次分析绑定导入时刻
- 更容易控制权限、超时、限行数和资源消耗
- 更适合当前 benchmark 和回归体系

结论：

- 第一阶段只做 `snapshot import`
- `live query` 单独作为后续能力设计

## 3. 领域模型

当前 `file` 代表的是对象存储中的物理文件，不适合直接承载数据库连接。

参考当前实现：

- [server/domain/file.go](/home/ifnodoraemon/myagent/data-analysis/server/domain/file.go#L1)
- [server/service/file_service.go](/home/ifnodoraemon/myagent/data-analysis/server/service/file_service.go#L20)

建议新增以下领域实体：

### 3.1 `DataSource`

表示一个工作区可用的数据源。

字段建议：

- `id`
- `workspace_id`
- `name`
- `source_type`
  - `file_upload`
  - `database_connection`
  - `object_storage_file`
- `status`
  - `active`
  - `invalid`
  - `disabled`
- `created_by`
- `created_at`
- `updated_at`

### 3.2 `DatabaseConnection`

表示数据库连接的可管理配置。

字段建议：

- `source_id`
- `driver`
  - `postgres`
  - `mysql`
- `host`
- `port`
- `database_name`
- `default_schema`
- `ssl_mode`
- `auth_mode`
  - `password`
  - `url_secret_ref`
- `username`
- `secret_ref`
- `allowlist_json`
- `last_tested_at`
- `last_test_status`
- `last_error_message`

### 3.3 `SessionSourceBinding`

表示某个 session 当前使用了哪些数据源。

字段建议：

- `session_id`
- `source_id`
- `binding_type`
  - `uploaded`
  - `imported_snapshot`
- `snapshot_id`
- `created_at`

### 3.4 `ImportedSnapshot`

表示从外部数据库导入到当前 session analysis SQLite 的一次快照。

字段建议：

- `id`
- `session_id`
- `source_id`
- `upstream_driver`
- `upstream_schema`
- `upstream_table`
- `import_mode`
  - `full`
  - `sample`
- `row_count`
- `column_count`
- `imported_at`
- `analysis_table_name`
- `freshness_hint`

### 3.5 `SemanticProfile`

表示数据源导入后的小样本语义预分析结果。

字段建议：

- `id`
- `session_id`
- `source_id`
- `analysis_table_name`
- `profile_status`
  - `draft`
  - `confirmed`
  - `rejected`
- `column_roles_json`
- `metric_candidates_json`
- `join_candidates_json`
- `warnings_json`
- `confidence_score`
- `created_at`
- `updated_at`

### 3.6 `SemanticConfirmation`

表示用户对 AI 预分析结果做出的确认或修正。

字段建议：

- `id`
- `semantic_profile_id`
- `workspace_id`
- `session_id`
- `confirmed_by`
- `scope`
  - `session`
  - `workspace`
- `overrides_json`
- `created_at`

## 4. 查询与分析路径

### 4.1 上传文件路径

1. 用户上传 CSV / Excel
2. 文件进入对象存储并写入 `file`
3. 用户执行导入
4. 数据进入 session analysis SQLite
5. 系统抽取列统计和少量样本
6. AI 生成 `SemanticProfile`
7. 若高置信度则自动进入可分析状态
8. 若低置信度则提示用户确认

### 4.2 数据库导入路径

1. 用户在工作区新增 PostgreSQL 连接
2. 系统测试连接
3. 用户选择 allowlist 内的 schema/table
4. 系统把表/视图导入 session analysis SQLite
5. 系统抽取列统计和少量样本
6. AI 生成 `SemanticProfile`
7. 若存在歧义则等待人工确认
8. 确认完成后再允许 agent 做多表关联和报告生成

## 5. AI 小样本预分析 + 人工确认

这是必须补的能力，而且应该成为默认路径。

### 5.1 适用场景

- 表名不规范，如 `Sheet1`、`data_final_v3`
- 列名语义弱，如 `id`、`name`、`value1`、`amt`
- 多表中有多个可能 join key
- 指标列和维度列难以仅靠类型判断
- 时间列存在多个候选
- 中英文混合列名、缩写、业务黑话较多

### 5.2 为什么不能直接让模型全量猜

- 会把低质量列名误当成可靠语义
- 会在多表场景下静默选错 join key
- 会把业务指标口径猜成确定事实
- 后续报告看起来完整，但证据基础是错的

### 5.3 预分析输入

AI 预分析只使用轻量输入：

- 表名
- 列名
- 推断类型
- 非空率
- 近似唯一值数量
- 少量样本值
- 多表之间的样本重合度

默认不要在这个阶段把整表都喂给模型。

### 5.4 预分析输出

系统应生成以下候选结果：

- 列角色：
  - 时间列
  - 主键候选
  - 外键候选
  - 维度列
  - 指标列
  - 金额列
  - 比例列
- 指标口径候选：
  - GMV
  - revenue
  - cost
  - orders
  - users
- join 候选：
  - 左表字段
  - 右表字段
  - 置信度
  - 风险提示
- 风险告警：
  - 多个时间列
  - 多个金额列
  - 主键不唯一
  - join 候选冲突

### 5.5 何时必须人工介入

命中任一条件即进入确认态：

- join 候选超过 1 个且置信度接近
- 指标口径存在多个合理解释
- 关键列置信度低于阈值
- 主键候选不唯一
- 时间列超过 1 个且影响分析结论
- AI 检测到样本值与列名语义明显冲突

### 5.6 用户确认项

用户至少可以确认或修正：

- 某列是不是时间列
- 某列是不是金额/比例/类别
- 哪两个字段用于 join
- 指标列的业务名称
- 该确认只用于本次 session，还是复用到工作区

### 5.7 确认后的行为

- session 级确认立即生效
- workspace 级确认可在同类数据源下复用
- agent 后续分析优先使用确认结果
- 后续追问时，系统不再重复问同一问题

## 6. API 草案

以下是建议的最小 API 面：

### 6.1 数据源管理

- `POST /api/data-sources`
  - 创建工作区数据源
- `GET /api/data-sources`
  - 列出当前工作区数据源
- `GET /api/data-sources/{sourceID}`
  - 查看数据源详情
- `POST /api/data-sources/{sourceID}/test`
  - 测试数据库连接
- `GET /api/data-sources/{sourceID}/catalog`
  - 浏览 allowlist 内 schema/table

### 6.2 数据导入

- `POST /api/data-sources/{sourceID}/import`
  - 导入指定 schema/table 到当前 session
- `GET /api/sessions/{sessionID}/sources`
  - 查看当前 session 已绑定数据源
- `GET /api/sessions/{sessionID}/snapshots`
  - 查看当前 session 快照导入记录

### 6.3 语义确认

- `GET /api/sessions/{sessionID}/semantic-profiles`
  - 查看当前 session 的 AI 预分析结果
- `POST /api/semantic-profiles/{profileID}/confirm`
  - 提交用户确认或修正
- `GET /api/workspaces/{workspaceID}/semantic-overrides`
  - 查看工作区复用的语义确认

## 7. Agent 上下文建议

当前的 file context 只暴露上传文件列表，参考：

- [server/session/types.go](/home/ifnodoraemon/myagent/data-analysis/server/session/types.go#L93)

后续建议改成 source context，至少包含：

- 可用数据源
- 来源类型
- 已导入表
- 上游来源
- 导入时间
- 语义确认状态
- 可用 join 候选
- 待确认风险

示例：

```text
当前会话可用数据源:
- source_id: src_pg_sales, type: postgres_snapshot, table: sales_orders, imported_at: 2026-03-13T10:20:00Z
  semantic_status: pending_confirmation
  warnings:
  - create_time / pay_time 都可能是时间列
  - customer_id / member_id 都可能与 users 表关联
```

## 8. UI 最小交互

### 8.1 数据库连接

- 工作区设置页新增“数据库连接”
- 表单字段：
  - 名称
  - host
  - port
  - database
  - schema
  - username
  - password
  - allowlist
- 操作：
  - 测试连接
  - 保存连接
  - 浏览表
  - 导入到当前 session

### 8.2 语义确认面板

上传或导入完成后，如果存在歧义，显示确认面板：

- 识别到的时间列候选
- 识别到的主键/外键候选
- 推荐 join 关系
- 风险提示
- 接受建议 / 手动修改 / 暂时跳过

### 8.3 运行中的阻塞提示

如果分析被语义问题阻塞，不能只返回一段报错文本，应明确提示：

- 当前缺少什么确认
- 为什么不确认就不能继续
- 用户应操作哪个字段或关系

## 9. Benchmark 补充

数据库连接和语义确认进入 MVP 后，benchmark 至少新增以下 case：

- PostgreSQL 单表导入后分析
- PostgreSQL 多表导入 + join 候选确认
- 上传 Excel 后时间列候选确认
- 低置信度指标口径确认
- 确认后再次分析，不应重复追问

建议新增指标：

- `semantic_profile_generation_success`
- `semantic_confirmation_required_rate`
- `semantic_confirmation_resolution_rate`
- `post_confirmation_task_success_rate`

## 10. 明确不做的事

在这个阶段不做：

- 任意数据库全支持
- 跨数据库实时 join
- 写回数据库
- 自动修改数据库 schema
- 跳过人工确认直接在低置信度关系上继续分析
