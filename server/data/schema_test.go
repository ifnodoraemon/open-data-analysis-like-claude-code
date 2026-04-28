package data

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeReadOnlyQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{name: "select", query: "SELECT * FROM sales;", wantErr: false},
		{name: "with", query: "WITH cte AS (SELECT 1 AS n) SELECT * FROM cte", wantErr: false},
		{name: "update", query: "UPDATE sales SET amount = 1", wantErr: true},
		{name: "multi statement", query: "SELECT 1; SELECT 2", wantErr: true},
		{name: "cte delete", query: "WITH gone AS (DELETE FROM sales RETURNING *) SELECT * FROM gone", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeReadOnlyQuery(tc.query)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for query %q", tc.query)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for query %q: %v", tc.query, err)
			}
		})
	}
}

func TestExecuteQueryRejectsOverRowLimit(t *testing.T) {
	t.Parallel()

	db := openTestSQLiteDB(t)
	if _, err := db.Exec(`CREATE TABLE sales (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < queryProbeRows; i++ {
		if _, err := db.Exec(`INSERT INTO sales (name) VALUES (?)`, fmt.Sprintf("row-%d", i)); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	_, err := ExecuteQuery(db, `SELECT id, name FROM sales ORDER BY id`)
	if err == nil {
		t.Fatal("expected row limit error")
	}
}

func TestExecuteQueryReturnsRowsWithinLimit(t *testing.T) {
	t.Parallel()

	db := openTestSQLiteDB(t)
	if _, err := db.Exec(`CREATE TABLE sales (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`INSERT INTO sales (name) VALUES (?)`, fmt.Sprintf("row-%d", i)); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}

	rows, err := ExecuteQuery(db, `SELECT id, name FROM sales ORDER BY id LIMIT 3`)
	if err != nil {
		t.Fatalf("ExecuteQuery returned error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestExecuteQueryRestoresWritableConnection(t *testing.T) {
	t.Parallel()

	db := openTestSQLiteDB(t)
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE sales (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sales (name) VALUES ('row-1')`); err != nil {
		t.Fatalf("insert initial row: %v", err)
	}

	if _, err := ExecuteQuery(db, `SELECT id, name FROM sales ORDER BY id LIMIT 1`); err != nil {
		t.Fatalf("ExecuteQuery returned error: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sales (name) VALUES ('row-2')`); err != nil {
		t.Fatalf("expected database connection to be writable after ExecuteQuery, got %v", err)
	}
}

func TestIngesterInitDBConfiguresSQLite(t *testing.T) {
	t.Parallel()

	ing := NewIngester(t.TempDir())
	if err := ing.InitDB("sess_config"); err != nil {
		t.Fatalf("InitDB returned error: %v", err)
	}
	t.Cleanup(func() {
		if ing.db != nil {
			_ = ing.db.Close()
		}
	})

	var journalMode string
	if err := ing.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("expected WAL journal mode, got %q", journalMode)
	}

	var busyTimeout int
	if err := ing.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout=5000, got %d", busyTimeout)
	}
}

func TestExtractSchemaDetectsTimeCoverage(t *testing.T) {
	t.Parallel()

	db := openTestSQLiteDB(t)
	if _, err := db.Exec(`CREATE TABLE spend (dt TEXT, channel TEXT, ad_spend INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO spend (dt, channel, ad_spend) VALUES
		('2025-01-05','Search',980),
		('2025-01-12','Search',1040),
		('2025-01-19','Search',1120),
		('2025-01-26','Search',1190),
		('2025-01-06','Social',760),
		('2025-01-13','Social',790),
		('2025-01-20','Social',820),
		('2025-01-27','Social',850),
		('2025-02-02','Search',1210),
		('2025-02-09','Search',1260),
		('2025-02-16','Social',870),
		('2025-02-23','Social',910)
	`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	schema, err := ExtractSchema(db, "spend")
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}
	if len(schema.TimeColumns) != 1 {
		t.Fatalf("expected 1 time column, got %#v", schema.TimeColumns)
	}
	timeInfo := schema.TimeColumns[0]
	if timeInfo.Name != "dt" || timeInfo.Grain != "day" {
		t.Fatalf("unexpected time info: %#v", timeInfo)
	}
	if timeInfo.CoverageStart != "2025-01-05" || timeInfo.CoverageEnd != "2025-02-23" {
		t.Fatalf("unexpected coverage: %#v", timeInfo)
	}
	if timeInfo.DistinctPeriodCount != 12 {
		t.Fatalf("expected 12 distinct periods, got %#v", timeInfo)
	}
	if len(timeInfo.RollupGrains) == 0 || timeInfo.RollupGrains[0] != "month" {
		t.Fatalf("expected day grain to roll up to month, got %#v", timeInfo.RollupGrains)
	}

	found := false
	for _, column := range schema.Columns {
		if column.Name != "dt" {
			continue
		}
		found = true
		if column.Type != "TIME" {
			t.Fatalf("expected dt column type TIME, got %#v", column.Type)
		}
		if column.TimeProfile == nil || column.TimeProfile.CoverageEnd != "2025-02-23" {
			t.Fatalf("expected dt column time profile, got %#v", column.TimeProfile)
		}
	}
	if !found {
		t.Fatalf("dt column not found in schema columns: %#v", schema.Columns)
	}
}

func TestExtractSchemaDetectsNegativeNumericStats(t *testing.T) {
	t.Parallel()

	db := openTestSQLiteDB(t)
	if _, err := db.Exec(`CREATE TABLE margins (delta TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO margins (delta) VALUES ('-5'), ('-2'), ('')`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	schema, err := ExtractSchema(db, "margins")
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}
	for _, column := range schema.Columns {
		if column.Name != "delta" {
			continue
		}
		if column.Type != "NUMERIC" {
			t.Fatalf("expected negative-only text column to be numeric, got %#v", column)
		}
		if column.Min == nil || column.Max == nil || *column.Min != -5 || *column.Max != -2 {
			t.Fatalf("unexpected numeric stats: min=%v max=%v", column.Min, column.Max)
		}
		return
	}
	t.Fatalf("delta column not found in schema columns: %#v", schema.Columns)
}

func openTestSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
