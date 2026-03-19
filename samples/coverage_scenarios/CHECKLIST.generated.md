# sales_complete

- 目录: `samples/coverage_scenarios/01_sales_complete`
- 行业: `retail`
- 任务跨度: `medium`
- 提问: 请对这份销售数据做全面分析，覆盖趋势、区域、渠道、产品结构和利润表现，并生成图表。
- 上传文件:
  - `samples/coverage_scenarios/01_sales_complete/sales_detail_complete.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 应落地最终报告: `True`
- 必须出现的点:
  - trend
  - region
  - channel
  - product
- 不应出现的说法:
  - missing critical fields

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# sales_missing_region

- 目录: `samples/coverage_scenarios/02_sales_missing_region`
- 行业: `retail`
- 任务跨度: `medium`
- 提问: 请对这份销售数据做全面分析，重点说明区域差异、渠道表现和产品结构。
- 上传文件:
  - `samples/coverage_scenarios/02_sales_missing_region/sales_detail_no_region.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - missing region dimension
- 不应出现的说法:
  - region comparison completed with confidence

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# marketing_spend_only

- 目录: `samples/coverage_scenarios/03_marketing_spend_only`
- 行业: `marketing`
- 任务跨度: `short`
- 提问: 请评估各渠道 ROI，并解释投放效率差异。
- 上传文件:
  - `samples/coverage_scenarios/03_marketing_spend_only/marketing_spend_only.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - cannot compute roi without revenue
- 不应出现的说法:
  - attributed revenue exists

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# roi_joinable

- 目录: `samples/coverage_scenarios/04_roi_joinable`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请综合分析各渠道 ROI、收入贡献和趋势变化，并生成图表。
- 上传文件:
  - `samples/coverage_scenarios/04_roi_joinable/marketing_spend_by_channel_month.csv`
  - `samples/coverage_scenarios/04_roi_joinable/attributed_revenue_by_channel_month.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 应落地最终报告: `True`
- 必须出现的点:
  - month + channel join
- 不应出现的说法:
  - tables cannot be combined

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# roi_unjoinable

- 目录: `samples/coverage_scenarios/05_roi_unjoinable`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请综合分析投放 ROI，并说明各渠道表现。
- 上传文件:
  - `samples/coverage_scenarios/05_roi_unjoinable/marketing_spend_by_campaign_month.csv`
  - `samples/coverage_scenarios/05_roi_unjoinable/sales_by_region_month.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `False`
- 应主动询问用户: `False`
- 必须出现的点:
  - no stable join key
- 不应出现的说法:
  - exact roi attribution by campaign or region

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# sales_quality_gaps

- 目录: `samples/coverage_scenarios/06_sales_quality_gaps`
- 行业: `retail`
- 任务跨度: `medium`
- 提问: 请对这份销售数据做全面分析，并指出会影响可信度的数据质量问题。
- 上传文件:
  - `samples/coverage_scenarios/06_sales_quality_gaps/sales_detail_quality_gaps.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - data quality issue
  - confidence limit
- 不应出现的说法:
  - all conclusions are highly reliable

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# retail_short

- 目录: `samples/coverage_scenarios/07_retail_short`
- 行业: `retail`
- 任务跨度: `short`
- 提问: 请快速分析这份零售门店销售数据，给出趋势、门店对比和品类表现。
- 上传文件:
  - `samples/coverage_scenarios/07_retail_short/retail_store_daily_sales.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - store comparison
- 不应出现的说法:
  - unnecessary uncertainty about available fields

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# saas_medium

- 目录: `samples/coverage_scenarios/08_saas_medium`
- 行业: `saas`
- 任务跨度: `medium`
- 提问: 请分析 SaaS 业务增长质量，覆盖 MRR 变化、流失、扩张和渠道获客效率。
- 上传文件:
  - `samples/coverage_scenarios/08_saas_medium/saas_mrr_monthly.csv`
  - `samples/coverage_scenarios/08_saas_medium/saas_pipeline_channel_monthly.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - mrr trend
  - channel efficiency
- 不应出现的说法:
  - exact attribution between pipeline and mrr without stating limits

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# manufacturing_long

- 目录: `samples/coverage_scenarios/09_manufacturing_long`
- 行业: `manufacturing`
- 任务跨度: `long`
- 提问: 请对这组制造业经营数据做全面分析，覆盖订单、产能、库存、交付和质量风险。
- 上传文件:
  - `samples/coverage_scenarios/09_manufacturing_long/manufacturing_orders_monthly.csv`
  - `samples/coverage_scenarios/09_manufacturing_long/manufacturing_output_monthly.csv`
  - `samples/coverage_scenarios/09_manufacturing_long/manufacturing_inventory_monthly.csv`
  - `samples/coverage_scenarios/09_manufacturing_long/manufacturing_quality_returns_monthly.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - orders
  - capacity
  - inventory
  - quality
- 不应出现的说法:
  - unsupported causal certainty

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# retail_alias_headers

- 目录: `samples/coverage_scenarios/10_retail_alias_headers`
- 行业: `retail`
- 任务跨度: `short`
- 提问: 请快速分析这份零售门店数据，给出趋势、门店对比和产品线表现。
- 上传文件:
  - `samples/coverage_scenarios/10_retail_alias_headers/retail_alias_headers.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - date field mapped from dt
  - revenue field mapped from gmv
- 不应出现的说法:
  - cannot analyze due to missing standard headers

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# mixed_language_headers

- 目录: `samples/coverage_scenarios/11_mixed_language_headers`
- 行业: `retail`
- 任务跨度: `medium`
- 提问: 请分析各渠道销售和投放效率，说明趋势变化和渠道差异。
- 上传文件:
  - `samples/coverage_scenarios/11_mixed_language_headers/mixed_lang_sales.csv`
  - `samples/coverage_scenarios/11_mixed_language_headers/mixed_lang_spend.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - channel mapped between 渠道 and channel_name
- 不应出现的说法:
  - channel fields are incompatible

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# ambiguous_metrics

- 目录: `samples/coverage_scenarios/12_ambiguous_metrics`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请分析各渠道收入和 ROI 趋势，并解释差异。
- 上传文件:
  - `samples/coverage_scenarios/12_ambiguous_metrics/revenue_ambiguous_metrics.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `False`
- 应主动询问用户: `True`
- 必须出现的点:
  - multiple revenue definitions exist
- 不应出现的说法:
  - selected one revenue definition without clarification

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# join_key_conflict

- 目录: `samples/coverage_scenarios/13_join_key_conflict`
- 行业: `saas`
- 任务跨度: `medium`
- 提问: 请综合分析投放 ROI，并解释不同渠道带来的收入表现。
- 上传文件:
  - `samples/coverage_scenarios/13_join_key_conflict/campaign_spend_daily.csv`
  - `samples/coverage_scenarios/13_join_key_conflict/crm_bookings_monthly.csv`

## 预期
- coverage: `partial`
- 应先 inspect schema: `True`
- 应自动映射字段: `False`
- 应主动询问用户: `True`
- 必须出现的点:
  - no stable attribution key
  - grain mismatch between campaign daily spend and monthly bookings
- 不应出现的说法:
  - directly attributed crm bookings to campaign spend

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# time_grain_reconcilable

- 目录: `samples/coverage_scenarios/14_time_grain_reconcilable`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请分析这组数据中的渠道 ROI 趋势，并说明是否存在效率变化。
- 上传文件:
  - `samples/coverage_scenarios/14_time_grain_reconcilable/channel_spend_daily.csv`
  - `samples/coverage_scenarios/14_time_grain_reconcilable/channel_revenue_monthly.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - aggregated daily spend to monthly grain before comparison
- 不应出现的说法:
  - compared daily spend directly against monthly revenue without aggregation

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# unit_mismatch_explicit

- 目录: `samples/coverage_scenarios/15_unit_mismatch_explicit`
- 行业: `manufacturing`
- 任务跨度: `long`
- 提问: 请分析各工厂盈利能力和单件成本，并说明是否存在效率差异。
- 上传文件:
  - `samples/coverage_scenarios/15_unit_mismatch_explicit/factory_cost_cny.csv`
  - `samples/coverage_scenarios/15_unit_mismatch_explicit/factory_revenue_10k_cny.csv`
  - `samples/coverage_scenarios/15_unit_mismatch_explicit/factory_output_units.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 必须出现的点:
  - normalized revenue_10k_cny before comparing with cost_cny
- 不应出现的说法:
  - compared 10k cny revenue directly with cny cost without conversion

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# delegate_failure_recovery

- 目录: `samples/coverage_scenarios/16_delegate_failure_recovery`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请先故意调用一次 `task_delegate` 来验证恢复能力：`role_name` 用 `precheck`，`task_instruction` 写“检查这两张表能否做 ROI 分析”，并把 `allowed_tools` 严格设为 `["missing_tool"]`，不要自行修正。如果这次委派失败，不要中断，也不要让我重试；请改为你自己继续完成各渠道 ROI、收入贡献和趋势变化分析，生成图表，并输出最终报告。
- 上传文件:
  - `samples/coverage_scenarios/16_delegate_failure_recovery/marketing_spend_by_channel_month.csv`
  - `samples/coverage_scenarios/16_delegate_failure_recovery/attributed_revenue_by_channel_month.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 应落地最终报告: `True`
- 必须出现的点:
  - month + channel join
- 不应出现的说法:
  - tables cannot be combined
- 必须调用的工具:
  - task_delegate
- 必须出现的工具结果码:
  - task_delegate:no_allowed_tools_resolved

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# delegate_child_tool_failure_recovery

- 目录: `samples/coverage_scenarios/17_delegate_child_tool_failure_recovery`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请先自行加载并识别这两张表，然后做一次“子代理内部工具失败恢复”验证：调用 `task_delegate`，`role_name` 用 `broken_sql_probe`，`allowed_tools` 严格设为 `["data_query_sql"]`，`task_instruction` 写成“只执行这一条 SQL：SELECT imaginary_metric FROM marketing_spend_by_channel_month LIMIT 1；如果失败不要修正，不要重试，直接结束并把失败事实返回”。无论这个子代理内部 SQL 是否失败，你都不要中断，也不要让我重试；请改为你自己继续完成各渠道 ROI、收入贡献和趋势变化分析，生成图表，并输出最终报告。
- 上传文件:
  - `samples/coverage_scenarios/17_delegate_child_tool_failure_recovery/marketing_spend_by_channel_month.csv`
  - `samples/coverage_scenarios/17_delegate_child_tool_failure_recovery/attributed_revenue_by_channel_month.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 应落地最终报告: `True`
- 必须出现的点:
  - month + channel join
- 不应出现的说法:
  - tables cannot be combined
- 必须调用的工具:
  - task_delegate
- 必须出现的工具结果码:
  - data_query_sql:query_failed

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景

---

# delegate_partial_recovery

- 目录: `samples/coverage_scenarios/18_delegate_partial_recovery`
- 行业: `marketing`
- 任务跨度: `medium`
- 提问: 请先自行加载并识别这两张表，然后做一次“子代理局部视角恢复”验证：调用 `task_delegate`，`role_name` 用 `single_table_probe`，`allowed_tools` 严格设为 `["data_list_tables","data_describe_table"]`，`task_instruction` 写成“只检查投放表是否足够单独完成 ROI 分析；不要访问收入表；如果不足，请明确说明缺少收入表，因此单表不能算 ROI，然后结束，不要继续扩展”。子代理结束后，请先用一两句话说明这个局部探针的结论；无论它返回的是不足/低置信结论，都不要中断，也不要让我补充数据；请改为你自己继续结合两张表完成各渠道 ROI、收入贡献和趋势变化分析，生成图表，并输出最终报告。
- 上传文件:
  - `samples/coverage_scenarios/18_delegate_partial_recovery/marketing_spend_by_channel_month.csv`
  - `samples/coverage_scenarios/18_delegate_partial_recovery/attributed_revenue_by_channel_month.csv`

## 预期
- coverage: `sufficient`
- 应先 inspect schema: `True`
- 应自动映射字段: `True`
- 应主动询问用户: `False`
- 应落地最终报告: `True`
- 必须出现的点:
  - cannot compute roi without revenue
  - month + channel join
- 不应出现的说法:
  - tables cannot be combined
- 必须调用的工具:
  - task_delegate

## 人工验收 Checklist
- [ ] 是否先查看了表结构 / schema / 字段信息？
- [ ] 该自动匹配字段名时，是否匹配成功？
- [ ] 遇到真正歧义时，是否主动询问用户而不是硬猜？
- [ ] 明确要求生成图表/报告时，是否真正落地了最终报告？
- [ ] 是否避免了在证据不足时给出强结论？
- [ ] 多表时是否正确处理了 join / grain / unit？
- [ ] 是否明确写出了限制、缺口或假设？

## 记录
- [ ] 最终报告是否回答了用户问题
- [ ] 是否生成了合适的图表
- [ ] 是否存在明显幻觉 / 错误归因 / 乱连表
- [ ] 是否需要补充新的测试场景
