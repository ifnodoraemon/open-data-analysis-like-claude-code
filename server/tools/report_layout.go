package tools

import (
	"fmt"
	"strings"
)

type reportLayoutParams struct {
	Action          string `json:"action"`
	CustomHTMLShell string `json:"custom_html_shell"`
	CustomCSS       string `json:"custom_css"`
	CustomJS        string `json:"custom_js"`
	BodyClass       string `json:"body_class"`
	HideCover       *bool  `json:"hide_cover"`
	HideTOC         *bool  `json:"hide_toc"`
}

type reportLayoutResult struct {
	Action       string
	HasCustomCSS bool
	BodyClass    string
	HideCover    bool
	HideTOC      bool
	UISummary    string
}

type reportLayoutUnsafeError struct {
	Action string
}

func (e reportLayoutUnsafeError) Error() string {
	return fmt.Sprintf("unsafe layout option for %s", e.Action)
}

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
			UISummary: "已恢复默认报告模板，当前仍是报告草稿",
		}, nil
	case "merge":
		if strings.TrimSpace(params.CustomHTMLShell) != "" || strings.TrimSpace(params.CustomJS) != "" {
			return reportLayoutResult{}, reportLayoutUnsafeError{Action: action}
		}
		if params.CustomCSS != "" {
			state.Layout.CustomCSS = params.CustomCSS
		}
		if params.BodyClass != "" {
			state.Layout.BodyClass = strings.TrimSpace(params.BodyClass)
		}
		if params.HideCover != nil {
			state.Layout.HideCover = *params.HideCover
		}
		if params.HideTOC != nil {
			state.Layout.HideTOC = *params.HideTOC
		}
		state.NeedsFinalize = true
		return reportLayoutResult{
			Action:       action,
			HasCustomCSS: state.Layout.CustomCSS != "",
			BodyClass:    state.Layout.BodyClass,
			HideCover:    state.Layout.HideCover,
			HideTOC:      state.Layout.HideTOC,
			UISummary:    "已更新报告布局配置，当前仍是报告草稿",
		}, nil
	default:
		return reportLayoutResult{}, fmt.Errorf("unknown action: %s", action)
	}
}
