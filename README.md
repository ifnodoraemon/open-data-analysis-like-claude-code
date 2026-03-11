# 📊 数据分析智能体 (Data Analysis Agent)

> 类似 Claude Code 的交互体验 — 左侧 Agent 执行面板 + 右侧研报实时渲染

## 技术栈

- **后端**: Go (Chi + Gorilla WebSocket + go-openai + SQLite)
- **前端**: Vite + Vue 3 + Pinia
- **大数据策略**: Excel → SQLite → LLM 生成 SQL 查询

## 快速开始

### 1. 配置 API Key

```bash
cp server/.env.example server/.env
# 编辑 .env，填入 LLM_BASE_URL / LLM_API_KEY / LLM_MODEL
```

### 2. 启动后端

```bash
cd server
go run main.go
```

### 3. 启动前端

```bash
cd client
npm install
npm run dev
```

### 4. 打开浏览器

访问 http://localhost:5173

## 使用流程

1. 上传 Excel/CSV 数据文件
2. 输入分析需求（如："分析销售数据的区域分布和趋势"）
3. 左侧面板实时展示 Agent 执行过程
4. 右侧面板实时渲染研究报告
5. 点击 "导出" 下载独立 HTML 研报

## 架构

```
用户上传 Excel → 导入 SQLite → LLM 写 SQL 查询 → 分析结果 → 生成 HTML 研报
```

## License

MIT
