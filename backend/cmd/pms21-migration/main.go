// pms21-migration runs the PMS_21 migration planner or guarded apply.
//
// Usage:
//
//	go run ./cmd/pms21-migration --db /absolute/path/to/pms.db --dry-run
//	pms21-migration --db /data/pms.db --dry-run
//
// Apply is never the default. Run this only after the additive schema migration.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/store"
)

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database (sqlite://... or filesystem path)")
	dryRun := flag.Bool("dry-run", false, "plan PMS_21 backfill without mutating data")
	apply := flag.Bool("apply", false, "apply the planned PMS_21 backfill")
	confirmApply := flag.Bool("confirm-apply", false, "required confirmation for --apply")
	allowReview := flag.Bool("allow-review-required", false, "create non-reservation stay candidates as needs_review")
	sampleLimit := flag.Int("sample-limit", 10, "maximum samples per conflict class")
	flag.Parse()
	dbFlagExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbFlagExplicit = true
		}
	})

	if *dryRun == *apply {
		log.Fatal("select exactly one mode: --dry-run or --apply")
	}
	if !dbFlagExplicit {
		if *apply {
			log.Fatal("--apply requires an explicit --db path; DATABASE_PATH is not accepted")
		}
		log.Fatal("--dry-run requires an explicit --db path; DATABASE_PATH is not accepted")
	}
	path, err := absoluteSQLitePath(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	if *apply {
		if !*confirmApply {
			log.Fatal("--confirm-apply is required with --apply")
		}
		log.Printf("PMS 21 apply requested: db=%s allow_review_required=%t", path, *allowReview)
	}
	dsn := *dbPath
	if !strings.HasPrefix(dsn, "sqlite://") {
		dsn = "sqlite://" + dsn
	}
	db, err := openMigrationDatabase(dsn, *apply)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	st := &store.Store{DB: db}
	var report *store.PMS21MigrationReport
	if *apply {
		report, err = st.ApplyPMS21Migration(context.Background(), *sampleLimit, *allowReview)
	} else {
		report, err = st.PlanPMS21Migration(context.Background(), *sampleLimit)
	}
	if err != nil {
		if report != nil {
			_ = json.NewEncoder(os.Stdout).Encode(report)
		}
		log.Fatalf("PMS 21 migration: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		log.Fatalf("encode: %v", err)
	}
}

func absoluteSQLitePath(databaseURL string) (string, error) {
	path := strings.TrimPrefix(databaseURL, "sqlite://")
	path = strings.SplitN(path, "?", 2)[0]
	if path == "" || path == ":memory:" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("--db requires an absolute, non-memory SQLite database path")
	}
	return path, nil
}

func openMigrationDatabase(databaseURL string, apply bool) (*sql.DB, error) {
	if apply {
		return dbconn.Open(databaseURL)
	}
	return dbconn.OpenReadOnly(databaseURL)
}
