package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
)

func TestRunValidatesArgumentsBeforeOpeningDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	t.Setenv("DATABASE_PATH", path)
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "000040") {
		t.Fatalf("help: output=%q err=%v", out.String(), err)
	}
	for _, args := range [][]string{nil, {"up"}, {"cleaning-calendar", "extra"}, {"cleaning-calendar"}} {
		if err := run(args, &out); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("command created missing database: %v", err)
	}
	t.Setenv("DATABASE_PATH", "")
	if err := run([]string{"cleaning-calendar"}, &out); err == nil || !strings.Contains(err.Error(), "DATABASE_PATH") {
		t.Fatalf("expected explicit database path error, got %v", err)
	}
}

func TestRunCleaningCalendarAlreadyMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pms.db")
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMS_ENV", "test")
	t.Setenv("DATABASE_PATH", path)
	var out bytes.Buffer
	if err := run([]string{"cleaning-calendar"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "000040 is complete") {
		t.Fatalf("output=%q", out.String())
	}
}
