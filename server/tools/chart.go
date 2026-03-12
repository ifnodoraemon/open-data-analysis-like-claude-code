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

	if len(params.Option) == 0 || string(params.Option) == "null" || string(params.Option) == "{}" {
		return chartValidationFeedback("missing_option", params.ChartID, params.Title, "option 必须是非空对象，且数据应来自之前的 query_data 结果", ""), nil
	}

	// 验证 option 是合法的 JSON 对象
	var optionCheck map[string]interface{}
	if err := json.Unmarshal(params.Option, &optionCheck); err != nil {
		return chartValidationFeedback("invalid_option", params.ChartID, params.Title, "option 必须是合法的 JSON 对象", err.Error()), nil
	}

	chart := ChartData{
		ID:     params.ChartID,
		Option: params.Option,
		Width:  "100%",
		Height: "400px",
	}

	t.ReportState.Charts = append(t.ReportState.Charts, chart)

	return toolSuccess("create_chart", map[string]interface{}{
		"chart_id":     params.ChartID,
		"title":        params.Title,
		"chart_ref":    "{{chart:" + params.ChartID + "}}",
		"summary_text": fmt.Sprintf("图表 %s 已创建成功", params.ChartID),
	}), nil
}

func chartValidationFeedback(code, chartID, title, message, detail string) string {
	payload := map[string]interface{}{
		"chart_id": chartID,
		"title":    title,
	}
	if detail != "" {
		payload["detail"] = detail
	}
	payload["required_fields"] = []string{"chart_id", "title", "option"}
	payload["next_action"] = "根据已有查询结果修正参数后再次调用 create_chart"
	return toolFailure("create_chart", code, message, payload)
}
