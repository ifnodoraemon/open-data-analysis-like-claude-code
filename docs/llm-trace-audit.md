# LLM Trace 审查

审查日期：2026-03-12

样本来源：`data/llm-debug/2026-03-12/`

## 结论摘要

当前智能体的主要组织问题不是“不会分析”，而是“上下文组织效率偏低”。在完整报告任务中，后半程 prompt 会快速膨胀，主要由 `report_create_chart` 的完整图表 option 和 `report_manage_blocks` 的大段正文重复回灌造成。

这会带来三个直接问题：

- 请求体越来越大，成本和延迟持续上升
- 后半程有效分析信息占比下降
- 模型更容易在最后几轮出现格式漂移或不必要的重复

## 样本观察

抽样 trace：

- `134834.026274299-9079bce4`
  - request 大小约 `7.6k`
  - input items `7`
- `134902.389093034-0b175311`
  - request 大小约 `32.1k`
  - input items `23`
- `135106.966264086-7e99fe92`
  - request 大小约 `51.0k`
  - input items `65`
- `135110.331386675-87393b5b`
  - request 大小约 `51.3k`
  - input items `67`

system prompt 本身约 `2268` 字符，虽然不算极端，但在多轮长任务里会持续重复叠加固定成本。

## 主要问题

### 1. 图表参数过大

在 `report_create_chart` 调用里，完整 ECharts `option` 会被回写进 assistant tool call 历史。这个参数本身经常就是几百到几千字符，而且对后续大多数轮次并不关键。

### 2. 报告正文回灌

报告 block 的正文会完整出现在 tool call arguments 中。随着摘要、多个 analysis block 逐步生成，历史里会重复保存大量自然语言正文。

### 3. 历史消息缺少分层

当前消息历史基本是“原样保留”，缺少“执行历史”和“推理上下文”的区别。导致本应只用于审计的长文本，也被持续带入后续推理。

### 4. prompt 仍有一定重复

system prompt 里有一些规则在多个段落重复表达，例如图表章节限制、图表引用要求、报告章节顺序。这不是首要瓶颈，但仍有压缩空间。

## 已完成优化

### 1. 历史参数压缩

已在 `server/agent/engine.go` 中对以下 tool call 做 history compaction：

- `report_create_chart`
- `report_manage_blocks`
- `report_finalize`

压缩后：

- `report_create_chart` 只保留 `chart_id`、`title` 和压缩标记
- `report_manage_blocks` 只保留 block 概要、内容长度和前缀摘要
- `report_finalize` 只保留标题和作者

### 2. LLM trace 事件索引降噪

`events.jsonl` 现在主要保留摘要字段：

- message/tool 数量
- 用户输入摘要
- request/response bytes
- sha256
- 文件路径

原始 request/response 仍在对应 `trace/spans/*` 目录中单独落盘。

### 3. 运行日志降噪

健康检查 access log 已从主应用日志中静音，不再干扰真实分析链路排查。

## 仍建议继续做的事

### 1. 压缩 data_query_sql 大结果

当前 `data_query_sql` 结果仍会完整进入 tool result 历史。对于 200 行上限的查询，这仍可能偏大。下一步建议：

- 对超过阈值的 query result 做 compact summary
- 同时保留原始结果到 artifact 或 trace 文件

### 2. Prompt 再收敛

建议把 system prompt 再压一版，保留硬规则，去掉重复说明和冗长示例，目标控制在 `1200-1600` 字符级别。

### 3. 继续压缩上下文，但避免引入阶段化 workflow

当前 agent 的“分析”和“写报告”会混在同一消息历史里，后续仍应继续做上下文隔离与压缩；但不建议把系统设计成固定的 phase workflow。更符合当前方向的做法是：

- 保持单一 agent runtime
- 用历史压缩、memory、状态观察工具来降低上下文负担
- 只在必要处保留薄 guardrail，而不是强制切 phase

### 4. 建立 prompt-size benchmark

建议把下面指标加入 benchmark：

- `request_json_bytes`
- `input_item_count`
- `avg_tool_arg_bytes`
- `final_report_phase_prompt_bytes`

这样以后每次改 prompt / tool schema，都能量化是否让上下文更重了。
