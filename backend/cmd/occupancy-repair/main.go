// occupancy-repair is a one-off operational tool for PMS_19 §11. It runs the
// deterministic ICS-reconciliation repair for every property: derive upstream
// ownership, build per-night coverage, and resolve capacity-one duplicate
// active rows. It never hard-deletes occupancy rows.
//
// Usage:
//
//	go run ./backend/cmd/occupancy-repair --db ./data/pms.db --dry-run
//	occupancy-repair --db /data/pms.db            # apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"pms/backend/internal/dbconn"
	mig "pms/backend/internal/migrate"
	"pms/backend/internal/store"
)

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database (sqlite://... or filesystem path)")
	dryRun := flag.Bool("dry-run", false, "if set, only report the resolution plan and do not mutate")
	flag.Parse()

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
	if err := mig.Up(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	st := &store.Store{DB: db}
	ctx := context.Background()

	if err := st.BackfillUpstreamOwnership(ctx); err != nil {
		log.Fatalf("backfill: %v", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT id FROM properties ORDER BY id ASC`)
	if err != nil {
		log.Fatalf("list properties: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, id := range ids {
		var report *store.OccupancyRepairReport
		if *dryRun {
			report, err = st.OccupancyRepairPlan(ctx, id)
		} else {
			report, err = st.OccupancyRepairApply(ctx, id)
		}
		if err != nil {
			log.Fatalf("repair property %d: %v", id, err)
		}
		if report.NightsResolved == 0 && *dryRun {
			continue
		}
		_ = enc.Encode(report)
	}
	log.Printf("occupancy-repair complete (dry_run=%v, properties=%d)", *dryRun, len(ids))
}
