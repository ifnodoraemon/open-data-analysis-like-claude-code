package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SemanticProfile 表示由 LLM 分析给出的一张表的精炼业务语义档案
type SemanticProfile struct {
	TableSummary string              `json:"table_summary"`
	Columns      []SemanticColumn    `json:"columns"`
	Relations    []SemanticRelation  `json:"relations"`
}

// SemanticColumn 列语义档案
type SemanticColumn struct {
	Name             string   `json:"name"`
	BusinessAlias    string   `json:"business_alias"`
	Role             string   `json:"role"`              // e.g., "time", "metric", "dimension", "id"
	CalculationLogic string   `json:"calculation_logic"` // 预测口径或逻辑
	Warnings         []string `json:"warnings"`          // 如：脏数据、意义不明的警告
}

// SemanticRelation 表间关联预测
type SemanticRelation struct {
	TargetTable  string `json:"target_table"`
	TargetColumn string `json:"target_column"`
	SourceColumn string `json:"source_column"`
	Reason       string `json:"reason"`
	Verified     bool   `json:"verified"` // 程序化校验是否通过
}

// LLMChatFunc 通用的 LLM 文本对话函数，由调用方提供，避免直接耦合 OpenAI 包
type LLMChatFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// AnalyzeTableSemantics 利用大模型分析表结构的小样本并生成语义化档案
// 签名改为接收 LLMChatFunc 而非直接依赖 openai.Client
var AnalyzeTableSemantics = func(ctx context.Context, chatFn LLMChatFunc, schema *SchemaInfo, activeTables []string) (*SemanticProfile, error) {
	// 构造待分析内容概要
	schemaBytes, _ := json.MarshalIndent(schema, "", "  ")

	// 构造环境里其它表的简易上下文以找出关联机会
	tablesCtx := "当前会话环境无其他表。"
	if len(activeTables) > 0 {
		tablesCtx = fmt.Sprintf("当前会话环境还有如下表存在，可分析 Join 关系: %s", strings.Join(activeTables, ", "))
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

	systemPrompt := "You are a specialized data semantic profiler outputting only valid JSON."

	rawJSON, err := chatFn(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM 分析请求失败: %w", err)
	}

	rawJSON = strings.TrimSpace(rawJSON)
	// 清理可能误带的 markdown 代码块符号
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

// ValidateRelationHints 对 LLM 返回的 relation hints 做程序化校验
// 只保留通过校验的 relations：
// 1. source_column 必须在当前 schema 中存在
// 2. target_table 必须在 activeTables 中存在
// 3. 如果提供了 targetSchemas，target_column 必须在目标表 schema 中存在
func ValidateRelationHints(
	relations []SemanticRelation,
	schema *SchemaInfo,
	activeTables []string,
	targetSchemas map[string]*SchemaInfo,
) []SemanticRelation {
	if len(relations) == 0 {
		return nil
	}

	// 构建查找集
	sourceColumns := make(map[string]struct{}, len(schema.Columns))
	for _, col := range schema.Columns {
		sourceColumns[strings.ToLower(col.Name)] = struct{}{}
	}

	activeTableSet := make(map[string]struct{}, len(activeTables))
	for _, t := range activeTables {
		activeTableSet[strings.ToLower(t)] = struct{}{}
	}

	verified := make([]SemanticRelation, 0, len(relations))
	for _, rel := range relations {
		// 检查 source_column 是否存在
		if _, ok := sourceColumns[strings.ToLower(rel.SourceColumn)]; !ok {
			continue
		}

		// 检查 target_table 是否在 activeTables 中
		if _, ok := activeTableSet[strings.ToLower(rel.TargetTable)]; !ok {
			continue
		}

		// 如果有目标表的 schema，检查 target_column 是否存在
		if targetSchemas != nil {
			if targetSchema, ok := targetSchemas[strings.ToLower(rel.TargetTable)]; ok {
				found := false
				for _, col := range targetSchema.Columns {
					if strings.EqualFold(col.Name, rel.TargetColumn) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
		}

		rel.Verified = true
		verified = append(verified, rel)
	}

	return verified
}
