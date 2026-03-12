package tools

import (
	"encoding/json"
	"fmt"
)

// ChartData 图表数据结构
type ChartData struct {
	ID     string          `json:"id"`
	Option json.RawMessage `json:"option"` // ECharts option JSON
	Width  string          `json:"width,omitempty"`
	Height string          `json:"height,omitempty"`
}

// CreateChartTool 创建 ECharts 图表
type CreateChartTool struct {
	ReportState *ReportState
}

func (t *CreateChartTool) Name() string { return "create_chart" }
func (t *CreateChartTool) Description() string {
	return `创建一个交互式 ECharts 图表。你需要提供完整的 ECharts option 配置。

常用图表类型示例：
1. 柱状图: {"title":{"text":"标题"},"xAxis":{"type":"category","data":["A","B","C"]},"yAxis":{"type":"value"},"series":[{"type":"bar","data":[10,20,30]}]}
2. 折线图: {"title":{"text":"标题"},"xAxis":{"type":"category","data":["Q1","Q2","Q3"]},"yAxis":{"type":"value"},"series":[{"type":"line","data":[100,200,150],"smooth":true}]}
3. 饼图: {"title":{"text":"标题"},"series":[{"type":"pie","data":[{"name":"A","value":40},{"name":"B","value":30}]}]}

注意：
- option 必须是合法的 ECharts option JSON
- 必须包含 title.text 作为图表标题
- 推荐设置 tooltip 和 legend
- 数据应来自之前 query_data 的查询结果
- 每个图表对应一个分析维度，配合 write_section 使用`
}

func (t *CreateChartTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"chart_id": {"type": "string", "description": "图表唯一标识，如 chart_sales_trend"},
			"title": {"type": "string", "description": "图表标题"},
			"option": {"type": "object", "description": "完整的 ECharts option 配置对象"}
		},
		"required": ["chart_id", "title", "option"]
	}`)
}

func (t *CreateChartTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		ChartID string          `json:"chart_id"`
		Title   string          `json:"title"`
		Option  json.RawMessage `json:"option"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	// 验证 option 不为空 — 返回 success result 而非 error，确保 LLM 能看到提示并重试
	if len(params.Option) == 0 || string(params.Option) == "null" || string(params.Option) == "{}" {
		return fmt.Sprintf(`❌ 图表 [%s] 创建失败：缺少 option 参数。

你必须在 option 中提供完整的 ECharts 配置对象（基于 query_data 的查询结果数据）。

请重新调用 create_chart，参数格式如下：
{
  "chart_id": "%s",
  "title": "%s",
  "option": {
    "title": {"text": "%s"},
    "tooltip": {"trigger": "axis"},
    "xAxis": {"type": "category", "data": ["填入实际数据"]},
    "yAxis": {"type": "value"},
    "series": [{"type": "bar", "data": [填入实际数据]}]
  }
}

请根据之前 query_data 返回的数据填写 option 配置。`, params.ChartID, params.ChartID, params.Title, params.Title), nil
	}

	// 验证 option 是合法的 JSON 对象
	var optionCheck map[string]interface{}
	if err := json.Unmarshal(params.Option, &optionCheck); err != nil {
		return fmt.Sprintf("❌ 图表 [%s] 创建失败：option 不是合法的 JSON 对象: %s。请修正 option 后重新调用。", params.ChartID, err.Error()), nil
	}

	chart := ChartData{
		ID:     params.ChartID,
		Option: params.Option,
		Width:  "100%",
		Height: "400px",
	}

	t.ReportState.Charts = append(t.ReportState.Charts, chart)

	return fmt.Sprintf("✅ 图表 [%s] '%s' 已创建成功。可在 write_section 的 content 中使用 {{chart:%s}} 来引用此图表。", params.ChartID, params.Title, params.ChartID), nil
}
