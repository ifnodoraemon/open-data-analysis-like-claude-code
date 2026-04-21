package tools

import (
	"encoding/json"
	"errors"
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

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &CreateChartTool{ReportState: ctx.ReportState, EditState: ctx.EditState}
	})
}

type createChartParams struct {
	ChartID    string             `json:"chart_id"`
	Title      string             `json:"title"`
	Option     json.RawMessage    `json:"option"`
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
	EditState   *ReportEditState
}

func (t *CreateChartTool) Name() string { return "report_create_chart" }

func (t *CreateChartTool) Strict() bool { return true }

func (t *CreateChartTool) Description() string {
	return "创建或更新一个 ECharts 图表。支持简化 DSL 或原生 option，返回 chart_id、chart_ref 与 delivery_state 等事实；会修改 report chart 状态，但不会自动创建或更新正文 block。若要在正文中内联展示图表，可在 markdown/html block 内容里使用 `{{chart:chart_id}}` 占位符。在局部编辑范围存在时，只允许修改被授权的 chart_id。"
}

func (t *CreateChartTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"chart_id": {"type": "string", "description": "图表唯一标识，如 chart_sales_trend"},
			"title": {"type": "string", "description": "图表标题"},
			"option": {"type": "object", "description": "可选。原生 ECharts option；存在时不再按 DSL 推导。"},
			"chart_type": {"type": "string", "enum": ["bar", "line", "pie"], "description": "图表类型。"},
			"categories": {"type": "array", "description": "柱状图/折线图的类目轴标签", "items": {"type": "string"}},
			"series": {
				"type": "array",
				"description": "柱状图/折线图的序列定义；data 应来自 data_query_sql 结果",
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

	t.ReportState.Lock()
	result, err := applyReportChartMutation(t.ReportState, t.EditState, params)
	t.ReportState.Unlock()
	if err != nil {
		var validationErr reportChartValidationError
		if errors.As(err, &validationErr) {
			return chartValidationFeedback("invalid_chart_spec", validationErr.ChartID, validationErr.Title, "图表定义无效", validationErr.Detail), nil
		}
		var scopeErr reportChartScopeError
		if errors.As(err, &scopeErr) {
			return reportEditScopeFailure("report_create_chart", "chart_id", scopeErr.ChartID, "图表", fmt.Sprintf("图表 %s 超出当前局部编辑范围", scopeErr.ChartID), nil), nil
		}
		return "", err
	}

	return reportDraftSuccess("report_create_chart", t.ReportState, map[string]interface{}{
		"chart_id":   result.ChartID,
		"title":      result.Title,
		"chart_ref":  result.ChartRef,
		"ui_summary": fmt.Sprintf("图表 %s 已%s到 report state；delivery_state=draft", result.ChartID, map[bool]string{true: "更新", false: "写入"}[result.Replaced]),
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

func hasRawChartOption(option json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(option))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
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
	return toolFailure("report_create_chart", code, message, payload)
}
