package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGenerateSchemaMetadata_NoLLM_StillWorks(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_schema_only"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	// 创建并填充测试表
	if _, err := ing.db.Exec(`CREATE TABLE sales (month TEXT, revenue INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO sales (month, revenue) VALUES ('2025-01', 100), ('2025-02', 120)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// 不配置 LLM，确定性 metadata 应正常落库
	if err := ing.GenerateSchemaMetadata("sales"); err != nil {
		t.Fatalf("GenerateSchemaMetadata failed: %v", err)
	}

	// 验证 schema_ready = true
	schemaReady, semanticReady, relationsVerified := ing.GetMetadataReadiness("sales")
	if !schemaReady {
		t.Fatal("expected schema_ready=true after GenerateSchemaMetadata")
	}
	if semanticReady {
		t.Fatal("expected semantic_ready=false without LLM enrichment")
	}
	if relationsVerified {
		t.Fatal("expected relations_verified=false without LLM enrichment")
	}

	// 验证 GetTableMetadata 可以返回 schema JSON
	metaJSON, err := ing.GetTableMetadata("sales")
	if err != nil {
		t.Fatalf("GetTableMetadata failed: %v", err)
	}
	if metaJSON == "" {
		t.Fatal("expected non-empty schema metadata")
	}

	// 反序列化验证 schema 内容
	var schema SchemaInfo
	if err := json.Unmarshal([]byte(metaJSON), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if schema.TableName != "sales" || schema.RowCount != 2 || len(schema.Columns) != 2 {
		t.Fatalf("unexpected schema: %+v", schema)
	}
}

func TestEnrichSemanticProfile_WithMockLLM(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_enrich"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	if _, err := ing.db.Exec(`CREATE TABLE orders (order_id TEXT, amount REAL, channel TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO orders VALUES ('o1', 99.5, 'web'), ('o2', 150.0, 'app')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// 先生成确定性 metadata
	if err := ing.GenerateSchemaMetadata("orders"); err != nil {
		t.Fatalf("GenerateSchemaMetadata: %v", err)
	}

	// Mock LLM chatFn
	mockChatFn := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		return `{
			"table_summary": "订单数据表",
			"columns": [
				{"name": "order_id", "business_alias": "订单编号", "role": "id", "calculation_logic": "", "warnings": []},
				{"name": "amount", "business_alias": "订单金额", "role": "metric", "calculation_logic": "订单总额", "warnings": []},
				{"name": "channel", "business_alias": "渠道", "role": "dimension", "calculation_logic": "", "warnings": []}
			],
			"relations": []
		}`, nil
	}

	if err := ing.EnrichSemanticProfile(context.Background(), "orders", mockChatFn); err != nil {
		t.Fatalf("EnrichSemanticProfile: %v", err)
	}

	// 验证 readiness
	schemaReady, semanticReady, _ := ing.GetMetadataReadiness("orders")
	if !schemaReady {
		t.Fatal("expected schema_ready=true")
	}
	if !semanticReady {
		t.Fatal("expected semantic_ready=true after enrichment")
	}

	// 验证完整 metadata
	meta, err := ing.GetFullTableMetadata("orders")
	if err != nil {
		t.Fatalf("GetFullTableMetadata: %v", err)
	}
	if meta.SchemaJSON == "" {
		t.Fatal("expected non-empty schema_json")
	}
	if meta.SemanticJSON == "" {
		t.Fatal("expected non-empty semantic_json")
	}

	var profile SemanticProfile
	if err := json.Unmarshal([]byte(meta.SemanticJSON), &profile); err != nil {
		t.Fatalf("unmarshal semantic: %v", err)
	}
	if profile.TableSummary != "订单数据表" {
		t.Fatalf("unexpected table_summary: %s", profile.TableSummary)
	}
}

func TestEnrichSemanticProfile_LLMFails_SchemaStillAvailable(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_llm_fail"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	if _, err := ing.db.Exec(`CREATE TABLE metrics (dt TEXT, value REAL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO metrics VALUES ('2025-01', 42.0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// 先生成确定性 metadata
	if err := ing.GenerateSchemaMetadata("metrics"); err != nil {
		t.Fatalf("GenerateSchemaMetadata: %v", err)
	}

	// LLM 失败
	failingChatFn := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		return "", context.DeadlineExceeded
	}

	err := ing.EnrichSemanticProfile(context.Background(), "metrics", failingChatFn)
	if err == nil {
		t.Fatal("expected EnrichSemanticProfile to return error on LLM failure")
	}

	// 确定性 schema 仍然可用
	schemaReady, semanticReady, _ := ing.GetMetadataReadiness("metrics")
	if !schemaReady {
		t.Fatal("schema should still be ready after LLM failure")
	}
	if semanticReady {
		t.Fatal("semantic should not be ready after LLM failure")
	}

	metaJSON, err := ing.GetTableMetadata("metrics")
	if err != nil {
		t.Fatalf("GetTableMetadata should still work: %v", err)
	}
	if metaJSON == "" {
		t.Fatal("schema metadata should still be available")
	}
}

func TestValidateRelationHints_FiltersInvalid(t *testing.T) {
	t.Parallel()

	schema := &SchemaInfo{
		TableName: "orders",
		Columns: []ColumnInfo{
			{Name: "user_id"},
			{Name: "product_id"},
			{Name: "amount"},
		},
	}

	activeTables := []string{"users", "products"}

	targetSchemas := map[string]*SchemaInfo{
		"users": {
			Columns: []ColumnInfo{
				{Name: "id"},
				{Name: "name"},
			},
		},
		"products": {
			Columns: []ColumnInfo{
				{Name: "id"},
				{Name: "title"},
			},
		},
	}

	relations := []SemanticRelation{
		// 有效: source_column=user_id 在 schema 中, target=users 在 activeTables 中, target_column=id 在 users schema 中
		{TargetTable: "users", TargetColumn: "id", SourceColumn: "user_id", Reason: "用户关联"},
		// 有效: source_column=product_id 在 schema 中, target=products 在 activeTables 中, target_column=id 在 products schema 中
		{TargetTable: "products", TargetColumn: "id", SourceColumn: "product_id", Reason: "产品关联"},
		// 无效: source_column=store_id 不在 schema 中
		{TargetTable: "stores", TargetColumn: "id", SourceColumn: "store_id", Reason: "门店关联"},
		// 无效: target_table=categories 不在 activeTables 中
		{TargetTable: "categories", TargetColumn: "id", SourceColumn: "user_id", Reason: "分类关联"},
		// 无效: target_column=email 不在 users schema 中
		{TargetTable: "users", TargetColumn: "email", SourceColumn: "user_id", Reason: "错误的列"},
	}

	verified := ValidateRelationHints(relations, schema, activeTables, targetSchemas)

	if len(verified) != 2 {
		t.Fatalf("expected 2 verified relations, got %d: %+v", len(verified), verified)
	}

	for _, rel := range verified {
		if !rel.Verified {
			t.Fatalf("expected verified=true for relation %+v", rel)
		}
	}

	if verified[0].TargetTable != "users" || verified[1].TargetTable != "products" {
		t.Fatalf("unexpected verified relations: %+v", verified)
	}
}

func TestValidateRelationHints_NoTargetSchemas(t *testing.T) {
	t.Parallel()

	schema := &SchemaInfo{
		TableName: "orders",
		Columns: []ColumnInfo{
			{Name: "user_id"},
		},
	}

	activeTables := []string{"users"}

	relations := []SemanticRelation{
		{TargetTable: "users", TargetColumn: "id", SourceColumn: "user_id", Reason: "用户关联"},
	}

	// 没有目标表 schema 时，只检查 source_column 和 target_table
	verified := ValidateRelationHints(relations, schema, activeTables, nil)

	if len(verified) != 1 {
		t.Fatalf("expected 1 verified relation without target schemas, got %d", len(verified))
	}
}

func TestGetActiveTables(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_active_tables"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	// 创建两张表
	if _, err := ing.db.Exec(`CREATE TABLE t1 (id INTEGER)`); err != nil {
		t.Fatalf("create t1: %v", err)
	}
	if _, err := ing.db.Exec(`CREATE TABLE t2 (id INTEGER)`); err != nil {
		t.Fatalf("create t2: %v", err)
	}

	// 生成 metadata
	if err := ing.GenerateSchemaMetadata("t1"); err != nil {
		t.Fatalf("GenerateSchemaMetadata t1: %v", err)
	}
	if err := ing.GenerateSchemaMetadata("t2"); err != nil {
		t.Fatalf("GenerateSchemaMetadata t2: %v", err)
	}

	tables := ing.GetActiveTables()
	if len(tables) != 2 {
		t.Fatalf("expected 2 active tables, got %d", len(tables))
	}
}

func TestReimport_ClearsStaleSemanticState(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_reimport"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	// 第一次导入 + enrichment
	if _, err := ing.db.Exec(`CREATE TABLE sales (month TEXT, revenue INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO sales VALUES ('2025-01', 100)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := ing.GenerateSchemaMetadata("sales"); err != nil {
		t.Fatalf("GenerateSchemaMetadata: %v", err)
	}
	mockChatFn := func(ctx context.Context, sys, usr string) (string, error) {
		return `{"table_summary":"既有分析","columns":[],"relations":[]}`, nil
	}
	if err := ing.EnrichSemanticProfile(context.Background(), "sales", mockChatFn); err != nil {
		t.Fatalf("EnrichSemanticProfile: %v", err)
	}

	// 验证 semantic 已就绪
	_, semanticReady, _ := ing.GetMetadataReadiness("sales")
	if !semanticReady {
		t.Fatal("expected semantic_ready=true after enrichment")
	}

	// 模拟重新导入：schema 变化 → 重新调用 GenerateSchemaMetadata
	if _, err := ing.db.Exec(`DROP TABLE sales`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if _, err := ing.db.Exec(`CREATE TABLE sales (month TEXT, revenue INTEGER, cost INTEGER)`); err != nil {
		t.Fatalf("recreate table: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO sales VALUES ('2025-01', 100, 50)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := ing.GenerateSchemaMetadata("sales"); err != nil {
		t.Fatalf("GenerateSchemaMetadata (reimport): %v", err)
	}

	// 已失效的 semantic 状态应被清除
	schemaReady, semanticReady, relationsVerified := ing.GetMetadataReadiness("sales")
	if !schemaReady {
		t.Fatal("expected schema_ready=true after reimport")
	}
	if semanticReady {
		t.Fatal("expected semantic_ready=false after reimport (stale data cleared)")
	}
	if relationsVerified {
		t.Fatal("expected relations_verified=false after reimport (stale data cleared)")
	}

	meta, err := ing.GetFullTableMetadata("sales")
	if err != nil {
		t.Fatalf("GetFullTableMetadata: %v", err)
	}
	if meta.SemanticJSON != "" {
		t.Fatalf("expected empty semantic_json after reimport, got: %s", meta.SemanticJSON)
	}
}

func TestEnrichSemanticProfile_StripsUnverifiedRelations(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_strip_rels"); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	// 创建 orders 表（有 user_id 列）
	if _, err := ing.db.Exec(`CREATE TABLE orders (user_id TEXT, amount REAL)`); err != nil {
		t.Fatalf("create orders: %v", err)
	}
	if _, err := ing.db.Exec(`INSERT INTO orders VALUES ('u1', 50.0)`); err != nil {
		t.Fatalf("insert orders: %v", err)
	}
	if err := ing.GenerateSchemaMetadata("orders"); err != nil {
		t.Fatalf("GenerateSchemaMetadata orders: %v", err)
	}

	// 注意：不创建 "stores" 表，所以指向 "stores" 的 relation 应被过滤掉
	mockChatFn := func(ctx context.Context, sys, usr string) (string, error) {
		return `{
			"table_summary": "订单表",
			"columns": [
				{"name": "user_id", "business_alias": "用户ID", "role": "id", "calculation_logic": "", "warnings": []},
				{"name": "amount", "business_alias": "金额", "role": "metric", "calculation_logic": "", "warnings": []}
			],
			"relations": [
				{"target_table": "stores", "target_column": "id", "source_column": "user_id", "reason": "不存在的表"}
			]
		}`, nil
	}

	if err := ing.EnrichSemanticProfile(context.Background(), "orders", mockChatFn); err != nil {
		t.Fatalf("EnrichSemanticProfile: %v", err)
	}

	// 验证 semantic_json 中也不含未校验的 relation
	meta, err := ing.GetFullTableMetadata("orders")
	if err != nil {
		t.Fatalf("GetFullTableMetadata: %v", err)
	}
	var profile SemanticProfile
	if err := json.Unmarshal([]byte(meta.SemanticJSON), &profile); err != nil {
		t.Fatalf("unmarshal semantic_json: %v", err)
	}
	if len(profile.Relations) != 0 {
		t.Fatalf("expected 0 relations in semantic_json (unverified stripped), got %d: %+v", len(profile.Relations), profile.Relations)
	}

	// relations_json 也应为空数组或 null
	if meta.RelationsJSON != "" && meta.RelationsJSON != "null" && meta.RelationsJSON != "[]" {
		t.Fatalf("expected empty relations_json, got: %s", meta.RelationsJSON)
	}
}

func TestInferColumnTypesDetectsIntegerRealTextAndEmpty(t *testing.T) {
	t.Parallel()

	rows := [][]string{
		{"1", "1.5", "abc", ""},
		{"2", "2.75", "def", ""},
		{"3", "", "ghi", "N/A"},
	}
	got := inferColumnTypes(rows, 4)
	want := []ColumnType{TypeInteger, TypeReal, TypeText, TypeText}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inferColumnTypes() = %#v, want %#v", got, want)
	}
}

func openTestSQLiteDB2(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
