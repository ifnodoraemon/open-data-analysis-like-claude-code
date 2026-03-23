# 系统审查报告（2026-03-23）

本报告汇总了截至 2026-03-23 对当前系统进行静态审查、代码路径核对，以及局部测试/构建验证后确认的主要问题。

已执行的验证：

1. 后端测试：`GOCACHE=/tmp/go-build go test ./...`
2. 数据层测试：`GOCACHE=/tmp/go-build go test ./data`
3. 前端构建：`npm run build`

说明：

1. 本报告以高置信度问题为主，偏向行为回归、权限边界、持久化一致性、恢复链路、前端状态机失真等。
2. 文件路径和行号均为审查时对应的代码位置，后续代码变动后行号可能漂移。
3. 部分问题之间会相互叠加，文中已尽量指出组合风险。

## 高优先级问题

1. 历史报告快照渲染已经出现回归。新版 `report_html` 逻辑优先使用正文里的 heading，同时不再为仅有 `block.Title` 的 markdown/html block 生成默认标题，导致旧快照中“title 有值、content 只是正文”的常见形态丢标题。现有测试已直接失败：`server/handler/run_test.go:36`。相关代码：`server/tools/report_html.go:520`、`server/tools/report_html.go:582`。

2. `waiting_user_input` 任务在服务重启、session eviction 或 cache miss 后不可可靠恢复。WS 入口会重建一个新的空 session；恢复逻辑又依赖内存中的 `ConsumeWaitingRun()` 与 engine 等待点，因此待确认任务可能在历史上显示“等待输入”，但实际上无法继续。相关代码：`server/handler/ws.go:357`、`server/handler/ws.go:437`、`server/session/types.go:298`、`server/session/manager.go:137`。

3. 最终报告保存失败时，系统仍可能宣告“报告已生成完成”。`finalizeAndPersistReport()` 在保存失败后先发 `error`，但随后无条件继续发 `report_final`；而这条错误又不经过 runtime dispatcher hook，因此 run 状态不会被一致地转成 failed。相关代码：`server/handler/ws.go:225`、`server/handler/runtime_hooks.go:92`、`server/handler/runtime_hooks.go:115`。

4. `data_load_file` 仍然没有做“当前 session 是否真正持有该文件”的校验，存在同工作区跨会话读文件的越权面。当前 session 初始化时把 `file_id -> MaterializeToTemp(workspaceID, fileID)` 暴露给工具；`MaterializeToTemp()` 已经补了 workspace 校验，但仍只验证 `file.WorkspaceID == workspaceID`，没有检查该文件是否通过 `session_files` 挂到了当前 session。只要模型拿到同工作区内其他会话的 `file_id`，仍可能被导入分析。相关代码：`server/session/types.go:84`、`server/session/types.go:91`、`server/service/file_service.go:103`。

5. `code_run_python` 并不是真正受限的 Python 沙箱，而是在共享解释器里直接 `exec` 任意代码。当前实现仍保留默认 builtins/import 能力，执行代码可直接访问标准库、文件系统和进程能力，与工具说明中的“受限 Python 沙箱”不一致。相关代码：`python-executor/main.py:28`、`python-executor/main.py:55`、`python-executor/main.py:141`、`python-executor/main.py:149`。

6. Python 执行的 `timeout` 只是 HTTP 客户端超时，不是执行器超时。执行器声明了 `timeout` 字段，但 `exec` 周围没有任何中断/超时控制；Go 侧只是等待 HTTP 请求超时。死循环或长时间阻塞代码会在调用方报错后继续占用执行器。相关代码：`python-executor/main.py:62`、`python-executor/main.py:129`、`python-executor/main.py:149`、`server/tools/python.go:82`、`server/tools/python.go:102`。

7. Python 执行之间没有运行隔离，前一次请求留下的文件和解释器状态会泄露到后一次请求。所有请求共用 `/app/workspace`；`files_before/files_after` 基于全局目录做差集；解释器命名空间只是浅拷贝 `GLOBAL_NS`。这使得后续 run 可以读取历史产物，甚至篡改共享模块状态影响后续执行。相关代码：`python-executor/main.py:25`、`python-executor/main.py:138`、`python-executor/main.py:142`、`python-executor/main.py:162`、`python-executor/main.py:185`。

8. 绝大多数受保护接口只验证 token 声明，不做实时工作区成员校验。被移出工作区的用户，在旧 token 过期前，仍可能继续访问属于该 workspace 的 session、run 和报告。鉴权中间件只做 `Parse(token)` 后写入 context；多数 handler 仅检查资源上的 `user_id/workspace_id` 是否与 token claim 相等。相关代码：`server/auth/identity.go:31`、`server/handler/session.go:12`、`server/handler/session.go:31`、`server/handler/run.go:21`、`server/handler/run.go:51`、`server/handler/run.go:79`。对比之下，文件下载路径反而有实时 `IsMember` 校验：`server/service/file_service.go:232`、`server/service/file_service.go:256`。

9. 删除 session 时会把该 session 关联的源文件实体和对象存储一并删除，但 `session_files` 实际是多对多关联，这会误删其他 session 仍然引用的文件。当前删除计划直接收集 `session_files.file_id` 并 `DELETE FROM files`，随后再删除底层对象，没有检查这些文件是否还被其他 session 关联。相关代码：`server/migrations/001_init.sql:63`、`server/handler/session_delete.go:56`、`server/handler/session_delete.go:59`、`server/handler/session_delete.go:71`、`server/handler/session_delete.go:87`、`server/handler/session_delete.go:152`。

## 中高优先级问题

10. 新增的“按顶级 heading 拆分 markdown block”会把一个源 block 渲染成多个章节，但这些章节复用了同一个 `data-block-id`。前端点击其中某一个拆分片段时，后端实际拿到的仍是原始 block id，导致“重生成本段”会重写整个原始 block。相关代码：`server/tools/report_html.go:703`、`client/src/components/report/ReportPreview.vue:151`、`client/src/components/report/ReportPreview.vue:207`。

11. 报告预览的前端安全边界过宽。当前 sanitizer 会保留所有无 `src` 的 inline script，而预览 iframe 同时启用了 `allow-scripts allow-same-origin`，导致一旦上游报告 HTML 的脚本清洗漏掉，前端最后一道防线几乎失效。相关代码：`client/src/utils/sanitize.js:118`、`client/src/components/report/ReportPreview.vue:50`。

12. 异步事件持久化队列的关闭逻辑仍有并发 panic 风险。`saveEventToDB()` 先将全局 channel 快照到局部变量再 send，但这并不能避免另一 goroutine 在其间 `close(q)`，从而触发“向已关闭 channel 发送” panic。相关代码：`server/handler/ws.go:642`、`server/handler/ws.go:652`，队列初始化与关闭逻辑：`server/handler/ws.go:45`。

13. 事件持久化队列明确允许高压丢消息，但运行态恢复又依赖消息历史重建 memory/subgoals，这两者在模型上是矛盾的。高负载下，不只是日志会缺失，恢复出来的 runtime state 事实本身也可能失真。相关代码：`server/handler/ws.go:654`、`server/handler/runtime_state.go:33`。

14. 运行历史恢复时，`tool_result` 与真实 `tool_call` 的对应关系会丢失。`run_messages` schema 没有 `tool_call_id` 字段，落库时每条消息都生成新 UUID；恢复时只能按“最近同名工具”猜测配对。一旦一个 assistant turn 连续调用同名工具两次，重建后的历史会串线。相关代码：`server/migrations/001_init.sql:106`、`server/handler/ws.go:299`、`server/handler/ws.go:320`、`server/handler/ws.go:664`、`server/handler/ws.go:684`、`server/domain/message.go:5`。

15. repository 层广泛使用 `INSERT OR REPLACE`，会把“更新”变成 SQLite 语义上的“删除旧行再插入新行”。这对 `workspaces/files/reports/sessions/analysis_runs` 等关键实体都很危险，容易在重复写入、重试、并发 finalize 或初始化时悄悄覆盖既有状态。相关代码：`server/repository/sqlite/store.go:118`、`server/repository/sqlite/store.go:130`、`server/repository/sqlite/store.go:198`、`server/repository/sqlite/store.go:213`、`server/repository/sqlite/store.go:281`。

16. `report_file_id` 没有数据库外键约束，run 可以绑定到一个不存在的报告文件且数据库不会报错。相关 schema 与写入代码：`server/migrations/001_init.sql:83`、`server/repository/sqlite/store.go:421`。

17. 删除 session 的失败语义不原子。删除链路在真正删库前会先 `sessionManager.Stop()`，该函数会直接取消活跃任务并等待 idle；若后续 build plan、事务开始或 SQL 删除失败，接口虽然返回失败，但用户任务已经被打断。相关代码：`server/handler/session_delete.go:19`、`server/session/manager.go:208`。

18. `bootstrap` 存在多处静默降级，会把真实异常伪装成“当前工作区没有数据”。`workspaceRepo.ListByUser` 的错误被忽略；`runRepo.ListBySession`、`fileService.GetSessionFiles`、`ensureSession` 等失败也大多直接少字段或回空状态。相关代码：`server/handler/bootstrap.go:14`、`server/handler/bootstrap.go:36`、`server/handler/bootstrap.go:48`、`server/handler/bootstrap.go:55`、`server/handler/bootstrap.go:65`。

19. DOCX 导出接口本质上是一个无大小限制的通用 HTML 转换入口。前端把当前 iframe 快照 HTML 直接 POST 到服务端，后端既不验证它是否对应某个已授权 run，也没有请求体大小限制，而是直接落临时文件并调用 `pandoc`，容易成为 CPU/磁盘消耗点。相关代码：`client/src/components/report/ReportPreview.vue:248`、`server/handler/report_export.go:9`、`server/service/report_export.go:12`、`server/service/report_export.go:25`、`server/service/report_export.go:37`。

20. Python 执行产物下载接口 `/files/{filename}` 没有任何鉴权上下文，也没有基于 run/session 做命名空间隔离，只按共享目录中的裸文件名直接返回文件。虽然当前主系统尚未直接对外暴露该路径给浏览器，但只要执行器容器可被访问，这就是一个独立的数据泄露面。相关代码：`python-executor/main.py:185`。

21. 本地对象存储实现会将 key 直接拼到 rootDir 下；虽然大多数 key 来自受控生成函数，但 `Get/Delete/Exists` 本身没有额外路径规整或越界防护，未来若引入可控 key 路径，将直接扩大文件系统攻击面。相关代码：`server/storage/local/storage.go:29`、`server/storage/local/storage.go:56`、`server/storage/local/storage.go:65`、`server/storage/local/storage.go:73`。

22. TTL 自动清理只扫描 `Manager.sessions` 里的内存态 session，不会遍历数据库中的历史 session。服务重启后未被重新加载到内存的过期 session，不会进入清理候选集，从而长期残留在 DB 与对象存储中。相关代码：`server/session/cleanup.go:17`、`server/session/cleanup.go:24`。

23. TTL 自动清理将 `waiting_user_input` 视为“非空闲”，因此被挂起后长期无人响应的 session 不会过期清理。对交互式确认场景这是保守策略，但从资源治理角度看，它会把 abandoned session 永久留存。相关代码：`server/session/cleanup.go:29`、`server/session/cleanup.go:45`。

24. 删除 session 时，对象存储删除失败和内存/cache 清理失败都会被吞掉并继续返回成功。事务提交后，存储对象删除只是写日志；`sessionManager.Delete()` 失败也只是写日志。结果是 API 可以回 `204`，但对象、cache 或本地 SQLite 残留并未真正清干净。相关代码：`server/handler/session_delete.go:67`、`server/handler/session_delete.go:71`、`server/handler/session_delete.go:79`、`server/session/types.go:478`。

25. 登录时若客户端传了无效的 `workspaceId`，服务端会静默回退到用户的第一个工作区并重新签发 token，而不是显式报错。这会把“工作区选择参数错误/过期”伪装成一次正常登录，前端后续在错误工作区继续 bootstrap，排障成本较高。相关代码：`server/handler/auth.go:98`。

## 中优先级问题

26. 前端刷新恢复报告时，仍把 `reportFileId` 作为主动加载报告 HTML 的前提。这会让“已有 `reports` 记录但没成功绑定 `report_file_id`”的任务刷新后直接显示空白预览。相关代码：`client/src/composables/useWebSocket.js:103`、`client/src/composables/useWebSocket.js:191`。

27. WebSocket 安全面弱于 REST。服务端 `CheckOrigin` 全放开，鉴权允许从 query string 取 token，前端实际就是把 token 拼进 WS URL。这会增加 token 落入日志、代理、浏览器诊断记录的概率，也放松了跨源连接边界。相关代码：`server/handler/ws.go:29`、`server/auth/identity.go:39`、`client/src/composables/useWebSocket.js:262`。

28. 生成的报告 HTML 依赖公网 CDN。字体使用 Google Fonts，图表 runtime 使用 jsDelivr 的 ECharts。在内网或受限网络环境下，导出 HTML 的展示质量和图表能力会直接退化。相关代码：`server/tools/report_html.go:95`、`server/tools/report_html.go:400`。

29. `user_request_input` 的 `allow_multiple` 前后端契约断裂。后端工具支持多选，UI 组件也支持，但 WebSocket 状态层在落地消息时丢弃了 `allow_multiple` 字段，导致前端永远按单选处理。相关代码：`server/agent/agentic_tools.go:181`、`client/src/composables/useWebSocket.js:496`、`client/src/components/agent/AgentPanel.vue:59`、`client/src/components/agent/AgentPanel.vue:230`。

30. 数据导入失败会留下半截表。CSV/Excel 都是在建表并插入部分数据之后，才继续批量导入剩余内容；中途失败时不会回滚已写入的表和数据。Excel 的 10 万行上限检查也发生在部分写入之后。相关代码：`server/data/ingester.go:179`、`server/data/ingester.go:299`、`server/data/ingester.go:317`。

31. 表名和列名清洗不够稳健。`sanitizeTableName()` 与 `sanitizeColumnName()` 只做了非常有限的替换，没有处理引号、重复列名、多个空列名冲突等情况；而建表 SQL 又直接拼接这些标识符。相关代码：`server/data/ingester.go:428`、`server/data/ingester.go:515`。

32. 前端上传控件允许 `.json`，但后端数据导入根本不支持 JSON。结果是用户可以上传成功，却只能在后续分析阶段收到“不支持的文件类型”错误。相关代码：`client/src/components/layout/InputBar.vue:20`、`server/handler/upload.go:13`、`server/data/ingester.go:124`。

33. 前端对 `waiting_user_input` 的状态展示仍然失真。bootstrap 时把 `waiting_user_input` 直接走 `startRun()`，会把挂起任务当成正在运行；实时收到 `user_request_input` 时，只关闭 `isRunning`，却不 patch run 状态，也不清理 `activeRunId`。因此运行树和 live badge 仍可能继续显示“实时/running”。相关代码：`client/src/composables/useWebSocket.js:61`、`client/src/composables/useWebSocket.js:496`、`client/src/stores/agent.js:198`、`client/src/stores/agent.js:204`、`server/handler/runtime_hooks.go:121`。

34. 最终报告型任务会在消息流里重复显示“完成”。`report_final` 分支先插入一条固定完成消息，随后 `run_completed` 又追加一条完成消息。相关代码：`client/src/composables/useWebSocket.js:428`、`client/src/composables/useWebSocket.js:442`。

35. WebSocket 建连后的 `session_ready` 事件会把已有会话标题覆盖成硬编码的“未命名分析”。bootstrap 已拿到真实标题，但前端在 `session_ready` 中再次 `upsertSession({ title: '未命名分析' })`，会把旧标题抹掉。相关代码：`client/src/composables/useWebSocket.js:364`、`client/src/stores/agent.js:133`。

36. 报告快照恢复、运行恢复和前端恢复之间的契约并不一致。后端文档仍强调可恢复，但目前实现更接近“报告可回放、运行态只能部分从消息历史推导”。相关代码与说明：`README.md:13`、`server/session/manager.go:132`。

37. `GetRunReportHandler` 的恢复逻辑会优先信任 `reports` 表中的 snapshot 或 HTML storage key，但前端刷新逻辑又依赖 `run.reportFileId`。这使得相同的真实状态在“接口可读”和“前端是否主动加载”之间出现分叉。相关代码：`server/handler/run.go:91`、`server/handler/run.go:113`、`client/src/composables/useWebSocket.js:103`。

38. 局部编辑和 block 拆分后的语义不一致，会进一步放大报告编辑体验问题。表面上 UI 允许精确选中拆分章节，但后端授权和重生成仍以原始 block 粒度工作。相关代码：`server/tools/report_html.go:703`、`server/tools/report_tools.go:87`、`client/src/components/report/ReportPreview.vue:151`。

39. 登录/Bootstrap 返回的用户字段与前端展示字段不一致。服务端返回的是 `name`，而侧边栏优先读 `store.user?.username`，导致用户昵称区域更容易退化成邮箱展示。不是权限问题，但属于明确的前后端契约不一致。相关代码：`server/handler/auth.go:81`、`server/handler/bootstrap.go:23`、`client/src/components/layout/Sidebar.vue:107`。

40. 删除当前正在查看的 session 时，旧 WebSocket 连接不会被服务端主动失效，前端也要等 `DELETE -> loadSessions -> createNewSession` 这串流程跑完后才断开旧连接。服务端 WS handler 在整个生命周期内都持有本地 `sess` 指针并持续 `ReadMessage()`；如果删除后旧连接上又收到 `user_message`，会继续尝试 `sess.StartRun()` 并写 `analysis_runs`，最终因为 session 已删而退化成持久化错误。这个问题更像删后残留连接导致的“幽灵会话”与混乱报错。相关代码：`client/src/composables/useWebSocket.js:663`、`server/handler/session_delete.go:19`、`server/handler/session_delete.go:79`、`server/handler/ws.go:416`、`server/handler/ws.go:498`、`server/metadata/store.go:47`。

41. 仓库中的 `server/migrations/001_init.sql` 并不是运行时 schema 的真实来源。实际启动时走的是 `metadata.Open()` 内嵌的 `store.migrate()` 和 `ensureColumn()` 逻辑，代码中没有看到任何对 `001_init.sql` 的执行路径。这会导致“SQL 文件中的 schema”与“真实运行中的 schema”发生静默漂移，进而误导审查、运维排障和手工修库。相关代码：`server/handler/init.go:43`、`server/metadata/store.go:32`、`server/metadata/store.go:58`、`server/metadata/store.go:210`。

42. 当前前端在“查看历史 run”时会出现同屏混态。`openRun()` 会把消息列表、报告和 runtimeState 切到目标历史 run；但后续实时事件过滤同时接受 `activeRunId` 和 `selectedRunId`，而 `state_memory_updated/state_subgoals_updated` 分支又不区分当前正在看的 run，导致活跃 run 的记忆/目标增量会覆盖历史 run 刚加载出来的 runtime state。结果就是用户明明在看历史 run 的消息与报告，侧边的工作记忆和阶段目标却显示实时 run 的状态。相关代码：`client/src/composables/useWebSocket.js:199`、`client/src/composables/useWebSocket.js:351`、`client/src/composables/useWebSocket.js:507`、`client/src/composables/useWebSocket.js:512`、`client/src/components/agent/WorkingMemoryPanel.vue:28`、`client/src/components/agent/SubgoalTree.vue:32`。

## 说明与后续建议

1. 当前最需要优先修的不是“单个 bug”，而是几条跨模块主链路：
   `report_finalize -> SaveReportHTML -> BindReportFile -> report reload`
   `user_request_input -> 挂起 -> 断线/重启 -> 恢复`
   `upload/materialize -> data_load_file -> session delete`
   `code_run_python -> 执行隔离 -> 产物暴露 -> 超时治理`

2. 如果要进入修复阶段，建议先补最小回归测试矩阵：
   历史快照渲染回归
   finalize 保存失败时 run 必须 failed
   `waiting_user_input` 跨重连恢复
   删除 session 不得误删共享文件
   Python 执行超时与隔离

3. TTL 自动清理路径复用了 `Manager.FullDeleteFunc`，因此手动删除链路中的删除语义问题也会同步影响后台自动清理。相关代码：`server/session/cleanup.go:64`。
