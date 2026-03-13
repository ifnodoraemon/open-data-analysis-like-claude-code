package data

import (
	"strings"
)

// JoinCandidate 描述表与表之间的关联关系候选
type JoinCandidate struct {
	LeftTable   string  `json:"leftTable"`
	LeftColumn  string  `json:"leftColumn"`
	RightTable  string  `json:"rightTable"`
	RightColumn string  `json:"rightColumn"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
}

// DiscoverRelations 分析一批表 Schema 提取潜在的多表关联候选
func DiscoverRelations(schemas []SchemaInfo) []JoinCandidate {
	var candidates []JoinCandidate
	n := len(schemas)
	if n < 2 {
		return candidates
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			s1 := schemas[i]
			s2 := schemas[j]

			// 单向测试 s1.cola -> s2.colb 
			// 也同时包含反向对比逻辑，由于我们只需要发现候选所以不用刻意严格定左右表
			cands := compareSchemas(s1, s2)
			candidates = append(candidates, cands...)
		}
	}

	// 依照置信度降序排序
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[i].Confidence < candidates[j].Confidence {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	return candidates
}

func compareSchemas(s1, s2 SchemaInfo) []JoinCandidate {
	var results []JoinCandidate

	baseTable1 := removeExt(s1.TableName)
	baseTable2 := removeExt(s2.TableName)

	for _, c1 := range s1.Columns {
		for _, c2 := range s2.Columns {
			confidence := 0.0
			var reasonParts []string

			n1 := strings.ToLower(c1.Name)
			n2 := strings.ToLower(c2.Name)

			// 排除一些显然不是 Join Key 的高频词汇（单独叫 id 的可能很多，除非特定匹配）
			isGeneric1 := n1 == "id" || n1 == "name" || n1 == "status" || n1 == "type" || n1 == "created_at"
			isGeneric2 := n2 == "id" || n2 == "name" || n2 == "status" || n2 == "type" || n2 == "created_at"

			// --- 规则 1：主外键命名匹配 ---
			// 例如: s1_tableName 有 col "user_id"，s2_tableName "user" 有 col "id"
			if !isGeneric1 && !isGeneric2 {
				// Exact match with non-generic names
				if n1 == n2 && len(n1) > 2 {
					confidence += 0.6
					reasonParts = append(reasonParts, "存在同名字段")
				}
			}

			// FK-like matches
			// table1 is "user", table2 has "user_id"
			if n1 == "id" && n2 == baseTable1+"_id" {
				confidence += 0.9
				reasonParts = append(reasonParts, "表名+_id 主外键命名匹配")
			}
			// table2 is "order", table1 has "order_id"
			if n2 == "id" && n1 == baseTable2+"_id" {
				confidence += 0.9
				reasonParts = append(reasonParts, "表名+_id 主外键命名匹配")
			}

			// --- 规则 2：样本数据交集 ---
			// 如果字段类型基本一致（大概率都是 string），测试重叠率
			if confidence > 0.0 && len(c1.SampleValues) > 0 && len(c2.SampleValues) > 0 {
				overlapCount := 0
				for _, v1 := range c1.SampleValues {
					for _, v2 := range c2.SampleValues {
						if v1 == v2 {
							overlapCount++
							break
						}
					}
				}
				if overlapCount > 0 {
					// 每个重合样本加 0.1 置信分
					extra := float64(overlapCount) * 0.1
					if extra > 0.3 {
						extra = 0.3
					}
					confidence += extra
					reasonParts = append(reasonParts, "样例数据存在重合")
				}
			}

			if confidence >= 0.5 {
				if confidence > 1.0 {
					confidence = 1.0
				}
				results = append(results, JoinCandidate{
					LeftTable:   s1.TableName,
					LeftColumn:  c1.Name,
					RightTable:  s2.TableName,
					RightColumn: c2.Name,
					Confidence:  confidence,
					Reason:      strings.Join(reasonParts, ", "),
				})
			}
		}
	}
	return results
}

func removeExt(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], ".")
	}
	return name
}
