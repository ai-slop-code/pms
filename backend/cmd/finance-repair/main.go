// finance-repair is a one-off operational tool that re-syncs every
// opened finance month for every property by re-running the same logic
// that powers POST /finance/months/{month}/open. After the fix for the
// recurring-rule edit/delete bug, running this binary against a
// production database deletes any orphaned auto-generated
// finance_transactions whose source rule is no longer active (or whose
// date range no longer covers the month), and rewrites all remaining
// ones from the current ruleset.
//
// It also backfills missing finance_transactions rows for any
// finance_bookings (booking payout) rows whose transaction_id is NULL.
// This repairs the "Monthly Incoming = 0 despite mapped payouts" bug
// where an earlier import created the booking row without its matching
// transaction.
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
// source_type = 'recurring_rule' or 'cleaning_salary', plus inserts
// missing booking_payout transactions; manual transactions are never
// touched.
package main

import (
	"context"
	"database/sql"
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
	} else {
		fmt.Printf("\nDone: re-synced %d months across %d properties.\n", totalMonths, totalProps)
	}

	// Backfill missing finance_transactions rows for orphan booking payouts.
	orphans, err := st.ListOrphanBookingPayouts(ctx)
	if err != nil {
		log.Fatalf("list orphan booking payouts: %v", err)
	}
	if len(orphans) == 0 {
		fmt.Println("\nNo orphan booking payouts found (all finance_bookings rows have a transaction_id).")
		return
	}
	fmt.Printf("\nFound %d orphan booking payout(s) (finance_bookings.transaction_id IS NULL):\n", len(orphans))
	categoryCache := map[int64]int64{}
	var backfilled, skipped, skippedZero int
	for _, op := range orphans {
		fmt.Printf("  property=%d ref=%s payout_date=%s net=%d\n",
			op.PropertyID, op.ReferenceNumber, op.PayoutDate.UTC().Format("2006-01-02"), op.NetCents)
		if op.NetCents == 0 {
			// By convention (see UpsertBookingFinanceTransaction in
			// finance_bookings_merge.go) we never create a finance
			// transaction for a zero-net booking row, since it
			// represents a cancellation / no-show / commission-only
			// statement row with no cash flow. Skip silently.
			skippedZero++
			continue
		}
		if *dryRun {
			continue
		}
		catID, ok := categoryCache[op.PropertyID]
		if !ok {
			id, err := st.FinanceCategoryIDByCode(ctx, op.PropertyID, "booking_income")
			if err != nil || id == 0 {
				log.Printf("    SKIP: booking_income category missing for property %d (err=%v)", op.PropertyID, err)
				skipped++
				continue
			}
			categoryCache[op.PropertyID] = id
			catID = id
		}
		txInput := &store.FinanceTransaction{
			PropertyID:      op.PropertyID,
			TransactionDate: op.PayoutDate,
			Direction:       "incoming",
			AmountCents:     op.NetCents,
			CategoryID:      sql.NullInt64{Int64: catID, Valid: true},
			Note:            sql.NullString{String: "Booking.com payout " + op.ReferenceNumber, Valid: true},
			SourceType:      "booking_payout",
			SourceReference: sql.NullString{String: op.ReferenceNumber, Valid: true},
			IsAutoGenerated: true,
		}
		if _, err := st.BackfillBookingPayoutTransaction(ctx, op.ID, txInput); err != nil {
			log.Printf("    FAIL: backfill payout id=%d: %v", op.ID, err)
			skipped++
			continue
		}
		backfilled++
	}
	if *dryRun {
		fmt.Printf("\nDRY RUN: would backfill %d orphan booking payout(s) (skipping %d zero-net rows).\n",
			len(orphans)-skippedZero, skippedZero)
		return
	}
	fmt.Printf("\nBackfilled %d / %d orphan booking payout(s) (skipped %d zero-net, %d errors).\n",
		backfilled, len(orphans), skippedZero, skipped)
}
