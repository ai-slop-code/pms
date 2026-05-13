// finance-repair is a one-off operational tool that re-syncs every
// opened finance month for every property by re-running the same logic
// that powers POST /finance/months/{month}/open. After the fix for the
// recurring-rule edit/delete bug, running this binary against a
// production database deletes any orphaned auto-generated
// finance_transactions whose source rule is no longer active (or whose
// date range no longer covers the month), and rewrites all remaining
// ones from the current ruleset.
//
// Usage:
//
//	go run ./backend/cmd/finance-repair --db ./data/pms.db
//
// Or in a container:
//
//	finance-repair --db /data/pms.db
//
// The tool is idempotent and safe to re-run. It only mutates rows whose
// source_type = 'recurring_rule' or 'cleaning_salary'; manual
// transactions are never touched.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/store"
)

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database (sqlite://... or filesystem path)")
	dryRun := flag.Bool("dry-run", false, "if set, only list affected properties/months and do not mutate")
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

	st := &store.Store{DB: db}
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(timezone, 'UTC') FROM properties ORDER BY id ASC`)
	if err != nil {
		log.Fatalf("list properties: %v", err)
	}
	type propInfo struct {
		id int64
		tz string
	}
	var props []propInfo
	for rows.Next() {
		var p propInfo
		if err := rows.Scan(&p.id, &p.tz); err != nil {
			rows.Close()
			log.Fatalf("scan property: %v", err)
		}
		props = append(props, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("iterate properties: %v", err)
	}

	var totalProps, totalMonths int
	for _, p := range props {
		months, err := st.ListOpenedFinanceMonths(ctx, p.id)
		if err != nil {
			log.Fatalf("list opened months for property %d: %v", p.id, err)
		}
		if len(months) == 0 {
			continue
		}
		totalProps++
		loc, err := time.LoadLocation(p.tz)
		if err != nil {
			log.Printf("property %d: bad timezone %q, falling back to UTC", p.id, p.tz)
			loc = time.UTC
		}
		fmt.Printf("property %d (%s): %d opened months\n", p.id, p.tz, len(months))
		for _, m := range months {
			if *dryRun {
				fmt.Printf("  would re-sync %s\n", m)
				totalMonths++
				continue
			}
			n, err := st.OpenFinanceMonth(ctx, p.id, m, nil, loc)
			if err != nil {
				log.Fatalf("re-sync property %d month %s: %v", p.id, m, err)
			}
			fmt.Printf("  re-synced %s (active rules now produce %d row(s))\n", m, n)
			totalMonths++
		}
	}

	if *dryRun {
		fmt.Printf("\nDRY RUN: would re-sync %d months across %d properties.\n", totalMonths, totalProps)
		return
	}
	fmt.Printf("\nDone: re-synced %d months across %d properties.\n", totalMonths, totalProps)
}
