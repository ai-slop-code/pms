package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
	"pms/backend/internal/store"
)

func TestOpenMigrationDatabaseSelectsReadOnlyForPlannerAndWritableForApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	dsn := "sqlite://" + path
	setup, err := dbconn.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(setup); err != nil {
		t.Fatal(err)
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoSQLiteSidecars(t, path)

	dryRunDB, err := openMigrationDatabase(dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	report, err := (&store.Store{DB: dryRunDB}).PlanPMS21Migration(context.Background(), 1)
	if err != nil {
		t.Fatalf("planner query through read-only connection: %v", err)
	}
	if report.Mode != "dry_run" {
		t.Fatalf("report mode = %q, want dry_run", report.Mode)
	}
	if _, err := dryRunDB.Exec(`CREATE TABLE should_not_exist (id INTEGER)`); err == nil {
		t.Fatal("dry-run connection unexpectedly allowed mutation")
	}
	if err := dryRunDB.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoSQLiteSidecars(t, path)

	applyDB, err := openMigrationDatabase(dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer applyDB.Close()
	if _, err := applyDB.Exec(`CREATE TABLE apply_write_check (id INTEGER)`); err != nil {
		t.Fatalf("apply connection was not writable: %v", err)
	}
}

func assertNoSQLiteSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-journal", "-shm", "-wal"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("SQLite sidecar %s exists (stat error: %v)", suffix, err)
		}
	}
}

func TestAbsoluteSQLitePath(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "pms.db")
	for _, databaseURL := range []string{abs, "sqlite://" + abs, "sqlite://" + abs + "?cache=shared"} {
		got, err := absoluteSQLitePath(databaseURL)
		if err != nil {
			t.Fatalf("absoluteSQLitePath(%q): %v", databaseURL, err)
		}
		if got != abs {
			t.Fatalf("absoluteSQLitePath(%q) = %q, want %q", databaseURL, got, abs)
		}
	}
	for _, databaseURL := range []string{"", ":memory:", "relative.db", "sqlite://relative.db"} {
		if _, err := absoluteSQLitePath(databaseURL); err == nil {
			t.Fatalf("absoluteSQLitePath(%q) unexpectedly succeeded", databaseURL)
		}
	}
}

func TestPrepareMigrationDatabaseAppliesEmbeddedSchemaOnlyForApply(t *testing.T) {
	db, err := dbconn.Open("sqlite://" + filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := prepareMigrationDatabase(db, false); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("dry-run preparation mutated the database")
	}

	if err := prepareMigrationDatabase(db, true); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version = '000037_finance_evidence_confirms_named_stays'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
}
