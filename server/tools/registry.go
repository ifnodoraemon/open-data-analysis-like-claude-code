package tools

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(args json.RawMessage) (string, error)
}

type StrictTool interface {
	Strict() bool
}

// Registry 工具注册表
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("工具 '%s' 未注册", name)
	}
	return tool, nil
}

// GetOpenAITools 转换为 OpenAI Tools 格式
func (r *Registry) GetOpenAITools() []openai.Tool {
	var oaiTools []openai.Tool
	for _, tool := range r.tools {
		params := tool.Parameters()
		strict := false
		if strictTool, ok := tool.(StrictTool); ok {
			strict = strictTool.Strict()
		}
		oaiTools = append(oaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name(),
				Description: tool.Description(),
				Strict:      strict,
				Parameters:  params,
			},
		})
	}
	return oaiTools
}

// Execute 执行工具
func (r *Registry) Execute(name string, args json.RawMessage) (string, error) {
	tool, err := r.Get(name)
	if err != nil {
		return "", err
	}
	return tool.Execute(args)
}
