package tools

import (
	"fmt"
	"strings"
)

type reportLayoutParams struct {
	Action    string `json:"action"`
	CustomCSS string `json:"custom_css"`
	BodyClass string `json:"body_class"`
}

type reportLayoutResult struct {
	Action       string
	HasCustomCSS bool
	BodyClass    string
	UISummary    string
}

const maxCustomCSSSize = 10240

func applyReportLayoutMutation(state *ReportState, params reportLayoutParams) (reportLayoutResult, error) {
	if state == nil {
		return reportLayoutResult{}, fmt.Errorf("report state is not initialized")
	}

	action := strings.TrimSpace(params.Action)
	if action == "" {
		action = "merge"
	}
	params.Action = action

	switch action {
	case "reset":
		state.Layout = ReportLayout{}
		state.NeedsFinalize = true
		return reportLayoutResult{
			Action:    action,
			UISummary: "已重置报告布局；delivery_state=draft",
		}, nil
	case "merge":
		if params.CustomCSS != "" {
			if len(params.CustomCSS) > maxCustomCSSSize {
				return reportLayoutResult{}, fmt.Errorf("custom_css exceeds maximum allowed size (%d bytes)", maxCustomCSSSize)
			}
			state.Layout.CustomCSS = params.CustomCSS
		}
		if params.BodyClass != "" {
			state.Layout.BodyClass = strings.TrimSpace(params.BodyClass)
		}
		state.NeedsFinalize = true
		return reportLayoutResult{
			Action:       action,
			HasCustomCSS: state.Layout.CustomCSS != "",
			BodyClass:    state.Layout.BodyClass,
			UISummary:    "已更新报告布局配置；delivery_state=draft",
		}, nil
	default:
		return reportLayoutResult{}, fmt.Errorf("unknown action: %s", action)
	}
}
