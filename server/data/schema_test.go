package data

import (
	"database/sql"
	"fmt"
	"path/filepath"
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
