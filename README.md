# 📊 数据分析智能体 (Open Data Analysis)

> 类似 Claude Code 的交互式数据分析 — 上传数据，AI 自主分析，生成带交互图表的专业研报

## 技术栈

| 层 | 技术 |
|---|------|
| **前端** | Vue 3 + Vite + Pinia |
| **后端** | Go (Chi + Gorilla WebSocket) |
| **LLM** | OpenAI / Anthropic 双格式兼容 |
| **数据** | Excel/CSV → SQLite → Text-to-SQL |
| **图表** | ECharts 5 (交互式) |
| **部署** | Docker + Docker Compose |

## Agent 工作流

**我们的 Agent 是动态的（类 Claude Code），不是预定义的固定工作流。**

LLM 拥有 7 个工具，在每一轮自主决定调用哪个工具、以什么参数调用、是否需要继续分析。没有硬编码的步骤顺序 —— Agent 会根据数据的实际情况动态调整策略：

```
┌─────────────────────────────────────────────────────────┐
│  System Prompt (角色定义 + 工具说明 + 分析原则)           │
└────────────────────────┬────────────────────────────────┘
                         │
  用户: "分析这份销售数据"  │  + 文件上下文: [sales.csv]
                         ▼
              ┌──────────────────┐
              │   Agent Loop     │  ← 最多 25 轮
              │  (动态决策)       │
              └────────┬─────────┘
                       │
         LLM 每轮自主选择 ↓
    ┌─────────────────────────────────────┐
    │                                     │
    │   load_data    → 导入数据到 SQLite   │
    │   list_tables  → 查看已有表          │
    │   describe_data→ 查看 Schema+统计    │
    │   query_data   → 执行 SQL 分析       │  ← LLM 动态决定
    │   create_chart → 生成 ECharts 图表   │     调用顺序和参数
    │   write_section→ 撰写研报章节        │
    │   finalize_report → 生成最终研报     │
    │                                     │
    └─────────────┬───────────────────────┘
                  │
                  ▼
        工具结果 → 追加到消息历史
                  │
                  ▼
          LLM 观察结果 → 决定下一步
                  │
          ┌───────┴───────┐
          │               │
     继续调用工具     回复用户(结束)
```

### 动态 vs 固定工作流

| 对比维度 | 固定工作流 (DAG) | 我们的方案 (动态 ReAct) |
|---------|-----------------|----------------------|
| 执行顺序 | 预定义 A→B→C→D | LLM 每轮自主决策 |
| 分支条件 | 硬编码 if/else | LLM 根据观察结果判断 |
| 错误处理 | 预设兜底逻辑 | LLM 读取错误后自主修正 |
| 灵活性 | 低 — 新场景需改代码 | 高 — 自动适应不同数据 |
| 可控性 | 高 — 完全可预测 | 中 — 受 Prompt 约束 |

### 一次典型执行示例

```
轮次  LLM 决策                    工具              结果
──────────────────────────────────────────────────────────
 1    "先加载数据"                load_data         → 表名: sales, 3000行×8列
 2    "看看数据结构"              describe_data     → 列: date/region/product/amount...
 3    "按区域汇总"                query_data        → [{华东:1.2M},{华南:890K}...]
 4    "做个柱状图"                create_chart      → chart_region_sales
 5    "按月看趋势"                query_data        → [{1月:200K},{2月:350K}...]
 6    "做个折线图"                create_chart      → chart_monthly_trend
 7    "写执行摘要"                write_section     → [summary] 已添加
 8    "写区域分析章节"             write_section     → [analysis] + {{chart:chart_region_sales}}
 9    "写趋势分析章节"             write_section     → [analysis] + {{chart:chart_monthly_trend}}
10    "写结论"                    write_section     → [conclusion] 已添加
11    "生成报告"                  finalize_report   → HTML 研报 (含交互图表)
```

> 注意：以上步骤不是硬编码的。如果数据有异常（如大量缺失值），Agent 会自动插入额外的数据质量检查步骤。

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Vue 3 Frontend                           │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────────────┐  │
│  │ InputBar │  │ AgentPanel   │  │ ReportPreview         │  │
│  │ 提示词模板│  │ 思考/工具/结果│  │ iframe + ECharts 渲染  │  │
│  └────┬─────┘  └──────▲───────┘  └──────────▲────────────┘  │
│       │               │                     │               │
│       └───── WebSocket (单例) ──── Pinia Store ──────────────┘
│                    │         ▲
└────────────────────┼─────────┼──────────────────────────────┘
                     │         │ 6 种事件类型
                     ▼         │
┌────────────────────────────────────────────────────────────┐
│                     Go Backend                             │
│  ┌──────────┐  ┌───────────────┐  ┌─────────────────────┐  │
│  │ Chi 路由  │  │ WS Handler    │  │ Upload Handler      │  │
│  │ CORS     │  │ Session UUID  │  │ POST /api/upload    │  │
│  └──────────┘  │ Engine 复用   │  └─────────────────────┘  │
│                └───────┬───────┘                           │
│                        │                                   │
│  ┌─────────────────────▼───────────────────────────────┐   │
│  │              Agent Engine (动态 ReAct 循环)           │   │
│  │  messages[] ←→ LLM API ←→ Tool Registry             │   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        │                                   │
│  ┌─────────────────────▼───────────────────────────────┐   │
│  │  load_data │ describe │ query │ chart │ write │ final│   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        │                                   │
│  ┌─────────────────────▼───────────────────────────────┐   │
│  │   SQLite (ingester.go + schema.go)                   │   │
│  │   CSV/Excel → 批量导入 → SQL 查询引擎                 │   │
│  └─────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────┘
                     │
                     ▼
          OpenAI / Anthropic API
```

## 快速开始

### 本地开发

```bash
# 1. 配置 LLM API
cp server/.env.example server/.env
vim server/.env  # 填入 LLM_PROVIDER / LLM_API_KEY / LLM_MODEL

# 2. 启动后端
cd server && go run main.go

# 3. 启动前端 (新终端)
cd client && npm install && npm run dev

# 4. 打开 http://localhost:5173
```

### Docker 部署

```bash
cp server/.env.example server/.env
vim server/.env
docker compose up -d --build
# 访问 http://localhost
```

## 使用流程

1. 📁 上传 Excel/CSV 数据文件
2. 💬 输入分析需求（或点击快捷提示词）
3. 🧠 左侧面板实时展示 Agent 思考 + 工具调用
4. 📊 右侧面板实时渲染研报（含交互式 ECharts 图表）
5. 📥 点击"导出"下载独立 HTML 研报
6. 💬 可继续追问补充分析（多轮对话）

## 项目结构

```
├── client/                  # Vue 3 前端
│   ├── src/
│   │   ├── components/
│   │   │   ├── agent/       # AgentPanel (消息流)
│   │   │   ├── layout/      # TopNav + InputBar (提示词模板)
│   │   │   └── report/      # ReportPreview (iframe + 导出)
│   │   ├── composables/     # useWebSocket (单例)
│   │   └── stores/          # Pinia 状态管理
│   ├── Dockerfile
│   └── nginx.conf
├── server/                  # Go 后端
│   ├── agent/               # Engine + LLM + Prompts + Types
│   ├── tools/               # 7 个工具 + 研报 HTML 生成
│   ├── data/                # Ingester + Schema (SQLite)
│   ├── handler/             # WebSocket + Upload
│   ├── config/              # .env 配置
│   ├── Dockerfile
│   └── main.go
├── data/                    # 示例数据
├── docker-compose.yml
└── README.md
```

## License

MIT
