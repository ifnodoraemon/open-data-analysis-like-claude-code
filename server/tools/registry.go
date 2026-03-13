package tools

import (
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
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

// ToolContext 提供给工具初始化时的上下文依赖
type ToolContext struct {
	Ingester         *data.Ingester
	ReportState      *ReportState
	FileMaterializer FileMaterializer
	Memory           any // Type: *agent.WorkingMemory
	Subgoals         any // Type: *agent.SubgoalManager
}

// ToolBuilder 是负责动态创建有状态工具的函数
type ToolBuilder func(ctx ToolContext) Tool

var globalToolBuilders []ToolBuilder

// RegisterGlobalTool 用于各个包的 init() 方法向全局注册自己
func RegisterGlobalTool(builder ToolBuilder) {
	globalToolBuilders = append(globalToolBuilders, builder)
}

// LoadGlobalTools 实例化并注册所有在 init 中声明的工具
func (r *Registry) LoadGlobalTools(ctx ToolContext) {
	for _, builder := range globalToolBuilders {
		if tool := builder(ctx); tool != nil {
			r.Register(tool)
		}
	}
}

// NewRegistry 创建工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// CloneFiltered 返回一个只包含指定名称工具的新 Registry
func (r *Registry) CloneFiltered(allowed []string) *Registry {
	filtered := NewRegistry()
	for _, name := range allowed {
		if tool, ok := r.tools[name]; ok {
			filtered.Register(tool)
		}
	}
	return filtered
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

// HasTool 检查工具是否已注册
func (r *Registry) HasTool(name string) bool {
	_, ok := r.tools[name]
	return ok
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
