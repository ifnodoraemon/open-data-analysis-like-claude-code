# Prompt/Runtime 分层改造计划

更新日期：2026-03-23

这份清单替代旧版 todo，聚焦一个明确目标：把当前 Agent runtime 从“仅在 API 适配层上区分 `system` / `user`”升级为“在内部语义上稳定区分 `policy`、`task`、`runtime context`、`history`”，同时不破坏项目既有的 agentic 原则。

## 一、背景判断

当前实现并不是完全没有区分提示词层级：

- 主引擎初始化时，planner prompt 作为首条 `system` 消息保存。
- 简单聊天接口仍然显式传入 `systemPrompt` 和 `userPrompt`。
- OpenAI Responses 适配层会把全部 `system` 消息合并进 `instructions`。
- Anthropic 适配层会把全部 `system` 消息合并进顶层 `system` 参数。

真正的问题不在于“有没有 `system`”，而在于“哪些内容被送进了高优先级指令层”。当前历史 digest 也会被注入为 `system` 消息，这会把“历史事实摘要”和“稳定约束”提升到同一优先级，不符合本项目强调的“runtime 提供事实，不替模型裁决”的方向。

补充观察：

- 最新报告 run `r_971ccb6b` 已出现明显渲染失真：
  - 最终 HTML 中目录和章节标题出现 `Block 1`、`Block 2`、`Block 4` 等文案。
  - 对应 `report_manage_blocks` 调用中，多处 markdown block 的 `title` 实际为空，但 `content` 中已经有模型生成的 `## 执行摘要`、`## 1. 数据概览`、`## 2. 销售表现分析` 等 heading。
  - 渲染层继续追加外层章节标题与章节编号，导致“外层 `Block N` + 内层模型 heading”双层标题冲突。
  - finalize 没有把这类问题视为非法结构，导致这份报告被正常交付。

这说明当前报告链路还有第二类问题：渲染器仍在改写或补全模型表达，而不是忠实渲染模型已经返回的标题、内容与顺序。

## 二、目标

- 保留明确的指令层级，不再把所有内容混成一个 prompt。
- 只把稳定的角色约束、硬 guardrail、不可违反的交付规则放进高优先级指令层。
- 把用户任务、运行时事实、历史摘要从高优先级指令层中拆出来。
- 保持 observation-first 和 pull-based state access，不把大量状态重新默认注入每轮上下文。
- 为 OpenAI、Anthropic 以及未来可能接入的 provider 建立统一的内部语义模型，而不是继续以某一家 SDK 的 message 结构作为内部真相。
- 报告渲染层以模型输出为单一事实来源：模型返回什么标题、编号、层级、顺序，就渲染什么；系统不再补题、不再补编号、不再把空标题渲染成占位文案。

## 三、非目标

- 不引入固定 workflow，不把 runtime 改造成阶段机。
- 不把“下一步应该做什么”重新塞回 system/developer prompt。
- 不把 memory、report state、subgoals、session files 改成每轮强制注入。
- 不为了“看起来更稳”而把历史摘要重新伪装成高优先级规则。
- 不在本轮顺手扩展数据库数据源、对象存储或其他无关能力。
- 不通过渲染器兜底生成 `Block N`、`HTML Block N`、`图表 chart_id`、默认报告标题等占位内容来掩盖模型输出缺口。

## 四、报告渲染原则

报告链路新增一条明确原则：

- 表达层以模型输出为准。
- 系统不改写模型已经给出的标题文案、章节编号、目录顺序、正文 heading 层级。
- 系统不为缺失标题生成占位标题。
- 系统不把 block ID、chart ID、数组序号包装成面向用户可见的章节标题。
- 系统不在渲染时偷偷推断“模型本来想写什么标题”。

允许保留的仅是结构性约束，而不是内容性兜底：

- 非法 HTML 清理
- 缺失图表引用拦截
- finalize 前结构完整性校验
- draft / finalized 交付边界校验

不允许保留的行为：

- `Block N` / `HTML Block N` / `图表 xxx` 之类占位标题
- 自动给章节加数字前缀
- 自动剥离或规范化模型标题中的序号前缀
- 当模型内容里已有 heading 时，再套一层系统生成 heading
- 因为模型没填某字段，就替它发明一个用户可见文案
- 用内容偏好冒充结构合法性，例如“章节标题重复”“图表必须有说明文字”之类规则直接阻止 finalize

补充边界：

- 内部持久化仍可保留非用户可见的技术性 ID 自动生成，例如 block ID、chart container ID。
- 这类技术 ID 不能进入最终对用户可见的标题、目录、正文或图下注释。

## 五、目标架构

后续内部 prompt/runtime 结构固定为 4 层：

### 1. Policy Layer

用途：

- 角色定位
- 不可违反的边界
- 薄 guardrail
- 与报告交付、歧义确认、领域边界相关的稳定规则

要求：

- 内容稳定、短小、少变
- 不混入历史摘要、检索结果、会话状态
- 不重复工具 description 里已经明确表达的契约

### 2. Task Layer

用途：

- 当前用户请求
- 用户补充说明
- 用户明确授权的假设范围

要求：

- 保持“这是用户想要什么”这一语义
- 不把 runtime 自己的建议伪装成用户意图

### 3. Runtime Context Layer

用途：

- 历史 digest
- 必要时注入的事实型上下文
- 压缩后的 report state / memory facts / session facts
- 需要临时随轮次携带、但不属于高优先级规则的运行时信息

要求：

- 事实表达，不夹带建议
- 默认按需出现，而不是每轮强制膨胀
- 优先级低于 policy

### 4. History Layer

用途：

- 多轮 user / assistant / tool 交互轨迹
- 工具调用参数和结果
- 子代理过程消息

要求：

- 继续做 compaction
- 保留可审计性
- 不把 compaction 产物回灌成 policy

## 六、Provider 编译规则

内部统一表示先建立，再由 provider adapter 编译：

- OpenAI Responses：
  - `policy` 编译到 `instructions`
  - `task` 编译到 `input` 中的用户消息
  - `runtime context` 编译到低优先级 carrier，不再并入 `instructions`
  - `history` 和 `tool` 继续按消息轨迹表达

- Anthropic：
  - `policy` 编译到顶层 `system`
  - `task` 编译到 `messages`
  - `runtime context` 编译到普通消息层，不再并入顶层 `system`
  - `history` 和 `tool result` 保持当前语义

- 未来 provider：
  - 不再直接复用 OpenAI SDK message 结构作为唯一内部抽象
  - 新 provider 只实现从内部 prompt bundle 到目标 API 的编译器

## 七、实施计划

### P0. 明确内部语义模型

目标：

- 先把“内部真相”从 `[]openai.ChatCompletionMessage` 中解耦出来。

待做：

- 新增统一的内部 prompt 数据结构，例如 `PromptBundle`、`PromptLayer`、`RuntimeContextBlock`、`ConversationItem`。
- 明确哪些字段属于 `policy`，哪些属于 `task`，哪些属于 `runtime context`，哪些属于 `history`。
- 规定 compaction 产物只能落到 `runtime context` 或专门的 digest carrier，不能再落到 `system`。
- 保留与现有工具调用、trace、memory、delegate 的兼容边界。

涉及文件：

- `server/agent/engine.go`
- `server/agent/llm.go`
- `server/agent/types.go`
- 新增内部 prompt bundle 相关文件

完成标准：

- 引擎内部不再把 provider-specific message role 当成唯一语义来源。
- 可以在不依赖 OpenAI role 细节的前提下表达四层语义。

### P1. 收敛 Policy Prompt

目标：

- 把当前 planner prompt 改成真正稳定、精简的 policy prompt。

待做：

- 将 `BuildPlannerPrompt` 重构为更明确的 `BuildPolicyPrompt` 或等价命名。
- 保留必须存在的稳定规则：
  - 无固定 workflow
  - 交付必须 finalize
  - 核心口径歧义必须确认
  - 领域边界约束
- 删除或下沉不应长期待在高优先级层的信息：
  - 可从 tool schema 获取的冗长工具描述
  - 可由 observation tool 拉取的运行时状态
  - 可从 trace/history 获得的过程信息
- 目标长度控制到明显低于当前版本，优先保留硬规则，去掉重复表达。

涉及文件：

- `server/agent/prompts.go`
- `server/agent/prompts_test.go`
- `docs/llm-trace-audit.md`

完成标准：

- policy prompt 不再承担“工具目录 + 规则大全 + 历史说明”的混合职责。
- prompt 测试仍能覆盖歧义确认、finalize 约束和 delivery guardrail。

### P2. 重构 Provider Adapter

目标：

- 让 OpenAI 和 Anthropic 的编译逻辑只接收内部 prompt bundle，不再自行拼接“所有 system 消息”。

待做：

- 重写 `buildResponsesRequest`，只把 `policy` 放入 `instructions`。
- 重写 Anthropic 消息转换逻辑，只把 `policy` 放入顶层 `system`。
- 为 `runtime context` 选定统一的低优先级 carrier，并在两个 provider 中保持一致语义。
- 为 trace 增加分层统计字段，至少区分：
  - `policy_chars`
  - `task_chars`
  - `runtime_context_chars`
  - `history_chars`

涉及文件：

- `server/agent/llm.go`
- `server/agent/debug_log.go`
- `server/agent/trace.go`

完成标准：

- 任意历史 digest 都不会再被并入 OpenAI `instructions` 或 Anthropic `system`。
- trace 可以看出每层上下文分别占了多少空间。

### P3. 调整 Context Compaction 策略

目标：

- 保留长任务上下文治理收益，同时取消“digest 提升为 system”这件事。

待做：

- 重写 `compactWorkerMessages` 及相关逻辑，使摘要产物进入 `runtime context` 而不是 `system`。
- 审查主代理与子代理两套 compaction 路径，保证行为一致。
- 明确哪些 tool result 可以直接保留，哪些只保留 `ui_summary` 或结构化摘要。
- 检查子代理 delegate 回流摘要，避免把 delegate trace 误注入 policy 层。

涉及文件：

- `server/agent/engine.go`
- `server/agent/worker.go`
- `server/agent/harness_test.go`
- `server/agent/worker_test.go`

完成标准：

- 历史摘要仍可节省 token。
- 摘要的语义优先级低于 policy。
- 主代理和子代理的 compaction 规则一致。

### P4. 校准 Runtime Facts 的注入边界

目标：

- 只在确有必要时注入最小事实块，其余状态继续通过 observation tools 拉取。

待做：

- 梳理当前哪些事实必须随轮次携带，哪些完全可以保持 pull-based。
- 为必须注入的事实定义统一格式，例如“runtime context packet”，内容仅含事实、限制、ID、状态，不含建议。
- 检查 handler、engine、delegate 链路中是否存在把“该怎么做”伪装成事实注入用户消息的情况。
- 确保 `ui_summary` 继续只服务于展示，不重新承担隐藏提示词作用。

涉及文件：

- `server/agent/engine.go`
- `server/agent/worker.go`
- `server/handler/*`
- `server/tools/*`

完成标准：

- 自动注入内容保持事实性。
- 不出现“请先调用某工具”或“建议下一步做什么”这类隐式 workflow 文本。

### P5. 子代理接口与命名收敛

目标：

- 避免 `task_delegate` 的接口继续强化“system_prompt 可以随便追加一切内容”的心智模型。

待做：

- 审查 `task_delegate` 的 `system_prompt` 参数语义。
- 评估改名为 `policy_appendix`、`delegate_policy` 或保留旧字段但在内部重解释的兼容方案。
- 明确子代理只能继承父级稳定 policy，并允许小范围附加边界，不能把大段历史和上下文塞进子代理 policy。
- 增补 tool description，强调这是约束补充，不是自由上下文通道。

涉及文件：

- `server/agent/worker.go`
- `server/agent/worker_test.go`
- `server/tools/registry.go`

完成标准：

- 子代理接口语义更清晰。
- 不再鼓励把临时事实通过 `system_prompt` 传给子代理。

### P6. 报告渲染去兜底，改为模型直出

目标：

- 报告标题、目录、章节 heading、正文层级与顺序全部以模型实际返回内容为准，渲染器不再补题、不再补编号、不再规范化。

待做：

- 删除报告渲染中的用户可见占位标题兜底：
  - `Block N`
  - `HTML Block N`
  - `图表 chart_id`
- 删除标题规范化逻辑中对序号、章节前缀的自动裁剪，避免系统重写模型原始标题文案。
- 停止在 markdown/html/chart block 外层自动注入章节编号。
- 调整 markdown/html block 的渲染策略：
  - 若模型显式给了 `title`，按原文渲染该标题
  - 若模型未给 `title`，则不生成替代标题；正文按原样渲染
  - 若正文里已有 heading，则仅渲染该 heading，不再额外包一层系统 heading
- 调整目录生成逻辑，使目录反映最终实际渲染出的 heading，而不是系统合成标题。
- 若某个 block 最终没有可见 heading，则该 block 可以正常渲染正文，但不应因为目录需要而被系统补一个标题。
- 去掉 `RenderReportHTML` 中的默认报告标题兜底；最终报告标题必须来自 finalize 参数或明确状态字段，而不是 renderer 自己发明。
- 移除默认作者兜底，不要在 renderer 或 finalize 中静默补成固定文案。
- 去掉 `resolveReportTitleFromBlocks` 一类面向最终交付的标题推断逻辑；最终报告标题必须来自显式字段，而不是从 block 里反推。
- 删除渲染阶段的“缺图占位”输出；若图表引用不存在，应在 finalize 或更早阶段直接报错，而不是输出“图表未找到”。
- 收紧 finalize 的“结构合法性”定义，只保留真正的结构错误：
  - 报告为空
  - 引用了不存在的图表
  - 同一图表被错误重复引用
  - 其他会导致最终 HTML/快照结构损坏的问题
- 从 finalize blockers 中移除内容偏好型限制，至少包括：
  - `duplicate_block_heading:*`
  - `chart_block_missing_caption:*`
- 审查 `state_report_inspect` 暴露的报告质量信号，把它们保留为 observation facts，而不是 finalize 的硬门槛。
- 保留结构合法性校验，但不要把“内容应该怎么写”重新编码成 guardrail。

涉及文件：

- `server/tools/report_html.go`
- `server/tools/report_blocks.go`
- `server/tools/report_finalize.go`
- `server/tools/report_guardrails.go`
- `server/tools/report_tools.go`
- `server/agent/state_tools.go`

完成标准：

- 最终报告中不再出现系统生成的占位标题。
- 模型内容中的标题、编号、层级不会被 renderer 二次改写。
- 报告顺序严格等于 block 实际顺序。
- 缺失标题不会被系统补成目录项或章节名。
- 缺失图表引用会直接失败，而不是输出占位内容。
- 内容偏好型规则不再阻断 finalize。
- 系统只拦结构非法结果，不再用占位文案掩盖内容缺失。

### P7. 测试、Benchmark 与回归

目标：

- 用测试和 trace 指标确保这次重构不只是“换了命名”，而是真正改变了上下文分层。

待做：

- 新增或更新单测：
  - policy prompt 内容测试
  - adapter 编译测试
  - compaction 层级测试
  - delegate policy 传递测试
- 更新现有 harness 测试，去掉“digest 一定是 system 消息”的过强假设，改为验证“digest 不属于高优先级 policy”。
- 扩展 benchmark 与 trace 审计文档，记录每层字节占比。
- 对关键 scenario 做回归，重点覆盖：
  - 歧义确认
  - report finalize
  - 长历史压缩
  - delegate 恢复与失败重试
  - 报告空标题但正文自带 heading 的渲染
  - 报告标题/编号原文保真
  - 目录与最终可见 heading 一致

涉及文件：

- `server/agent/prompts_test.go`
- `server/agent/harness_test.go`
- `server/agent/engine_test.go`
- `server/agent/llm_test.go`
- `server/agent/worker_test.go`
- `docs/benchmark.md`
- `docs/llm-trace-audit.md`

完成标准：

- 测试能证明层级是有效存在的，而不是只靠注释解释。
- benchmark 能量化 policy/context/history 的体积变化。
- 报告渲染测试能证明系统不再发明用户可见标题。

### P8. 文档与迁移说明

目标：

- 把新分层方案固化到项目文档，避免后续回退到“什么都往 system 塞”的模式。

待做：

- 更新 `docs/agentic-principles.md`，补充 prompt/runtime 分层原则。
- 在 `AGENTS.md` 或相邻文档中补充最小实现约定：
  - policy 只放稳定规则
  - runtime context 只放事实
  - history digest 不得进入高优先级层
- 补一份迁移说明，明确旧字段、旧测试和旧 trace 字段如何过渡。

涉及文件：

- `docs/agentic-principles.md`
- `AGENTS.md`
- `README.md`
- `README.zh-CN.md`

完成标准：

- 新同事不需要翻实现细节，也能理解为什么不能把 digest 放进 `system`。

## 八、建议落地顺序

建议按以下顺序推进，避免一次性大改动导致调试困难：

1. 若当前优先级是修复报告质量，先做 P6；这部分与 prompt layering 基本解耦，收益也最直接。
2. 再完成 P0，定义内部 bundle 和层级语义。
3. 接着做 P1 和 P2，把 policy 与 provider 编译通道拆开。
4. 然后做 P3，迁移 compaction。
5. 再做 P5，收敛 delegate 接口语义。
6. 最后做 P4、P7、P8，补齐边界、测试与文档。

说明：

- P4 放在 P2/P3 之后更稳，因为先有分层容器，才能准确决定哪些 runtime facts 该注入。
- P7 必须伴随每个阶段逐步推进，但在清单中单独列出，用于强调验收门槛。

## 九、验收标准

全部工作完成时，应满足以下条件：

- 引擎内部存在明确的四层语义模型。
- OpenAI `instructions` 和 Anthropic `system` 中只包含 policy。
- 历史 digest 不再进入高优先级指令层。
- 用户任务与 runtime facts 在语义和载体上都可区分。
- 现有 agentic 原则没有被固定 workflow 回归破坏。
- trace 和 benchmark 能直接量化各层上下文体积。
- 最终报告的用户可见标题、章节编号、目录项不再由系统兜底发明。
- 单测、集成测试、scenario 回归全部通过。

## 十、暂不纳入本轮

- 重新设计全部 tool schema
- 引入新的多代理编排框架
- 因本次分层改造而新增大规模自动状态注入
- 为了兼容某一家 provider 而牺牲内部语义清晰度
