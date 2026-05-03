package tools

import (
	"sort"
	"strings"
	"sync"
)

type ReportState struct {
	mu                         sync.RWMutex  `json:"-"`
	Blocks                     []ReportBlock `json:"blocks"`
	Charts                     []ChartData   `json:"charts"`
	FinalTitle                 string        `json:"finalTitle,omitempty"`
	FinalAuthor                string        `json:"finalAuthor,omitempty"`
	Layout                     ReportLayout  `json:"layout,omitempty"`
	NeedsFinalize              bool          `json:"needsFinalize,omitempty"`
	FinalizeAttempts           int           `json:"-"`
	LastFinalizeIssueSignature string        `json:"-"`
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
	mu                  sync.RWMutex        `json:"-"`
	Mode                string              `json:"mode,omitempty"`
	TargetRunID         string              `json:"targetRunId,omitempty"`
	TargetBlockID       string              `json:"targetBlockId,omitempty"`
	TargetBlockLabel    string              `json:"targetBlockLabel,omitempty"`
	TargetChartID       string              `json:"targetChartId,omitempty"`
	SelectionText       string              `json:"selectionText,omitempty"`
	SelectionStart      int                 `json:"selectionStart,omitempty"`
	SelectionEnd        int                 `json:"selectionEnd,omitempty"`
	SelectionRangeSet   bool                `json:"selectionRangeSet,omitempty"`
	PreserveOtherBlocks bool                `json:"preserveOtherBlocks,omitempty"`
	AllowedChartIDs     map[string]struct{} `json:"-"`
	TargetBlockContent  string              `json:"-"`
	TargetBlockKind     string              `json:"-"`
	TargetBlockTitle    string              `json:"-"`
	TargetBlockChartID  string              `json:"-"`
	TargetBlockSources  []EvidenceRef       `json:"-"`
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Mode = ""
	s.TargetRunID = ""
	s.TargetBlockID = ""
	s.TargetBlockLabel = ""
	s.TargetChartID = ""
	s.SelectionText = ""
	s.SelectionStart = 0
	s.SelectionEnd = 0
	s.SelectionRangeSet = false
	s.PreserveOtherBlocks = false
	s.AllowedChartIDs = nil
	s.TargetBlockContent = ""
	s.TargetBlockKind = ""
	s.TargetBlockTitle = ""
	s.TargetBlockChartID = ""
	s.TargetBlockSources = nil
}

func (s *ReportEditState) Active() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modeLocked() != ""
}

func (s *ReportEditState) ActiveLocked() bool {
	if s == nil {
		return false
	}
	return s.modeLocked() != ""
}

func (s *ReportEditState) ScopeKind() string {
	if s == nil {
		return "inactive"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.modeLocked() == "" {
		return "inactive"
	}
	return s.scopeKindLocked()
}

func (s *ReportEditState) ScopeKindLocked() string {
	if s == nil {
		return "inactive"
	}
	if s.modeLocked() == "" {
		return "inactive"
	}
	return s.scopeKindLocked()
}

func (s *ReportEditState) modeLocked() string {
	return strings.TrimSpace(s.Mode)
}

func (s *ReportEditState) scopeKindLocked() string {
	mode := strings.ToLower(strings.TrimSpace(s.Mode))
	if mode == "configure_layout" || mode == "revise_layout" {
		return "layout"
	}
	if strings.TrimSpace(s.TargetChartID) != "" {
		return "partial_chart"
	}
	if strings.TrimSpace(s.SelectionText) != "" && (mode == "regenerate_selection" || mode == "revise_selection") {
		return "partial_selection"
	}
	if strings.TrimSpace(s.TargetBlockID) != "" {
		return "partial_block"
	}
	if s.PreserveOtherBlocks {
		return "partial_block"
	}
	return "whole_report"
}

func (s *ReportEditState) Snapshot() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	charts := make([]string, 0, len(s.AllowedChartIDs))
	for chartID := range s.AllowedChartIDs {
		charts = append(charts, chartID)
	}
	sort.Strings(charts)
	return map[string]interface{}{
		"mode":                  s.Mode,
		"target_run_id":         s.TargetRunID,
		"target_block_id":       s.TargetBlockID,
		"target_block_label":    s.TargetBlockLabel,
		"target_chart_id":       s.TargetChartID,
		"selection_text":        s.SelectionText,
		"selection_start":       s.SelectionStart,
		"selection_end":         s.SelectionEnd,
		"selection_range_set":   s.SelectionRangeSet,
		"preserve_other_blocks": s.PreserveOtherBlocks,
		"allowed_chart_ids":     charts,
		"scope_kind":            s.ScopeKindLocked(),
		"active":                s.ActiveLocked(),
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
