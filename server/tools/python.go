package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RunPythonTool 通过 MCP 服务执行 Python 代码
type RunPythonTool struct {
	MCPEndpoint string // Python MCP 服务地址，如 http://python-executor:8081
}

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		// PythonTool 的真正激活在引擎层判断，或在执行时进行 health check
		return &RunPythonTool{MCPEndpoint: ""} // 默认配置，由引擎初始化或读取全局 config
	})
}

func (t *RunPythonTool) Name() string { return "code_run_python" }
func (t *RunPythonTool) Description() string {
	return "Execute Python code in a sandboxed environment and return stdout, stderr, generated files, and duration."
}

func (t *RunPythonTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"code": {"type": "string", "description": "Python code to execute."},
			"timeout": {"type": "integer", "description": "Timeout in seconds, default 30", "default": 30}
		},
		"required": ["code"]
	}`)
}

type pyExecRequest struct {
	Code    string `json:"code"`
	Timeout int    `json:"timeout"`
}

type pyExecResponse struct {
	Success    bool     `json:"success"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	Error      *string  `json:"error"`
	Files      []string `json:"files"`
	DurationMs int      `json:"duration_ms"`
	Truncated  bool     `json:"truncated"`
}

func (t *RunPythonTool) Endpoint() string {
	endpoint := strings.TrimSpace(t.MCPEndpoint)
	if endpoint == "" {
		endpoint = "http://python-executor:8081"
	}
	return strings.TrimRight(endpoint, "/")
}

func (t *RunPythonTool) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.Endpoint()+"/health", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (t *RunPythonTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Code    string `json:"code"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}
	if params.Timeout <= 0 {
		params.Timeout = 30
	}

	endpoint := t.Endpoint()

	reqBody, _ := json.Marshal(pyExecRequest{
		Code:    params.Code,
		Timeout: params.Timeout,
	})

	req, err := http.NewRequest(http.MethodPost, endpoint+"/execute", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if proxyToken := os.Getenv("PROXY_TOKEN"); proxyToken != "" {
		req.Header.Set("X-Proxy-Token", proxyToken)
	}

	client := &http.Client{Timeout: time.Duration(params.Timeout+5) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Python MCP service unavailable: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	var result pyExecResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse Python execution result: %w", err)
	}

	apiBaseURL := strings.TrimRight(os.Getenv("API_BASE_URL"), "/")
	if apiBaseURL == "" {
		apiBaseURL = "http://localhost:8080"
	}
	for i, f := range result.Files {
		result.Files[i] = fmt.Sprintf("%s/api/python-files/%s", apiBaseURL, f)
	}

	return formatPythonResult(result), nil
}

func formatPythonResult(result pyExecResponse) string {
	payload := map[string]interface{}{
		"duration_ms": result.DurationMs,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"files":       result.Files,
	}
	if result.Success {
		payload["ui_summary"] = fmt.Sprintf("Python execution succeeded (%dms)", result.DurationMs)
		return toolSuccess("code_run_python", payload)
	}

	errorText := ""
	if result.Error != nil {
		errorText = strings.TrimSpace(*result.Error)
	}
	if errorText == "" {
		errorText = strings.TrimSpace(result.Stderr)
	}
	payload["detail"] = errorText
	payload["ui_summary"] = fmt.Sprintf("Python execution failed (%dms)", result.DurationMs)
	return toolFailure("code_run_python", "execution_failed", "Python execution failed", payload)
}
