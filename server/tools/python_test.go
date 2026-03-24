package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestRunPythonToolExecutionTimeout(t *testing.T) {
	t.Parallel()

	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(7 * time.Second) // wait longer than the client timeout (1 + 5 = 6s)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(fakeHandler)
	t.Cleanup(func() { server.Close() })

	tool := &RunPythonTool{
		MCPEndpoint: server.URL,
	}

	start := time.Now()
	_, err := tool.Execute(json.RawMessage(`{"code": "import time; time.sleep(10)", "timeout": 1}`))
	dur := time.Since(start)

	if err == nil {
		t.Fatal("expected run to fail with a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "Python MCP 服务不可用") {
		t.Fatalf("expected MCP unavailable error, got %v", err)
	}
	if dur > 8*time.Second {
		t.Fatalf("expected tool to time out at around 6s, but it took %v", dur)
	}
}
