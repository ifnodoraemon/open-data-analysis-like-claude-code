package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChartData 图表数据结构
type ChartData struct {
	ID     string          `json:"id"`
	Option json.RawMessage `json:"option"` // ECharts option JSON
	Width  string          `json:"width,omitempty"`
	Height string          `json:"height,omitempty"`
}

type createChartParams struct {
	ChartID    string             `json:"chart_id"`
	Title      string             `json:"title"`
	ChartType  string             `json:"chart_type"`
	Categories []string           `json:"categories"`
	Series     []chartSeriesInput `json:"series"`
	Values     []chartValueInput  `json:"values"`
	Legend     []string           `json:"legend"`
	XAxisName  string             `json:"x_axis_name"`
	YAxisName  string             `json:"y_axis_name"`
	Y2AxisName string             `json:"y2_axis_name"`
}

type chartSeriesInput struct {
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Data   []float64 `json:"data"`
	YAxis  string    `json:"y_axis"`
	Smooth bool      `json:"smooth"`
	Color  string    `json:"color"`
	Stack  string    `json:"stack"`
}

type chartValueInput struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Color string  `json:"color"`
}

// CreateChartTool 创建 ECharts 图表
type CreateChartTool struct {
	ReportState *ReportState
}

func (t *CreateChartTool) Name() string { return "create_chart" }

func (t *CreateChartTool) Strict() bool { return true }

func (t *CreateChartTool) Description() string {
	return `创建一个交互式 ECharts 图表。只允许使用简化 DSL，由后端自动生成稳定的图表配置。

推荐字段：
1. 柱状图/折线图：chart_type + categories + series
2. 饼图：chart_type=pie + values
3. 双轴图：在 series 中给个别序列设置 y_axis="right"

示例：
1. {"chart_id":"chart_sales_trend","title":"销售趋势","chart_type":"line","categories":["1月","2月","3月"],"series":[{"name":"销售额","data":[120,135,160],"smooth":true}]}
2. {"chart_id":"chart_region_revenue","title":"区域销售额","chart_type":"bar","categories":["华东","华北","华南"],"series":[{"name":"收入","data":[320,280,260]}]}
3. {"chart_id":"chart_channel_mix","title":"渠道贡献","chart_type":"pie","values":[{"name":"Search","value":42},{"name":"Social","value":35},{"name":"Affiliate","value":23}]}

注意：
- 必须提供 chart_id 和 title
- 数据必须来自之前的 query_data 结果
- 图表只能写在 analysis 章节中，并用 {{chart:chart_id}} 引用`
}

func (t *CreateChartTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"chart_id": {"type": "string", "description": "图表唯一标识，如 chart_sales_trend"},
			"title": {"type": "string", "description": "图表标题"},
			"chart_type": {"type": "string", "enum": ["bar", "line", "pie"], "description": "优先使用的图表类型"},
			"categories": {"type": "array", "description": "柱状图/折线图的类目轴标签", "items": {"type": "string"}},
			"series": {
				"type": "array",
				"description": "柱状图/折线图的序列定义；data 应来自 query_data 结果",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"properties": {
						"name": {"type": "string"},
						"type": {"type": "string", "enum": ["bar", "line"]},
						"data": {"type": "array", "items": {"type": "number"}},
						"y_axis": {"type": "string", "enum": ["left", "right"]},
						"smooth": {"type": "boolean"},
						"color": {"type": "string"},
						"stack": {"type": "string"}
					},
					"required": ["data"]
				}
			},
			"values": {
				"type": "array",
				"description": "饼图数据",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"properties": {
						"name": {"type": "string"},
						"value": {"type": "number"},
						"color": {"type": "string"}
					},
					"required": ["name", "value"]
				}
			},
			"legend": {"type": "array", "items": {"type": "string"}},
			"x_axis_name": {"type": "string"},
			"y_axis_name": {"type": "string"},
			"y2_axis_name": {"type": "string"}
		},
		"required": ["chart_id", "title"]
	}`)
}

func (t *CreateChartTool) Execute(args json.RawMessage) (string, error) {
	var params createChartParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	option, err := buildOptionFromDSL(params)
	if err != nil {
		return chartValidationFeedback("invalid_chart_spec", params.ChartID, params.Title, "请按 DSL 传入图表定义", err.Error()), nil
	}

	chart := ChartData{
		ID:     params.ChartID,
		Option: option,
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

func buildOptionFromDSL(params createChartParams) (json.RawMessage, error) {
	chartType := strings.ToLower(strings.TrimSpace(params.ChartType))
	if chartType == "" {
		switch {
		case len(params.Values) > 0:
			chartType = "pie"
		case len(params.Series) > 0:
			chartType = firstNonEmptySeriesType(params.Series, "bar")
		}
	}

	switch chartType {
	case "pie":
		return buildPieOption(params)
	case "bar", "line":
		return buildAxisChartOption(params, chartType)
	default:
		return nil, fmt.Errorf("必须提供 chart_type，当前仅支持 bar、line、pie")
	}
}

func buildAxisChartOption(params createChartParams, defaultType string) (json.RawMessage, error) {
	if len(params.Categories) == 0 {
		return nil, fmt.Errorf("柱状图/折线图必须提供 categories")
	}
	if len(params.Series) == 0 {
		return nil, fmt.Errorf("柱状图/折线图必须提供 series")
	}

	legend := append([]string(nil), params.Legend...)
	hasRightAxis := false
	series := make([]map[string]interface{}, 0, len(params.Series))
	for i, item := range params.Series {
		if len(item.Data) == 0 {
			return nil, fmt.Errorf("series[%d].data 不能为空", i)
		}
		if len(item.Data) != len(params.Categories) {
			return nil, fmt.Errorf("series[%d].data 长度必须与 categories 一致", i)
		}

		seriesType := strings.ToLower(strings.TrimSpace(item.Type))
		if seriesType == "" {
			seriesType = defaultType
		}
		if seriesType != "bar" && seriesType != "line" {
			return nil, fmt.Errorf("series[%d].type 仅支持 bar 或 line", i)
		}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			if len(params.Series) == 1 {
				name = params.Title
			} else {
				name = fmt.Sprintf("系列%d", i+1)
			}
		}
		legend = appendIfMissing(legend, name)

		seriesItem := map[string]interface{}{
			"name": name,
			"type": seriesType,
			"data": item.Data,
		}
		if strings.EqualFold(item.YAxis, "right") {
			seriesItem["yAxisIndex"] = 1
			hasRightAxis = true
		}
		if item.Smooth {
			seriesItem["smooth"] = true
		}
		if strings.TrimSpace(item.Stack) != "" {
			seriesItem["stack"] = strings.TrimSpace(item.Stack)
		}
		if strings.TrimSpace(item.Color) != "" {
			seriesItem["itemStyle"] = map[string]interface{}{"color": strings.TrimSpace(item.Color)}
		}
		series = append(series, seriesItem)
	}

	yAxis := []map[string]interface{}{
		{
			"type": "value",
			"name": strings.TrimSpace(params.YAxisName),
		},
	}
	if hasRightAxis {
		rightAxisName := strings.TrimSpace(params.Y2AxisName)
		if rightAxisName == "" {
			rightAxisName = "右轴"
		}
		yAxis = append(yAxis, map[string]interface{}{
			"type": "value",
			"name": rightAxisName,
		})
	}

	option := map[string]interface{}{
		"title": map[string]interface{}{"text": params.Title},
		"tooltip": map[string]interface{}{
			"trigger": "axis",
		},
		"legend": map[string]interface{}{
			"data": legend,
		},
		"grid": map[string]interface{}{
			"left":         "3%",
			"right":        "4%",
			"bottom":       "3%",
			"containLabel": true,
		},
		"xAxis": map[string]interface{}{
			"type": "category",
			"data": params.Categories,
			"name": strings.TrimSpace(params.XAxisName),
		},
		"yAxis":  yAxis,
		"series": series,
	}
	normalized, err := json.Marshal(option)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func buildPieOption(params createChartParams) (json.RawMessage, error) {
	if len(params.Values) == 0 {
		return nil, fmt.Errorf("饼图必须提供 values")
	}

	legend := append([]string(nil), params.Legend...)
	pieData := make([]map[string]interface{}, 0, len(params.Values))
	for i, item := range params.Values {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, fmt.Errorf("values[%d].name 不能为空", i)
		}
		legend = appendIfMissing(legend, name)

		entry := map[string]interface{}{
			"name":  name,
			"value": item.Value,
		}
		if strings.TrimSpace(item.Color) != "" {
			entry["itemStyle"] = map[string]interface{}{"color": strings.TrimSpace(item.Color)}
		}
		pieData = append(pieData, entry)
	}

	option := map[string]interface{}{
		"title": map[string]interface{}{"text": params.Title},
		"tooltip": map[string]interface{}{
			"trigger": "item",
		},
		"legend": map[string]interface{}{
			"data": legend,
		},
		"series": []map[string]interface{}{
			{
				"name":   params.Title,
				"type":   "pie",
				"radius": "55%",
				"data":   pieData,
			},
		},
	}
	normalized, err := json.Marshal(option)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func firstNonEmptySeriesType(series []chartSeriesInput, fallback string) string {
	for _, item := range series {
		if value := strings.ToLower(strings.TrimSpace(item.Type)); value != "" {
			return value
		}
	}
	return fallback
}

func appendIfMissing(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

func chartValidationFeedback(code, chartID, title, message, detail string) string {
	payload := map[string]interface{}{
		"chart_id": chartID,
		"title":    title,
	}
	if detail != "" {
		payload["detail"] = detail
	}
	payload["required_fields"] = []string{"chart_id", "title"}
	payload["supported_shapes"] = []string{
		"chart_type + categories + series",
		"chart_type=pie + values",
	}
	payload["next_action"] = "按 DSL 重新调用 create_chart"
	return toolFailure("create_chart", code, message, payload)
}
