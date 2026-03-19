# 数据充分性测试场景

这组文件用于手工测试 agent 在“字段充分 / 部分充分 / 明显不足 / 多表可关联 / 多表不可关联 / 数据质量问题”下的行为。

建议重点观察 4 件事：

1. agent 是否先基于现有数据分析，而不是臆造缺失字段。
2. 当字段不够时，是否明确说出不能回答的部分，而不是把结论写满。
3. 当多表无法关联时，是否指出 join key 或口径问题。
4. 当数据质量有缺陷时，是否把结论降级为“有限可信”。

## 场景 1：单表字段充分
- 目录：`01_sales_complete`
- 文件：`sales_detail_complete.csv`
- 推荐提问：`请对这份销售数据做全面分析，覆盖趋势、区域、渠道、产品结构和利润表现，并生成图表。`
- 预期：
  - 可以完成较完整的销售全面分析
  - 应能输出趋势、区域、渠道、产品、利润相关结论
  - 不应无端声称缺关键字段

## 场景 2：缺关键维度，但还能部分分析
- 目录：`02_sales_missing_region`
- 文件：`sales_detail_no_region.csv`
- 推荐提问：`请对这份销售数据做全面分析，重点说明区域差异、渠道表现和产品结构。`
- 预期：
  - 应明确说明没有 `region`，无法做区域对比
  - 趋势、渠道、产品结构仍然可以分析
  - 结论应是 partial，而不是直接失败

## 场景 3：缺核心指标，无法回答 ROI
- 目录：`03_marketing_spend_only`
- 文件：`marketing_spend_only.csv`
- 推荐提问：`请评估各渠道 ROI，并解释投放效率差异。`
- 预期：
  - 应明确说明只有投放数据，没有收入/归因收入，无法计算 ROI
  - 可以分析曝光、点击、点击率、转化成本等中间指标
  - 不应编造 ROI 或 attributed revenue

## 场景 4：多表可关联，可做 ROI 分析
- 目录：`04_roi_joinable`
- 文件：
  - `marketing_spend_by_channel_month.csv`
  - `attributed_revenue_by_channel_month.csv`
- 推荐提问：`请综合分析各渠道 ROI、收入贡献和趋势变化，并生成图表。`
- 预期：
  - 应能识别两表可按 `month + channel` 关联
  - 应能计算 ROI 或至少明确使用的 ROI 口径
  - 应输出渠道对比与趋势分析

## 场景 5：多表不可关联
- 目录：`05_roi_unjoinable`
- 文件：
  - `marketing_spend_by_campaign_month.csv`
  - `sales_by_region_month.csv`
- 推荐提问：`请综合分析投放 ROI，并说明各渠道表现。`
- 预期：
  - 应指出两表缺少稳定关联键，无法直接做 ROI 归因分析
  - 可以分别分析投放侧和销售侧
  - 不应把 campaign 花费硬映射到 region 销售

## 场景 6：数据质量问题明显
- 目录：`06_sales_quality_gaps`
- 文件：`sales_detail_quality_gaps.csv`
- 推荐提问：`请对这份销售数据做全面分析，并指出会影响可信度的数据质量问题。`
- 预期：
  - 应识别空值、负值、缺失日期或异常记录
  - 结论应带边界说明
  - 不应把所有图表和结论写成高置信度口径

## 建议的测试顺序
1. 先跑场景 1，确认系统在字段充分时不会过度保守。
2. 再跑场景 2 和 3，确认系统会做 partial / insufficient 表达。
3. 再跑场景 4 和 5，确认多表关联判断是否稳。
4. 最后跑场景 6，观察 agent 对数据质量问题的敏感度。

## 本轮测试重点
这一轮不是测试“应不应该有第三个 Excel”，而是测试：
- 当前字段是否足够支撑用户问题
- 不足时 agent 是否会明确承认边界
- 多表时是否会误连表
- 数据质量差时是否会降低结论强度

## 任务跨度与行业场景

上面 6 个场景主要测“字段是否足够”。除此之外，还建议补一条测试轴：
- `short`：单表、单主题、快速结论
- `medium`：双表或三表、跨主题但链路还比较清晰
- `long`：多表、跨环节、上下文更长、需要更强的边界表达

这一轮先放 3 个代表性行业场景，避免一开始做成全行业 * 全长度的组合爆炸。

### 场景 7：零售 short
- 目录：`07_retail_short`
- 文件：`retail_store_daily_sales.csv`
- 推荐提问：`请快速分析这份零售门店销售数据，给出趋势、门店对比和品类表现。`
- 预期：
  - 应快速完成单表分析
  - 输出不需要过度铺陈
  - 适合看 agent 在短任务里会不会过度探索

### 场景 8：SaaS medium
- 目录：`08_saas_medium`
- 文件：
  - `saas_mrr_monthly.csv`
  - `saas_pipeline_channel_monthly.csv`
- 推荐提问：`请分析 SaaS 业务增长质量，覆盖 MRR 变化、流失、扩张和渠道获客效率。`
- 预期：
  - 应能做多指标增长分析
  - 应把订阅收入和渠道效率分开再汇总
  - 适合观察 medium 任务下 agent 的结构组织能力

### 场景 9：制造 long
- 目录：`09_manufacturing_long`
- 文件：
  - `manufacturing_orders_monthly.csv`
  - `manufacturing_output_monthly.csv`
  - `manufacturing_inventory_monthly.csv`
  - `manufacturing_quality_returns_monthly.csv`
- 推荐提问：`请对这组制造业经营数据做全面分析，覆盖订单、产能、库存、交付和质量风险。`
- 预期：
  - 应能在多表下做更长链路的综合分析
  - 应明确哪些结论是直接证据，哪些只是线索
  - 适合观察 long 任务下的上下文管理、图表组织和边界表达

## 推荐测试矩阵

先不要做全排列。第一轮建议用下面这个矩阵：

1. `字段充分性轴`
   - 场景 1 / 2 / 3 / 4 / 5 / 6
2. `任务跨度与行业轴`
   - 场景 7 / 8 / 9

这样一共 9 组，已经足够看出：
- 短任务是否过度分析
- 中任务是否能稳定组织多指标结论
- 长任务是否会乱连表、乱下强结论
- 行业切换时是否过度依赖固定模板

## 第二批真实边界场景

这一批不再只是测“字段够不够”，而是测 agent 在真实脏环境下的判断：
- 字段名像不像真实业务黑话
- 中英混合时会不会自我匹配
- 指标口径有歧义时会不会主动询问
- 表能不能连时会不会乱连
- 时间粒度不一致时会不会先聚合
- 单位不一致时会不会先做标准化

### 场景 10：alias headers
- 目录：`10_retail_alias_headers`
- 重点：字段名是业务别名和缩写，应该自我匹配，不必追问。

### 场景 11：mixed language headers
- 目录：`11_mixed_language_headers`
- 重点：中英混合字段名和 join 字段语义不一致时，应该自我匹配。

### 场景 12：ambiguous metrics
- 目录：`12_ambiguous_metrics`
- 重点：同一个“收入”有多个候选口径时，不应硬选，应询问或明确假设。

### 场景 13：join key conflict
- 目录：`13_join_key_conflict`
- 重点：表面像能连，但缺稳定主键或粒度不一致时，不应乱连。

### 场景 14：time grain reconcilable
- 目录：`14_time_grain_reconcilable`
- 重点：一张表是日粒度、一张表是月粒度，但可以先聚合后分析。

### 场景 15：unit mismatch explicit
- 目录：`15_unit_mismatch_explicit`
- 重点：单位差异写在字段名里时，应先标准化再分析，并说明处理方式。

### 场景 16：delegate failure recovery
- 目录：`16_delegate_failure_recovery`
- 重点：先触发一次 `task_delegate` 的结构化失败，再观察主 agent 是否能基于失败事实自行恢复、继续本地分析并完成最终报告。

## 建议的判定维度

每个场景都建议记录这些问题：
- `did_inspect_schema`：有没有先看 schema / 表结构
- `did_auto_map_fields`：该自动映射时有没有映射成功
- `did_ask_user`：该追问时有没有追问
- `did_finalize_report`：明确要求生成图表/报告时，有没有真正落地最终报告
- `did_overclaim`：有没有在证据不足时给出强结论
- `did_join_correctly`：多表时有没有正确处理 join / grain / unit
- `did_state_limits`：有没有明确写出限制和假设
- `did_recover_from_delegate_failure`：子代理失败后，主 agent 是否利用结构化错误继续完成主任务
