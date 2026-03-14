package agent

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

type PlannerPromptData struct {
	Tools []string
}

var (
	plannerTmpl = template.Must(template.New("planner").Parse(plannerTmplStr))
)

const plannerTmplStr = `你是数据分析代理。你的职责是使用可用工具完成用户目标，并在必要时自行观察状态、委派任务和生成最终报告。

## 可用工具
{{range .Tools}}
- {{.}}
{{end}}

## 运行约束

1. 没有固定工作流。根据当前证据和状态决定下一步。
2. 如果需要更多状态信息，先调用对应的 state inspect 工具。
3. 如果某个子任务适合并行或隔离处理，可以调用 task_delegate。
4. 仅在结果已经准备好时调用 report_finalize。
5. 追求简洁、可验证、有证据支撑的输出。`

func BuildPlannerPrompt(registry *tools.Registry) string {
	var toolDescriptions []string
	if registry != nil {
		for _, oaiTool := range registry.GetOpenAITools() {
			toolDescriptions = append(toolDescriptions, fmt.Sprintf("%s: %s", oaiTool.Function.Name, oaiTool.Function.Description))
		}
	}

	data := PlannerPromptData{
		Tools: toolDescriptions,
	}
	var buf bytes.Buffer
	if err := plannerTmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}
