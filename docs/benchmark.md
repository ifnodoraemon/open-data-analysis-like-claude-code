# Benchmark 设计

本文档定义当前仓库的数据分析智能体 benchmark 方案。目标不是复刻学术榜单，而是建立一个能持续回归产品核心链路的内部评测集。

## 目标

- 覆盖 `上传文件 -> 建表理解 -> SQL / Python 分析 -> 图表生成 -> 报告生成 -> 刷新恢复`
- 同时衡量正确率、稳定性、成本和执行组织质量
- 能接入当前 `docker compose` 本地环境和后续 CI

## 参考基线

当前方案主要借鉴这些公开 benchmark 的评测思路：

- `Spider 2.0 / BIRD / LiveSQLBench`
  - 关注复杂 schema、真实 SQL 查询和执行结果
- `SpreadsheetBench`
  - 关注真实 Excel / Spreadsheet 文件任务
- `DS-1000 / DSCodeBench`
  - 关注 pandas / numpy / matplotlib 等数据科学代码能力
- `BIRD-Interact`
  - 关注交互式、多步 agent 工作流

我们不直接照搬这些数据集，而是抽取其评测方法，落到本产品的实际能力边界。

## Benchmark 分层

### 1. SQL 查询层

输入：

- 一个或多个 CSV / Excel 文件
- 用户任务，如“比较各区域毛利率并给出 top2 / bottom2”

判分：

- `sql_exec_success`
- `numeric_answer_accuracy`
- `read_only_compliance`
- `row_limit_compliance`

重点：

- schema 理解
- 只读 SQL 生成质量
- 聚合、排序、过滤、时间字段处理

### 2. Spreadsheet 理解层

输入：

- 多 sheet Excel
- 非标准表头
- 含空行、合计行、中文字段名

判分：

- `file_load_success`
- `table_mapping_accuracy`
- `header_inference_quality`

重点：

- `load_data`
- `describe_data`
- 多文件 / 多表命名稳定性

### 3. Python 分析层

输入：

- SQL 不适合的问题，如分组透视、滚动计算、复杂异常检测

判分：

- `python_exec_success`
- `computed_metric_accuracy`
- `fallback_behavior`

重点：

- `run_python` 只在必要时使用
- 工具不可用时是否降级到 SQL / 文本解释

### 4. 图表层

输入：

- 明确要求图表的分析任务

判分：

- `chart_valid_rate`
- `chart_grounding_rate`
- `chart_reference_integrity`

重点：

- 图表是否能渲染
- 图表数据是否来自前序查询
- `{{chart:chart_id}}` 是否与已创建图表一致

### 5. 报告层

输入：

- 完整业务分析任务

判分：

- `report_generation_success`
- `report_groundedness`
- `section_completeness`
- `report_reopen_success`

重点：

- 标题、摘要、overview、analysis、conclusion 是否齐全
- 结论是否引用真实数字
- 报告是否能持久化下载并刷新恢复

### 6. Agent 编排层

输入：

- 需要多步探索的问题

判分：

- `task_success_rate`
- `tool_sequence_quality`
- `tool_error_recovery`
- `context_growth_control`

重点：

- 是否先看 schema 再分析
- 是否避免无意义重复调用
- prompt / tool history 是否失控膨胀

## 任务格式

建议每个 benchmark case 使用一个目录：

```text
benchmarks/
  cases/
    retail_h1/
      files/
        regional_sales_monthly.csv
        marketing_channel_monthly.csv
        inventory_snapshot.csv
      task.md
      expected.json
```

`task.md`：

- 用户原始需求
- 是否允许 `run_python`
- 是否要求图表 / 报告

`expected.json`：

- `expected_tables`
- `expected_metrics`
- `expected_chart_ids`
- `expected_report_sections`
- `tolerances`

## 最小指标集

第一版建议先落 8 个核心指标：

- `task_success_rate`
- `load_success_rate`
- `sql_exec_success_rate`
- `numeric_answer_accuracy`
- `python_exec_success_rate`
- `chart_valid_rate`
- `report_generation_success_rate`
- `report_reopen_success_rate`

同时记录：

- `median_duration_ms`
- `median_tool_steps`
- `median_llm_calls`
- `median_prompt_bytes`

## 基准任务清单

建议先做 12 个 case：

- 4 个 SQL 主导 case
- 3 个 Excel / 多表理解 case
- 2 个 Python 必需 case
- 2 个完整图表 + 报告 case
- 1 个恢复 / 重新打开报告 case

## 回归触发条件

这些改动后必须跑 benchmark 冒烟：

- system prompt 变更
- tool schema / tool description 变更
- `query_data` / `run_python` 执行边界变更
- 报告生成逻辑变更
- session / run 恢复逻辑变更

## 当前已知需要重点盯的失败模式

- 过早写报告，分析证据还不够
- `create_chart` 参数过大，导致上下文快速膨胀
- `write_section` 大段内容回灌历史，后续轮次 prompt 失控增长
- `run_python` 在 SQL 足够时被误调用
- 历史 run / 当前 run 状态混淆

## 实施顺序

1. 先补 `benchmarks/cases/` 的样例任务目录结构
2. 再补一个最小 benchmark runner，只跑 Docker 本地环境
3. 第一阶段只做 `pass/fail + 指标汇总`
4. 第二阶段再补更细的 groundedness 和图表一致性检查
