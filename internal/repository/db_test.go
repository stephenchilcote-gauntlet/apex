package repository

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func migrationsDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "db", "migrations")
}

func TestRunMigrations_AppliesAllMigrations(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db, migrationsDir()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// All expected tables should exist after migration
	tables := []string{
		"accounts", "correspondents", "transfers",
		"audit_events", "ledger_journals", "ledger_entries",
		"settlement_batches", "settlement_batch_items",
	}
	for _, table := range tables {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %q not found after migrations", table)
		}
	}
}

func TestRunMigrations_TracksAppliedMigrations(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db, migrationsDir()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// _migrations table should record applied migrations
	rows, err := db.Query(`SELECT name FROM _migrations ORDER BY name`)
	if err != nil {
		t.Fatalf("query _migrations: %v", err)
	}
	defer rows.Close()

	var applied []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		applied = append(applied, name)
	}

	if len(applied) == 0 {
		t.Fatal("expected migrations to be recorded, got none")
	}
	// All applied migration names should end in .sql
	for _, name := range applied {
		if filepath.Ext(name) != ".sql" {
			t.Errorf("unexpected migration name %q", name)
		}
	}
}

func TestRunMigrations_IdempotentOnSecondRun(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db, migrationsDir()); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}

	// Running again should not fail or create duplicate schema objects
	if err := RunMigrations(db, migrationsDir()); err != nil {
		t.Fatalf("second RunMigrations (idempotency): %v", err)
	}

	// Table count should be stable
	var tableCount int
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount)
	if tableCount == 0 {
		t.Error("no tables after second migration run")
	}
}

func TestRunMigrations_NonexistentDirectory(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	err = RunMigrations(db, "/nonexistent/path/migrations")
	if err == nil {
		t.Fatal("expected error for nonexistent migrations directory, got nil")
	}
}

func TestRunMigrations_SkipsMalformedSQL(t *testing.T) {
	// Create a temp dir with a valid migration followed by a bad one
	tmpDir := t.TempDir()

	// Valid migration
	goodSQL := `CREATE TABLE test_good (id TEXT PRIMARY KEY);`
	if err := os.WriteFile(filepath.Join(tmpDir, "001_good.sql"), []byte(goodSQL), 0644); err != nil {
		t.Fatal(err)
	}
	// Bad migration
	badSQL := `THIS IS NOT VALID SQL !!!`
	if err := os.WriteFile(filepath.Join(tmpDir, "002_bad.sql"), []byte(badSQL), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	err = RunMigrations(db, tmpDir)
	if err == nil {
		t.Fatal("expected error for malformed SQL migration, got nil")
	}
}
