package tools

import (
	"fmt"
	"strings"
)

type reportFinalizeParams struct {
	ReportTitle string `json:"report_title"`
	Author      string `json:"author"`
}

type reportFinalizeResult struct {
	ReportTitle string
	Author      string
	BlockCount  int
	ChartCount  int
}

type reportFinalizeBlockedError struct {
	Blockers []string
}

func (e reportFinalizeBlockedError) Error() string {
	return fmt.Sprintf("finalize blocked by %d active branches", len(e.Blockers))
}

type reportFinalizeIssuesError struct {
	Issues []string
}

func (e reportFinalizeIssuesError) Error() string {
	return fmt.Sprintf("finalize blocked by %d report issues", len(e.Issues))
}

type reportAlreadyFinalizedError struct{}

func (e reportAlreadyFinalizedError) Error() string {
	return "report already finalized"
}

func finalizeReportState(state *ReportState, subgoals SubgoalChecker, params reportFinalizeParams, ambiguityChecker AmbiguityChecker) (reportFinalizeResult, error) {
	if state == nil {
		return reportFinalizeResult{}, fmt.Errorf("report state is not initialized")
	}

	if describeReportDeliveryStateLocked(state).IsFinalized {
		return reportFinalizeResult{}, reportAlreadyFinalizedError{}
	}

	params.Author = strings.TrimSpace(params.Author)

	if subgoals != nil {
		canFinalize, blockers := subgoals.CanFinalize()
		if !canFinalize {
			return reportFinalizeResult{}, reportFinalizeBlockedError{Blockers: blockers}
		}
	}

	if issues := reportFinalizeIssues(state); len(issues) > 0 {
		return reportFinalizeResult{}, reportFinalizeIssuesError{Issues: issues}
	}

	if ambiguityChecker != nil {
		blockers, err := ambiguityChecker.CheckAmbiguities()
		if err == nil && len(blockers) > 0 {
			var blockerDescs []string
			for _, b := range blockers {
				blockerDescs = append(blockerDescs, b.Description)
			}
			return reportFinalizeResult{}, reportFinalizeBlockedError{Blockers: blockerDescs}
		}
	}

	state.FinalTitle = params.ReportTitle
	state.FinalAuthor = params.Author
	state.NeedsFinalize = false

	return reportFinalizeResult{
		ReportTitle: params.ReportTitle,
		Author:      params.Author,
		BlockCount:  len(state.Blocks),
		ChartCount:  len(state.Charts),
	}, nil
}
