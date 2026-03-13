package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// SemanticProfile 表示由 LLM 分析给出的一张表的精炼业务语义档案
type SemanticProfile struct {
	TableSummary string `json:"table_summary"`
	Columns      []struct {
		Name             string   `json:"name"`
		BusinessAlias    string   `json:"business_alias"`
		Role             string   `json:"role"`              // e.g., "time", "metric", "dimension", "id"
		CalculationLogic string   `json:"calculation_logic"` // 预测口径或逻辑
		Warnings         []string `json:"warnings"`          // 如：脏数据、意义不明的警告
	} `json:"columns"`
	Relations []struct {
		TargetTable  string `json:"target_table"`
		TargetColumn string `json:"target_column"`
		SourceColumn string `json:"source_column"`
		Reason       string `json:"reason"`
	} `json:"relations"`
}

// AnalyzeTableSemantics 利用大模型分析表结构的小样本并生成语义化档案
func AnalyzeTableSemantics(ctx context.Context, client *openai.Client, schema *SchemaInfo, activeSchemas []SchemaInfo) (*SemanticProfile, error) {
	// 构造待分析内容概要
	schemaBytes, _ := json.MarshalIndent(schema, "", "  ")

	// 构造环境里其它表的简易上下文以找出关联机会
	var contextTables []string
	for _, s := range activeSchemas {
		if s.TableName != schema.TableName {
			contextTables = append(contextTables, s.TableName)
		}
	}
	tablesCtx := "当前会话环境无其他表。"
	if len(contextTables) > 0 {
		tablesCtx = fmt.Sprintf("当前会话环境还有如下表存在，可分析 Join 关系: %s", strings.Join(contextTables, ", "))
	}

	prompt := fmt.Sprintf(`你是资深数据分析师。你需要对以下刚刚抽取的新表 Schema 进行业务语义预分析。
目标表结构和数据样本分布如下：
%s

%s

请分析表并输出 JSON，只能输出标准的 JSON 格式，结构规定如下：
{
  "table_summary": "一句话总结这张表的业务用途",
  "columns": [
    {
      "name": "列原名",
      "business_alias": "业务别名猜想",
      "role": "time | metric | dimension | id",
      "calculation_logic": "对指标的口径猜测或者说明，若无则留空",
      "warnings": ["低置信度或者异常数据提示", ...] 
    }
  ],
  "relations": [
    {
      "target_table": "关联表名",
      "target_column": "关联表字段猜想",
      "source_column": "本表字段",
      "reason": "猜测连接理由"
    }
  ]
}

只需直接返回 JSON 文本，不要用 Markdown 代码块包裹，也不要说其他多余的话。`, string(schemaBytes), tablesCtx)

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are a specialized data semantic profiler outputting only valid JSON.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 分析请求失败: %w", err)
	}

	rawJSON := strings.TrimSpace(resp.Choices[0].Message.Content)
	// 清理可能误带的 markdown 代码快符号
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var profile SemanticProfile
	if err := json.Unmarshal([]byte(rawJSON), &profile); err != nil {
		return nil, fmt.Errorf("解析大模型输出 JSON 失败: %w\nOutput %s", err, rawJSON)
	}

	return &profile, nil
}
