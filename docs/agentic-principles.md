# Agentic Principles

更新日期：2026-03-14

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

不允许把 guardrail 扩大成隐式流程编排器。

### 5. 观察工具优先于系统注入建议

如果模型需要更多自我判断能力，优先增加观察类工具，例如：

- `state_memory_inspect`
- `state_goal_inspect`
- `state_report_inspect`
- `state_context_inspect`

不要优先在 system prompt 或 runtime 注入“你现在应该如何行动”的判断文本。

### 6. 结构化中间表示应是可选的，不是默认思维路径

像目标树、报告 block tree 这样的结构化表示可以存在，但它们应被视为可选工具或可选产物，而不是模型默认必须依赖的思维路径。

例如：

- `goal_manage` 应是可选的结构化 scratchpad
- `report_manage_blocks` 是最终产物组织接口，但不应反过来规定模型的推理顺序

## 当前明确反对的模式

以下实现方向与本项目方向不符：

- 线性 workflow prompt，例如 `A -> B -> C -> finalize`
- 系统自动告诉模型“这里有问题、你该怎么修”
- 以阶段机名义强制进入固定 phase
- 因为模型偶尔做得不好，就把判断逻辑搬回代码里
- 用预设章节模板替代 agent 自主组织内容

## 当前允许的运行时能力

以下能力被认为符合方向：

- 自动上下文压缩
- trace 落盘与审计
- 目标树和报告树持久化
- 纯事实型状态检查工具
- 薄 finalize guardrail
- 子 agent 工具边界裁剪

## 当前实现约定

目前系统默认不再把 memory、subgoals、report 状态自动注入到每一轮模型上下文。

如果模型需要这些状态，应显式调用观察工具：

- `state_memory_inspect`
- `state_goal_inspect`
- `state_report_inspect`

这条约定的目的，是让“观察状态”成为模型的自主动作，而不是 runtime 的隐式指导。

## 改动前自检

当你准备新增一个 prompt 规则、runtime 逻辑或工具时，先问这 5 个问题：

1. 这是在提供状态，还是在替模型下判断？
2. 这是防坏结果的 guardrail，还是在暗中规定路径？
3. 这个能力能否改成一个 observation tool，而不是 system instruction？
4. 如果删掉这段逻辑，模型只是更难，还是会产出非法结果？
5. 这项改动会不会让系统更像 workflow engine，而不是 agent runtime？

如果答案偏向“替模型判断”或“规定路径”，默认不应这样做。
