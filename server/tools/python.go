package tools

import (
	"bytes"
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

func (t *RunPythonTool) Name() string { return "run_python" }
func (t *RunPythonTool) Description() string {
	return `在安全的 Python 沙箱环境中执行代码。预装了 pandas, numpy, matplotlib, scipy, scikit-learn。

适用场景:
- 复杂统计分析（回归、聚类、假设检验）
- 数据清洗和转换（pandas DataFrame 操作）
- 高级可视化（matplotlib 图表保存到文件）
- 机器学习建模（scikit-learn）
- 自定义计算逻辑

注意:
- 代码中 print() 的输出会作为结果返回
- 图表请用 plt.savefig(WORK_DIR / 'chart_name.png') 保存
- 数据文件在 /app/data/ 目录下
- 超时时间默认 30 秒
- 最终结果请用 print() 输出，不要只赋值给变量`
}

func (t *RunPythonTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"code": {"type": "string", "description": "要执行的 Python 代码"},
			"timeout": {"type": "integer", "description": "超时时间（秒），默认 30", "default": 30}
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
}

func (t *RunPythonTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Code    string `json:"code"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if params.Timeout <= 0 {
		params.Timeout = 30
	}

	endpoint := t.MCPEndpoint
	if endpoint == "" {
		endpoint = os.Getenv("PYTHON_MCP_URL")
	}
	if endpoint == "" {
		endpoint = "http://python-executor:8081"
	}

	// 调用 Python MCP 服务
	reqBody, _ := json.Marshal(pyExecRequest{
		Code:    params.Code,
		Timeout: params.Timeout,
	})

	client := &http.Client{Timeout: time.Duration(params.Timeout+5) * time.Second}
	resp, err := client.Post(endpoint+"/execute", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("Python MCP 服务不可用: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result pyExecResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 Python 执行结果失败: %w", err)
	}

	// 格式化输出
	var output strings.Builder
	if result.Success {
		output.WriteString("✅ 执行成功")
	} else {
		output.WriteString("❌ 执行失败")
	}
	output.WriteString(fmt.Sprintf(" (%dms)\n", result.DurationMs))

	if result.Stdout != "" {
		output.WriteString("\n📤 输出:\n")
		output.WriteString(result.Stdout)
	}

	if result.Error != nil && *result.Error != "" {
		output.WriteString("\n⚠️ 错误:\n")
		output.WriteString(*result.Error)
	}

	if len(result.Files) > 0 {
		output.WriteString(fmt.Sprintf("\n📁 生成文件: %s", strings.Join(result.Files, ", ")))
	}

	return output.String(), nil
}
