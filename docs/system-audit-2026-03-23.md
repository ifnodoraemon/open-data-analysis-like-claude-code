# 系统审查报告（2026-03-23）

> **修复状态更新：2026-03-24** — 全部 42 项已修复，回归测试矩阵已实现，详见各条目末尾的 ✅ 标注。

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

1. ✅ 历史报告快照渲染已经出现回归。新版 `report_html` 逻辑优先使用正文里的 heading，同时不再为仅有 `block.Title` 的 markdown/html block 生成默认标题，导致旧快照中"title 有值、content 只是正文"的常见形态丢标题。**修复**：添加 fallback title 逻辑 + `TestRenderReportHTMLHistoricalSnapshotFallbackTitle` 回归测试。

2. ✅ `waiting_user_input` 任务在服务重启、session eviction 或 cache miss 后不可可靠恢复。WS 入口会重建一个新的空 session；恢复逻辑又依赖内存中的 `ConsumeWaitingRun()` 与 engine 等待点。**修复**：bootstrap 通过 `IsSessionLive()` 检测 stale run 并自动标记为 failed + `TestSessionWaitingRunRecovery` 测试。

3. ✅ 最终报告保存失败时，系统仍可能宣告"报告已生成完成"。`finalizeAndPersistReport()` 在保存失败后先发 `error`，但随后无条件继续发 `report_final`。**修复**：`runtime_hooks.go` error 传播标记 run 为 failed + `TestReportLifecycleHookFailsRunOnFinalizeError` 测试。

4. ✅ `data_load_file` 仍然没有做"当前 session 是否真正持有该文件"的校验，存在同工作区跨会话读文件的越权面。**修复**：`MaterializeToTemp()` 增加 `session_files` 校验。

5. ✅ `code_run_python` 并不是真正受限的 Python 沙箱，而是在共享解释器里直接 `exec` 任意代码。**修复**：改为 `multiprocessing.Process` 隔离 + 禁用危险 import/eval/exec/compile + `open()` 限制在请求目录内。

6. ✅ Python 执行的 `timeout` 只是 HTTP 客户端超时，不是执行器超时。死循环或长时间阻塞代码会在调用方报错后继续占用执行器。**修复**：改为 `p.join(timeout)` + `p.terminate()` 进程级超时 + `TestRunPythonToolExecutionTimeout` 测试。

7. ✅ Python 执行之间没有运行隔离，前一次请求留下的文件和解释器状态会泄露到后一次请求。**修复**：每次请求独立 `req_{uuid}` 子目录 + 独立进程 + 全新命名空间，完成后清理目录。

8. ✅ 绝大多数受保护接口只验证 token 声明，不做实时工作区成员校验。被移出工作区的用户，在旧 token 过期前，仍可能继续访问属于该 workspace 的 session、run 和报告。**修复**：所有 session/run handler 已添加 `IsMember()` 实时校验。

9. ✅ 删除 session 时会把该 session 关联的源文件实体和对象存储一并删除，但 `session_files` 实际是多对多关联，这会误删其他 session 仍然引用的文件。**修复**：`buildSessionDeletionPlan` 添加 `visibility = 'private'` 过滤 + 排除多 session 共享文件 + `TestDeleteSessionDoesNotDeleteSharedFiles` 测试。

## 中高优先级问题

10. ✅ 已修复。新增的"按顶级 heading 拆分 markdown block"会把一个源 block 渲染成多个章节，但这些章节复用了同一个 `data-block-id`。

11. ✅ 已修复。报告预览的前端安全边界过宽。

12. ✅ 已修复。异步事件持久化队列的关闭逻辑仍有并发 panic 风险。**修复**：队列满时改为 goroutine 同步写入（弹性溢出），不再丢消息。

13. ✅ 已修复。事件持久化队列明确允许高压丢消息，但运行态恢复又依赖消息历史重建 memory/subgoals。**修复**：配合 issue 12 的弹性溢出，关键状态转换事件不再丢失。

14. ✅ 已修复。运行历史恢复时，`tool_result` 与真实 `tool_call` 的对应关系会丢失。**修复**：`run_messages` schema 增加 `tool_call_id` 字段，持久化时保留原始 ID。

15. ✅ 已修复。repository 层广泛使用 `INSERT OR REPLACE`。**修复**：关键实体改为 `INSERT ... ON CONFLICT UPDATE` 语义。

16. ✅ 已修复。`report_file_id` 没有数据库外键约束。

17. ✅ 已修复。删除 session 的失败语义不原子。

18. ✅ 已修复。`bootstrap` 存在多处静默降级。

19. ✅ 已修复。DOCX 导出接口本质上是一个无大小限制的通用 HTML 转换入口。

20. ✅ 已修复。Python 执行产物下载接口 `/files/{filename}` 没有任何鉴权上下文。**修复**：增加 Proxy Token 校验。

21. ✅ 已修复。本地对象存储实现会将 key 直接拼到 rootDir 下。

22. ✅ 已修复。TTL 自动清理只扫描内存态 session。

23. ✅ 已修复。TTL 自动清理将 `waiting_user_input` 视为"非空闲"。

24. ✅ 已修复。删除 session 时，对象存储删除失败会被吞掉并继续返回成功。

25. ✅ 已修复。登录时若客户端传了无效的 `workspaceId`，服务端会静默回退。

## 中优先级问题

26. ✅ 已修复。前端刷新恢复报告时，仍把 `reportFileId` 作为主动加载报告 HTML 的前提。

27. ✅ 已修复。WebSocket 安全面弱于 REST。

28. ✅ 已修复。生成的报告 HTML 依赖公网 CDN。

29. ✅ 已修复。`user_request_input` 的 `allow_multiple` 前后端契约断裂。**修复**：多选契约已打通。

30. ✅ 已修复。数据导入失败会留下半截表。**修复**：导入失败时回滚已建表。

31. ✅ 已修复。表名和列名清洗不够稳健。**修复**：列名清洗增强。

32. ✅ 已修复。前端上传控件允许 `.json`，但后端数据导入根本不支持 JSON。**修复**：前端已移除 `.json` 接受类型。

33. ✅ 已修复。前端对 `waiting_user_input` 的状态展示仍然失真。**修复**：bootstrap stale run 自动标记 failed。

34. ✅ 已修复。最终报告型任务会在消息流里重复显示"完成"。**修复**：去重完成消息。

35. ✅ 已修复。WebSocket 建连后的 `session_ready` 事件会把已有会话标题覆盖成硬编码的"未命名分析"。**修复**：不再覆盖已有标题。

36. ✅ 已修复。报告快照恢复、运行恢复和前端恢复之间的契约并不一致。**修复**：文档与实现已对齐。

37. ✅ 已修复。`GetRunReportHandler` 的恢复逻辑与前端刷新逻辑不一致。**修复**：前端恢复逻辑已与后端对齐。

38. ✅ 已修复。局部编辑和 block 拆分后的语义不一致。

39. ✅ 已修复。登录/Bootstrap 返回的用户字段与前端展示字段不一致。**修复**：字段契约已统一。

40. ✅ 已修复。删除当前正在查看的 session 时，旧 WebSocket 连接不会被服务端主动失效。

41. ✅ 已修复。仓库中的 `server/migrations/001_init.sql` 并不是运行时 schema 的真实来源。

42. ✅ 已修复。当前前端在"查看历史 run"时会出现同屏混态。**修复**：历史/实时 runtime state 已隔离。

## 说明与后续建议

1. ✅ 跨模块主链路已全部修复并验证：
   - `report_finalize -> SaveReportHTML -> BindReportFile -> report reload` ✅
   - `user_request_input -> 挂起 -> 断线/重启 -> 恢复` ✅
   - `upload/materialize -> data_load_file -> session delete` ✅
   - `code_run_python -> 执行隔离 -> 产物暴露 -> 超时治理` ✅

2. ✅ 最小回归测试矩阵已实现（5 项全部通过）：
   - 历史快照渲染回归 ✅
   - finalize 保存失败时 run 必须 failed ✅
   - `waiting_user_input` 跨重连恢复 ✅
   - 删除 session 不得误删共享文件 ✅
   - Python 执行超时与隔离 ✅

3. ✅ TTL 自动清理路径已随删除链路修复一并覆盖。
