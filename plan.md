# 数据源演进计划：Source-First Agent Runtime 与 SQL Snapshot 接入

## Summary

- 将系统从“文件上传驱动”升级为“数据源驱动”：所有可分析对象统一建模为 `DataSource -> SourceSnapshot -> SemanticProfile -> Analysis`。
- 不考虑历史兼容。旧的 file-centric agent 工具和 UI 状态可以被替换为 source-first 形态；`file` 只保留为上传对象的物理存储记录。
- SQL 数据源首版只支持 PostgreSQL 只读接入，并导入为 session SQLite snapshot。agent 不直接 live query 上游数据库。
- CSV 与 PostgreSQL snapshot 的目标能力包含百万级行数分析，但只承诺面向筛选、分组、聚合和抽样解释的分析；明细全量浏览、任意复杂 OLAP 和高并发查询不在本阶段目标内。
- 语义层不做隐藏 workflow。runtime 只暴露 schema、profile、ambiguity、confirmation 等事实；agent 自己判断何时询问用户、何时继续局部分析。
- Guardrail 保持薄约束：只阻止基于未确认关键歧义落地强结论、错误 join 或 finalized report，不阻止不依赖该歧义的局部分析。

## Core Design

- `DataSource` 是所有分析入口的统一实体。首版支持 `file_upload` 和 `postgres_connection` 两类 source。
- `SourceSnapshot` 表示某个 source 在某个 session 中被物化到 analysis SQLite 的一次固定快照。所有 SQL、图表、报告都只基于 snapshot 表执行。
- `SemanticProfile` 是事实载荷，不是流程裁判。它保存确定性 schema、LLM 语义候选、候选 join、候选 metric、候选 time column、候选 unit、ambiguity flags 和 confidence/warnings。
- `SemanticConfirmation` 保存用户或 workspace 明确确认过的业务口径。confirmation 只在 `schema_signature` 精确匹配时复用，不做模糊迁移。
- `file` 不再直接出现在 agent 的主观察面里。上传文件后立即生成 file-backed `DataSource`，并在当前 session 中创建 `SourceSnapshot`。
- `state_session_sources_inspect` 成为主要 observation tool，返回当前 session 的 sources、snapshots、semantic profile 摘要、ambiguity 状态、已确认 override。
- `state_semantic_profile_inspect` 返回单个 profile 的详细候选事实，用于 agent 判断是否需要 `user_request_input`。
- `data_describe_table` 只描述 snapshot table 的事实，并叠加已确认 override；不把未确认候选包装成 verified fact。
- `data_query_sql` 继续只查询 session SQLite，只允许只读 `SELECT/WITH`。SQL 数据源导入后与文件导入后的表完全同质。
- `user_request_input` 仍由 agent 自主调用。runtime 不自动向用户发问，也不在 handler 拼接“下一步应该确认”的提示。

## Backend Changes

- 新增 metadata 表：`data_sources`、`database_connections`、`source_snapshots`、`session_source_bindings`、`semantic_profiles`、`semantic_confirmations`。
- `data_sources` 最小字段：`id`、`workspace_id`、`name`、`source_type`、`status`、`file_id`、`created_by`、`created_at`、`updated_at`。
- `database_connections` 最小字段：`source_id`、`driver`、`host`、`port`、`database_name`、`default_schema`、`ssl_mode`、`username`、`secret_ciphertext`、`allowlist_json`、`last_tested_at`、`last_test_status`、`last_error_message`。
- `source_snapshots` 最小字段：`id`、`session_id`、`source_id`、`upstream_kind`、`upstream_schema`、`upstream_object`、`analysis_table_name`、`row_count`、`column_count`、`status`、`error_message`、`schema_signature`、`imported_at`。
- `semantic_profiles` 最小字段：`id`、`session_id`、`source_id`、`snapshot_id`、`analysis_table_name`、`schema_signature`、`profile_status`、`profile_json`、`created_at`、`updated_at`。
- `semantic_confirmations` 最小字段：`id`、`profile_id`、`workspace_id`、`session_id`、`confirmed_by`、`scope`、`overrides_json`、`created_at`。
- 增加 `source` service，负责 file upload source、PostgreSQL source、snapshot import、semantic profiling、confirmation 查询与合并。
- 将现有 `Ingester.GenerateSchemaMetadata` 和 `EnrichSemanticProfile` 收敛到 snapshot import 完成后的统一 profiling 阶段。
- profiling 输出统一结构：`schema`、`semantic_candidates`、`join_candidates`、`metric_candidates`、`time_candidates`、`unit_candidates`、`ambiguities`、`warnings`。
- ambiguity 只是事实字段。不要用 `needs_confirmation` 作为全局阻塞状态；使用 `profile_status=draft|profiled|confirmed|rejected`，并在 `profile_json.ambiguities` 中表达风险。
- confirmation 合并逻辑固定为：workspace confirmation 先应用，session confirmation 后应用，session 覆盖 workspace。
- 新增 thin guardrail：当 report finalize 时，如果报告正文或 chart 明确使用了未确认的 ambiguous join/metric/time/unit，finalize 失败并返回结构化 blocker；不返回下一步建议。
- PostgreSQL 接入使用 `pgx/v5/stdlib`。上游连接必须只读，所有 catalog 和 import 对象必须在 allowlist 中。
- 凭证加密使用 `sha256(AUTH_SECRET)` 派生 AES-GCM key。`AUTH_SECRET` 缺失或长度不足时禁止创建 SQL 数据源。
- SQL snapshot import 不设置源数据行数上限。导入时按批次流式读取上游结果并写入 session SQLite，限制主要来自磁盘、时间和 SQLite 写入能力。百万级能力的具体要求见 `Million-Row Requirements`。
- agent 不允许自由生成上游数据库 SQL。PostgreSQL 数据源只允许从 allowlist 中选择表或视图做 snapshot import。

## Million-Row Requirements

- CSV 和 PostgreSQL snapshot import 必须使用 streaming reader + batch writer，默认 batch size 从当前小批量写入升级为可配置值，首版默认 `5000`。导入过程不得把完整数据集加载到 Go 内存。
- Snapshot 记录必须补充运行事实：`rows_imported`、`import_duration_ms`、`profile_duration_ms`、`snapshot_size_bytes`、`profile_mode`、`last_error_message`。这些字段供 UI、benchmark 和 agent observation 使用。
- 大表 profiling 不能默认对每列做全表 `COUNT(DISTINCT)`、`SELECT DISTINCT`、`MIN/MAX/AVG`。导入阶段同步维护 bounded sample，默认最多 `10000` 行，用于列类型、样本值、非空率估计、候选时间列、候选指标和候选 join 初筛。
- Profile 中必须区分 `estimated` 和 `exact`。默认 profile 以 sampled/mixed facts 为主；精确统计只在 agent 或用户明确需要某个字段、指标或口径时，通过 `data_query_sql` 执行只读聚合查询得到。
- 索引策略只覆盖 candidate time columns、id columns、candidate/confirmed join keys、用户确认的高频 filter fields。不要对所有 nominal columns 自动建索引，避免百万级导入后索引构建失控。
- `data_query_sql` 继续限制返回最多 `200` 行，但查询超时改为可配置：默认 quick timeout `5s`，large snapshot aggregate timeout `30s`。超时返回结构化事实错误，不返回下一步建议。
- 当 `row_count >= 1000000` 时，`state_session_sources_inspect` 返回 `large_dataset=true`、`row_count`、`profile_mode=sampled|mixed|exact`、`snapshot_size_bytes`。这些是观察事实，不是隐藏 workflow 指令。
- 百万级支持的首版验收口径是 `1M-5M` 行、几十列以内、单表或少量 snapshot join、聚合优先分析。超过千万级、宽表上百列、复杂多表 join 或高并发场景需要另立执行引擎计划。

## APIs And Tools

- `POST /api/upload?session_id=...`：上传文件，创建 file-backed source，导入当前 session snapshot，返回 `source_id`、`snapshot_id`、`analysis_table_name`、`row_count`、`column_count`、`semantic_profile_id`。
- `GET /api/sessions/{sessionID}/sources`：返回当前 session 的 source/snapshot/profile 摘要。
- `GET /api/semantic-profiles/{profileID}`：返回完整 profile facts、ambiguities、applied confirmations。
- `POST /api/semantic-profiles/{profileID}/confirm`：提交 `scope` 和 `overrides`，只写 confirmation，不自动触发分析。
- `POST /api/data-sources`：创建 PostgreSQL workspace source。
- `GET /api/data-sources`：列出当前 workspace source。
- `POST /api/data-sources/{sourceID}/test`：测试连接和 allowlist。
- `GET /api/data-sources/{sourceID}/catalog`：只返回 allowlist 对象。
- `POST /api/data-sources/{sourceID}/import`：将指定 allowlist 对象导入指定 session snapshot。
- 主工具集替换为：`state_session_sources_inspect`、`state_semantic_profile_inspect`、`data_describe_table`、`data_query_sql`、`report_*`、`memory_*`、`goal_manage`、`task_delegate`、`user_request_input`。
- `state_session_files_inspect` 和 `data_load_file` 从主 agent 工具集中移除。必要时可保留内部 helper，但不再作为主要 agent 合约。

## Frontend Changes

- `InputBar` 保留文件上传入口，同时新增 `Sources` 按钮打开数据源抽屉。
- 数据源抽屉包含三块：当前会话 snapshots、工作区 SQL sources、语义歧义/确认项。
- 上传文件后前端展示的是 source/snapshot，而不是 uploaded file tag。
- PostgreSQL 创建表单包含：名称、host、port、database、schema、ssl mode、username、password、allowlist。
- SQL source 导入 UI 只允许选择 allowlist 对象；导入行为固定为完整 snapshot import，不暴露行数上限、limit 或 filter 选项。
- semantic profile detail UI 展示候选时间列、候选指标、候选 join、候选单位和 warnings。用户可保存为 session confirmation 或 workspace confirmation。
- Pinia store 新增：`sessionSources`、`workspaceDataSources`、`semanticProfileSummaries`、`semanticProfileDetails`。
- 首版不新增 websocket 事件。上传、导入、确认完成后通过 REST refetch 当前 session source/profile 状态。
- 大表导入状态通过 REST 轮询 `GET /api/sessions/{sessionID}/sources` 展示，不新增 websocket 事件。UI 展示 `status`、`rows_imported`、`row_count`、`import_duration_ms`、`profile_mode`、`last_error_message`。

## Benchmark And Validation

- 先不把 benchmark 作为 SQL 数据源接入的前置阻塞项，但必须在 source-first 改造完成后立即接入回归。
- 复用 `samples/coverage_scenarios` 作为主语料，不再维护空的 `benchmarks/cases` 平行目录。
- runner 读取 scenario 配置，执行上传/导入/提问/等待 run 完成/导出 summary。
- 第一批 blocking smoke：`01_sales_complete`、`04_roi_joinable`、`12_ambiguous_metrics`、`14_time_grain_reconcilable`。
- 第二批扩展回归：`05_roi_unjoinable`、`13_join_key_conflict`、`15_unit_mismatch_explicit`、`16_delegate_failure_recovery`、`17_delegate_child_tool_failure_recovery`、`18_delegate_partial_recovery`。
- SQL 数据源完成后新增 PostgreSQL fixtures：单表导入分析、多表 join 歧义确认、大表批量导入成功。
- 新增百万级 fixtures：`csv_million_single_table_import`、`pg_million_snapshot_import`、`million_row_grouped_aggregate`、`million_row_profile_latency`。
- 核心验收指标：source import success、schema profile success、ambiguity surfaced、confirmation applied、SQL query success、report finalize guardrail accuracy、no overclaim on ambiguous evidence。
- 百万级验收指标：导入内存有界、profile 不做默认逐列全表扫描、profile facts 正确标记 estimated/exact、聚合查询在 large snapshot timeout 内完成、工具返回不超过行数上限、报告不把抽样事实包装成精确结论。

## Milestones

1. Source-first metadata and services：新增领域模型、repository、migration、source service，文件上传直接生成 source/snapshot/profile。
2. Agent observation refactor：新增 source/profile observation tools，移除主工具集中的 file-centric 工具，调整 prompt/tool contract，确保工具返回事实而非建议。
3. Semantic confirmation and guardrail：实现 confirmation API/UI、override 合并、finalize thin guardrail。
4. PostgreSQL snapshot import：实现连接管理、凭证加密、allowlist catalog、无行数上限的批量 snapshot import。
5. Benchmark stabilization：runner 接入 scenario corpus，CI smoke 从 continue-on-error 逐步升级为 blocking。

## Assumptions And Defaults

- 不考虑旧 session、旧工具、旧 UI 状态兼容；可以做破坏性迁移和 API 替换。
- session SQLite 继续是唯一分析执行层。
- SQL 数据源首版只支持 PostgreSQL，不展开讨论 MySQL、Snowflake、BigQuery 等其他源。
- CSV 和 PostgreSQL snapshot import 不设置行数上限；实现时必须使用批量/流式导入，避免一次性把全量数据载入内存。
- “不设置行数上限”不是承诺无限规模。当前阶段承诺百万级可用，实际边界由磁盘、导入耗时、SQLite 查询能力、profile 采样策略和单机资源决定。
- session SQLite 仍是首版执行层。如果目标升级到千万级以上、复杂 OLAP、多用户并发或长期数据仓库能力，需要评估 DuckDB、ClickHouse 或上游数据库 pushdown，但不纳入本轮 source-first PostgreSQL 接入。
- runtime 不自动替 agent 提问；ambiguity 是事实，是否询问由 agent 决定。
- guardrail 只拦非法或高风险最终输出，不规定分析路径。
