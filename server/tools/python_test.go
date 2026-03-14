package tools

import (
	"encoding/json"
	"testing"
)

func TestFormatPythonResultReturnsStructuredSuccess(t *testing.T) {
	t.Parallel()

	result := formatPythonResult(pyExecResponse{
		Success:    true,
		Stdout:     "42\n",
		Stderr:     "",
		Files:      []string{"plot.png"},
		DurationMs: 123,
	})

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if payload["tool"] != "code_run_python" {
		t.Fatalf("unexpected tool: %#v", payload["tool"])
	}
}

func TestFormatPythonResultReturnsStructuredFailure(t *testing.T) {
	t.Parallel()

	errorText := "NameError"
	result := formatPythonResult(pyExecResponse{
		Success:    false,
		Stdout:     "",
		Stderr:     "Traceback",
		Error:      &errorText,
		Files:      nil,
		DurationMs: 88,
	})

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %#v", payload["ok"])
	}
	if payload["error_code"] != "execution_failed" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
}
