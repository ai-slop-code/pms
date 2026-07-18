// pms21-migration runs the PMS_21 migration dry-run planner.
//
// Usage:
//
//	go run ./cmd/pms21-migration --db ../data/pms.db --dry-run
//	pms21-migration --db /data/pms.db --dry-run
//
// Apply mode is intentionally not implemented in Stage 1. The command refuses
// to run without --dry-run so old-data backfill cannot be triggered by accident.
// Run this only after the additive Stage 1 schema migration is already applied;
// the dry-run command itself does not migrate or write to the database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/store"
)

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database (sqlite://... or filesystem path)")
	dryRun := flag.Bool("dry-run", false, "required; plan PMS_21 backfill without mutating data")
	sampleLimit := flag.Int("sample-limit", 10, "maximum samples per conflict class")
	flag.Parse()

	if !*dryRun {
		log.Fatal("--dry-run is required; PMS_21 apply mode is not implemented yet")
	}
	if *dbPath == "" {
		if env := os.Getenv("DATABASE_PATH"); env != "" {
			*dbPath = env
		}
	}
	if *dbPath == "" {
		log.Fatal("--db (or DATABASE_PATH) is required")
	}
	dsn := *dbPath
	if !strings.HasPrefix(dsn, "sqlite://") {
		dsn = "sqlite://" + dsn
	}
	db, err := dbconn.Open(dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	report, err := (&store.Store{DB: db}).PlanPMS21Migration(context.Background(), *sampleLimit)
	if err != nil {
		log.Fatalf("plan: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		log.Fatalf("encode: %v", err)
	}
}
