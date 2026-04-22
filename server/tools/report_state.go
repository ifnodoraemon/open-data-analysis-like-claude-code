package tools

import (
	"sort"
	"strings"
	"sync"
)

type ReportState struct {
	mu            sync.RWMutex `json:"-"`
	Blocks        []ReportBlock `json:"blocks"`
	Charts        []ChartData   `json:"charts"`
	FinalTitle    string        `json:"finalTitle,omitempty"`
	FinalAuthor   string        `json:"finalAuthor,omitempty"`
	Layout        ReportLayout  `json:"layout,omitempty"`
	NeedsFinalize bool          `json:"needsFinalize,omitempty"`
}

func (s *ReportState) Lock()    { s.mu.Lock() }
func (s *ReportState) Unlock()  { s.mu.Unlock() }
func (s *ReportState) RLock()   { s.mu.RLock() }
func (s *ReportState) RUnlock() { s.mu.RUnlock() }

// EvidenceRef 报告 block 的来源引用，记录结论基于哪次查询/哪张图/哪一步分析
type EvidenceRef struct {
	Kind      string `json:"kind"` // sql | chart | table | python | tool_result
	ToolName  string `json:"tool_name,omitempty"`
	SQL       string `json:"sql,omitempty"`
	TableName string `json:"table_name,omitempty"`
	ChartID   string `json:"chart_id,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type ReportBlock struct {
	ID      string        `json:"id"`
	Kind    string        `json:"kind"`
	Title   string        `json:"title,omitempty"`
	Content string        `json:"content,omitempty"`
	ChartID string        `json:"chartId,omitempty"`
	Sources []EvidenceRef `json:"sources,omitempty"`
}

type ReportLayout struct {
	CustomCSS string `json:"customCss,omitempty"`
	BodyClass string `json:"bodyClass,omitempty"`
}

type ReportEditState struct {
	Mode                string              `json:"mode,omitempty"`
	TargetRunID         string              `json:"targetRunId,omitempty"`
	TargetBlockID       string              `json:"targetBlockId,omitempty"`
	SelectionText       string              `json:"selectionText,omitempty"`
	PreserveOtherBlocks bool                `json:"preserveOtherBlocks,omitempty"`
	AllowedChartIDs     map[string]struct{} `json:"-"`
}

type ReportDeliveryState struct {
	HasContent    bool   `json:"has_content"`
	IsFinalized   bool   `json:"is_finalized"`
	NeedsFinalize bool   `json:"needs_finalize"`
	DeliveryState string `json:"delivery_state"`
	BlockCount    int    `json:"block_count"`
	ChartCount    int    `json:"chart_count"`
	FinalTitle    string `json:"final_title,omitempty"`
	FinalAuthor   string `json:"final_author,omitempty"`
}

func (s *ReportEditState) Reset() {
	if s == nil {
		return
	}
	s.Mode = ""
	s.TargetRunID = ""
	s.TargetBlockID = ""
	s.SelectionText = ""
	s.PreserveOtherBlocks = false
	s.AllowedChartIDs = nil
}

func (s *ReportEditState) Active() bool {
	return s != nil && strings.TrimSpace(s.Mode) != ""
}

func (s *ReportEditState) Snapshot() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{}
	}
	charts := make([]string, 0, len(s.AllowedChartIDs))
	for chartID := range s.AllowedChartIDs {
		charts = append(charts, chartID)
	}
	sort.Strings(charts)
	return map[string]interface{}{
		"mode":                  s.Mode,
		"target_run_id":         s.TargetRunID,
		"target_block_id":       s.TargetBlockID,
		"selection_text":        s.SelectionText,
		"preserve_other_blocks": s.PreserveOtherBlocks,
		"allowed_chart_ids":     charts,
		"active":                s.Active(),
	}
}

func DescribeReportDeliveryState(state *ReportState) ReportDeliveryState {
	if state == nil {
		return ReportDeliveryState{DeliveryState: "empty"}
	}
	state.RLock()
	defer state.RUnlock()
	return describeReportDeliveryStateLocked(state)
}

func DescribeReportDeliveryStateLocked(state *ReportState) ReportDeliveryState {
	if state == nil {
		return ReportDeliveryState{DeliveryState: "empty"}
	}
	return describeReportDeliveryStateLocked(state)
}

func describeReportDeliveryStateLocked(state *ReportState) ReportDeliveryState {
	delivery := ReportDeliveryState{
		DeliveryState: "empty",
	}
	delivery.BlockCount = len(state.Blocks)
	delivery.ChartCount = len(state.Charts)
	delivery.FinalTitle = strings.TrimSpace(state.FinalTitle)
	delivery.FinalAuthor = strings.TrimSpace(state.FinalAuthor)
	delivery.HasContent = delivery.BlockCount > 0 || delivery.ChartCount > 0
	delivery.NeedsFinalize = state.NeedsFinalize
	delivery.IsFinalized = delivery.HasContent && !state.NeedsFinalize && delivery.FinalTitle != ""
	if delivery.HasContent {
		if delivery.IsFinalized {
			delivery.DeliveryState = "finalized"
		} else {
			delivery.DeliveryState = "draft"
		}
	}
	return delivery
}
