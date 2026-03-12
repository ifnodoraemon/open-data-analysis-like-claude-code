# Demo Retail Dataset

This sample package is designed for end-to-end testing of the analysis agent.

Files:

- `regional_sales_monthly.csv`: monthly regional sales performance
- `marketing_channel_monthly.csv`: monthly marketing channel performance
- `inventory_snapshot.csv`: latest warehouse inventory snapshot

Recommended upload flow:

1. Upload all three CSV files into one session
2. Ask the agent to load the files and inspect table structures
3. Run a full business analysis report

Suggested prompts:

- "请先加载所有数据表，然后分析 2025 年上半年的销售表现、区域差异和利润变化。"
- "结合营销投放数据，分析各渠道 ROI、转化效率和增长趋势。"
- "结合库存快照，识别缺货风险最高的 SKU，并给出补货优先级建议。"
- "请生成一份完整研报，包含执行摘要、销售趋势、营销 ROI、库存风险和行动建议。"

Notes:

- Data is synthetic but internally consistent enough for demo analysis.
- March performance in the South region is intentionally weak.
- Search and affiliate channels perform better than social in ROI.
- Several SKUs are intentionally close to or below reorder risk levels.
