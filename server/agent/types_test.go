package agent

import (
	"encoding/json"
	"testing"
)

func TestReportEditContextRequiresExplicitSelectionRangeSet(t *testing.T) {
	t.Parallel()

	var withoutFlag ReportEditContext
	if err := json.Unmarshal([]byte(`{"selectionText":"收入","selectionStart":0,"selectionEnd":2}`), &withoutFlag); err != nil {
		t.Fatalf("unmarshal without flag: %v", err)
	}
	if withoutFlag.SelectionRangeSet {
		t.Fatal("expected selectionRangeSet to remain false when the field is omitted")
	}

	var withFlag ReportEditContext
	if err := json.Unmarshal([]byte(`{"selectionText":"收入","selectionStart":0,"selectionEnd":2,"selectionRangeSet":true}`), &withFlag); err != nil {
		t.Fatalf("unmarshal with flag: %v", err)
	}
	if !withFlag.SelectionRangeSet {
		t.Fatal("expected explicit selectionRangeSet=true to be preserved")
	}
}
