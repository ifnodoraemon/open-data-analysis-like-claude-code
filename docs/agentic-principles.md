# Agentic Principles

更新日期：2026-03-18

这份文档用于固定本项目的智能体方向，避免后续实现逐步滑回“隐藏 workflow + 系统代判”的模式。

## 核心原则

### 1. 不预设路径，只预设目标与边界

系统只定义：

- 用户目标是什么
- 可用工具有哪些
- 哪些约束不能违反
- 当前世界状态是什么

系统不定义：

- 先做什么再做什么
- 哪一步之后必须进入哪一步
- 主 Agent 何时必须 delegate
- 报告应该按哪种固定章节顺序生成

Agent 可以自己决定探索、委派、写作、修订、收尾的顺序。

### 2. 系统提供状态，不提供裁决

系统可以提供事实状态：

- 目标树
- 工作记忆
- 报告 block / chart 状态
- 上下文大小
- 历史 trace

但系统不应直接给出这类判断：

- “现在应该 delegate”
- “这个章节证据不足”
- “下一步应该先补图再写摘要”

这些判断应由 LLM 自己通过观察状态后得出。

### 3. Judge 属于模型，不属于 Runtime

本项目区分四层：

- `Runtime`
  - 负责上下文压缩、超时、持久化、并发、取消
- `State`
  - 负责暴露当前世界状态和中间产物
- `Act`
  - 负责执行查询、画图、写 block、委派子 agent
- `Judge`
  - 由 LLM 自己决定下一步、是否并行、是否收尾

Runtime 不应代替 Judge。

### 4. Guardrail 只能做薄约束

允许保留的系统硬约束：

- 防止非法或损坏结果落地
- 防止上下文无限膨胀
- 防止引用不存在对象
- 防止在根目标未闭环时 finalize
- 防止模型生成的报告 HTML 绕过前端 sanitizer / CSP 后执行非可信脚本

不允许把 guardrail 扩大成隐式流程编排器。

### 4.1 Finalize 是交付边界，不是下一轮提示

`report_finalize` 的职责是校验并标记报告可交付。它成功返回 `ok=true`、`is_finalized=true`、`delivery_state=finalized` 后，当前 run 应进入收尾状态。

这属于交付边界，不属于“模型下一步应该做什么”的建议。runtime 可以据此停止开放工具的执行循环，避免 finalize 成功后继续发起下一轮可调用工具的 LLM 循环，造成界面看起来仍在分析或重复收尾。

但最终给用户看的收尾回复仍应由模型生成，而不是由 runtime 拼硬编码模板或注入一段“应该如何总结”的任务提示。合理模式是：把 `report_finalize` 结果作为事实放入历史，再给模型一次 tools disabled 的自然续写机会；模型根据历史和报告状态写出自然语言总结，然后 runtime 完成当前 run。

如果 finalize 失败，工具应返回 blockers / issues 等事实，run 继续由模型自行判断如何处理。

### 5. 观察工具优先于系统注入建议

如果模型需要更多自我判断能力，优先增加观察类工具，例如：

- `state_memory_inspect`
- `state_goal_inspect`
- `state_report_inspect`
- `state_report_edit_inspect`
- `state_session_sources_inspect`

不要优先在 system prompt 或 runtime 注入“你现在应该如何行动”的判断文本。

### 6. 工具契约要详细，但不能替模型决策

工具描述不应该只写一句极短摘要，也不应该把 workflow 写进 description。

更合适的工具契约应明确说明：

- 这个工具做什么
- 什么时候适合调用
- 什么时候不适合调用
- 输入参数的语义
- 会读取哪些状态
- 会修改哪些状态
- 返回结果包含哪些事实
- 有哪些限制、失败条件或边界

工具 description 可以很详细，这本身并不违背 agentic 方向。问题不在于“写得多”，而在于是否把“下一步应该怎么做”的判断偷偷塞进工具契约。

### 7. 工具返回事实，不返回下一步指令

工具返回可以包含：

- 原始结果
- 结构化状态
- 错误码
- 错误细节
- 用于 UI 展示的简短摘要（统一使用 `ui_summary`）

工具返回不应包含这类字段或语义：

- `next_action`
- “你现在应该先做 X 再做 Y”
- “建议接下来调用某个工具”

系统可以提供结果摘要，但摘要应描述“发生了什么”，而不是规定“接下来怎么走”。

不要为新的工具返回继续引入 `summary_text` 这类语义模糊字段。展示摘要和事实载荷应分离：

- `ui_summary`
  - 给前端、运行预览和日志使用
- 结构化事实字段
  - 给模型、工具链和后续状态处理使用

旧字段可以为兼容历史记录保留读取能力，但不应再作为新的主写入路径。

### 8. 用户消息中的运行时补充信息应保持事实性

Runtime 在必要时可以为用户消息补充结构化运行时事实，例如：

- 当前请求是否是报告编辑
- 目标 run / block 是什么
- 是否存在编辑范围约束

但这些补充信息应保持事实表达，而不是变成对模型的隐式指导。例如，不应在用户消息里写“请先调用某个 state tool”。

### 9. 状态应优先以可拉取的方式暴露

像上传文件摘要、报告结构、局部编辑范围、工作记忆这类状态，优先通过显式 observation tool 暴露，让模型按需拉取。

不优先采用：

- 每轮自动注入大段状态文本
- 在 handler 中把文件语义、报告结构直接拼进用户消息
- 因为担心模型遗漏状态，就默认把所有状态塞进上下文

这条原则的目标，是让状态获取成为 agent 的主动行为，而不是 runtime 的隐式 workflow。

### 10. 结构化中间表示应是可选的，不是默认思维路径

像目标树、报告 block tree 这样的结构化表示可以存在，但它们应被视为可选工具或可选产物，而不是模型默认必须依赖的思维路径。

例如：

- `goal_manage` 应是可选的结构化 scratchpad
- `report_manage_blocks` 是最终产物组织接口，但不应反过来规定模型的推理顺序

### 10.1 报告渲染脚本必须与模型内容分层

报告内容可以由模型组织，但图表运行时代码不应混入模型生成的 HTML 内容。

当前约定：

- 报告 HTML 可以包含 chart 容器和结构化 chart option 数据
- ECharts loader 与 chart runtime 应使用可信的外部脚本资源
- 默认优先使用同源静态资源，例如 `/assets/echarts.min.js` 和 `/oda-chart-runtime.js`
- 前端 sanitizer 负责只放行可信脚本、样式、URL 和报告属性
- CSP 不应为了修复图表而放开通用 inline script，例如不应新增宽泛的 `unsafe-inline`

这样做的目标不是把报告生成变成固定流程，而是保持安全边界清晰：模型负责内容与数据，runtime / frontend 负责可信渲染机制。

### 11. 歧义应由 agent 基于事实暴露，而不是静默拍板

当分析依赖的核心口径存在多个合理候选，例如：

- `gross_revenue` / `net_revenue` / `recognized_revenue`
- 多个可能的 join key
- 日 / 周 / 月粒度混用
- 元 / 万元 / 美元等单位口径并存
- 字段名别名映射存在多个高相似候选

runtime 不应替模型默认选一个，也不应在 handler 里偷偷替用户下定义。

更符合 agentic 的做法是：

- 通过 tool 暴露候选事实与约束
- 由模型判断这类歧义是否会实质影响计算或结论
- 第一时间明确向用户确认，而不是边算边默认拍板
- 只有用户明确允许“你自行做合理假设”时，模型才可以带着假设继续，并且需要在输出里说清楚假设

### 12. 严格的上下文分层 (Prompt Layering)

为了保持代理行为的稳定性和防止上下文污染，系统严格执行四层 Prompt 模型：

- **Policy (`system` 层)**: 纯静态行为准则和工具边界。绝对禁止将临时事实（Runtime Facts）、用户对话历史或任务提示注入到此层。
- **Task (`user` 层)**: 用户的直接指令。
- **Runtime Context (`runtime` 层)**: 当前轮次必需的客观事实（如：编辑范围、激活目标）。不能包含对模型的任何建议和操作指引。
- **History (`history` 层)**: 对话记忆，包括历史压缩摘要。注意：历史摘要绝对不能被提升到 `system` 层。

无论是主 Agent 还是 Delegate Agent，都默认继承最纯粹的 Policy。如果是子代理需要额外规则约束，必须使用专用字段 `policy_appendix` 而不是利用它做 Context Dump。

## 当前明确反对的模式

以下实现方向与本项目方向不符：

- 线性 workflow prompt，例如 `A -> B -> C -> finalize`
- 系统自动告诉模型“这里有问题、你该怎么修”
- 以阶段机名义强制进入固定 phase
- 因为模型偶尔做得不好，就把判断逻辑搬回代码里
- 用预设章节模板替代 agent 自主组织内容
- 在工具返回中加入 `next_action` 一类建议字段
- 在 handler / runtime 拼接“请先调用某个工具”之类的指路文本
- 因为想提升稳定性，就把文件语义、报告状态、编辑上下文默认注入每轮 prompt

## 当前允许的运行时能力

以下能力被认为符合方向：

- 自动上下文压缩
- trace 落盘与审计
- 目标树和报告树持久化
- 纯事实型状态检查工具
- 薄 finalize guardrail
- 子 agent 工具边界裁剪
- 详细但契约化的工具描述
- 面向 UI 的展示摘要，但不夹带下一步指令
- 展示摘要字段与事实字段显式分离，例如使用 `ui_summary`
- 前端 sanitizer 与 CSP 作为报告预览的安全边界
- finalize 成功后由 runtime 禁止继续工具循环，并让模型生成最终用户回复后结束当前 run

## 当前实现约定

目前系统默认不再把 memory、subgoals、report 状态自动注入到每一轮模型上下文。

如果模型需要这些状态，应显式调用观察工具：

- `state_memory_inspect`
- `state_goal_inspect`
- `state_report_inspect`
- `state_report_edit_inspect`
- `state_session_sources_inspect`

这条约定的目的，是让“观察状态”成为模型的自主动作，而不是 runtime 的隐式指导。

## Tool Contract Checklist

当你新增或修改工具 description / schema / 返回结构时，至少检查下面这些问题：

1. description 是否说明了用途、边界和副作用，而不仅是一个空标题？
2. description 是否在暗中告诉模型固定路径或推荐下一步？
3. 返回 payload 是否主要是事实，而不是建议？
4. 是否错误地把 UI 友好摘要和 agent 决策信息混在一起？如果需要 UI 摘要，是否单独放在 `ui_summary`？
5. 如果工具存在状态写入、副作用或作用域限制，是否已经明确写出？

## 改动前自检

当你准备新增一个 prompt 规则、runtime 逻辑或工具时，先问这 5 个问题：

1. 这是在提供状态，还是在替模型下判断？
2. 这是防坏结果的 guardrail，还是在暗中规定路径？
3. 这个能力能否改成一个 observation tool，而不是 system instruction？
4. 如果删掉这段逻辑，模型只是更难，还是会产出非法结果？
5. 这项改动会不会让系统更像 workflow engine，而不是 agent runtime？

如果答案偏向“替模型判断”或“规定路径”，默认不应这样做。

## 外部实践借鉴

以下模式来自对 Harness AI Agent 架构的分析，经过筛选后纳入本项目的实践：

### 借鉴：结构化错误分类与自恢复

Harness 的 HQL 采用 fail-fast 模式，错误信息清晰且可直接重试。本项目遵循相同原则：

- 工具返回结构化错误字段（`error`、`ok`），不返回 `next_action` 建议
- 模型根据错误细节自主决定恢复策略，runtime 不替模型选择重试路径
- 破坏性操作（如 report finalize）采用 fail-closed 模式：如果前提条件不满足，操作失败而不是静默降级

### 不借鉴：Skills 层的 phase gates

Harness 的 Skills 层编码了多步有序工作流（如"7-step Deploy New Service"），包含 phase gate（"do not proceed until this step is done"）。本项目明确反对这种模式，原因：

- 数据分析路径是涌现的，不是预设的
- Phase gate 本质上是隐式 workflow 编排，违反原则 1
- 在分析场景中，"错误的探索顺序"成本远低于 DevOps 场景

### 不借鉴：Skills 启动时自动注入上下文

Harness 在 agent 启动时自动加载 Skills 指令（CLAUDE.md、AGENT.md）到 agent context。本项目采用 pull-based 暴露：状态通过 observation tool 按需获取，不在每轮 prompt 中预注入。

### 不借鉴：混合 prompt 分层

Harness 没有严格的四层 prompt 模型，Skills 混合了事实状态和操作指导。本项目严格执行四层分层（Policy / Task / RuntimeContext / History），防止上下文污染和层级越权。

### 借鉴：Sondera Harness 的 STEER 模式

Sondera Harness（Cedar-policy-based agent guardrail framework）提出 STEER 模式：当 guardrail 拒绝某个 action 时，返回拒绝原因（factual），由模型自主决定替代路径，而不是简单硬阻断。这与本项目的 agentic 原则高度契合：

- 工具返回事实性错误（`ok=false`、`error_code`、`detail`），不附加恢复建议
- 模型读取错误事实后自主选择恢复策略（修改 SQL、缩小查询范围、换用其他工具等）
- 这与 BLOCK 模式不同：BLOCK 适合安全关键操作（如禁止的 DDL），而 STEER 适合可恢复的操作失败（如查询超时、行数超限）
- 本项目的 finalize guardrail 本质是 BLOCK 模式（阻止非法最终输出），而查询失败等场景采用 STEER 模式（返回事实，模型自主恢复）
