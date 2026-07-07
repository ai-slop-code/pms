package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/testutil"
)

func setupFinanceProperty(t *testing.T, st *Store) int64 {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
	u, err := st.CreateUser(ctx, "owner@finance.test", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "FinanceTest", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func categoryIDByCode(t *testing.T, st *Store, pid int64, code string) int64 {
	t.Helper()
	rows, err := st.ListFinanceCategories(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rows {
		if c.Code == code {
			return c.ID
		}
	}
	t.Fatalf("category %q not found", code)
	return 0
}

func TestOpenFinanceMonth_IsIdempotentForRecurringRules(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.UTC
	month := time.Now().UTC().Format("2006-01")

	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet monthly",
		CategoryID:    sql.NullInt64{},
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Now().UTC().Add(-24 * time.Hour),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.ID == 0 {
		t.Fatalf("rule id missing")
	}

	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	txs, err := st.ListFinanceTransactions(context.Background(), pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tx := range txs {
		if tx.SourceType == "recurring_rule" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("recurring transactions = %d, want 1", count)
	}
}

func TestSyncFinanceGeneratedEntries_TracksMetadataAndProtectsManualRows(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()
	month := "2026-04"
	prop, err := st.GetProperty(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	actorID := prop.OwnerUserID

	if _, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet",
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-01",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	}); err != nil {
		t.Fatal(err)
	}
	manual, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Direction:       "incoming",
		AmountCents:     12345,
		Note:            sql.NullString{String: "manual income", Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	sync, changes, err := st.SyncFinanceGeneratedEntriesForMonth(ctx, pid, month, &actorID, time.UTC, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if sync.Status != "synced" || !sync.LastSyncedAt.Valid || sync.LastSyncedReason.String != "manual" {
		t.Fatalf("sync metadata = %+v", sync)
	}
	if sync.LastSyncedBy.Int64 != actorID {
		t.Fatalf("last_synced_by = %d, want %d", sync.LastSyncedBy.Int64, actorID)
	}
	if changes.RecurringInserted != 1 {
		t.Fatalf("recurring inserted = %d, want 1", changes.RecurringInserted)
	}

	if _, _, err := st.SyncFinanceGeneratedEntriesForMonth(ctx, pid, month, &actorID, time.UTC, "recurring_rule_update"); err != nil {
		t.Fatal(err)
	}
	sync, err = st.GetFinanceGeneratedEntrySync(ctx, pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if sync.LastSyncedReason.String != "recurring_rule_update" {
		t.Fatalf("last_synced_reason = %q, want recurring_rule_update", sync.LastSyncedReason.String)
	}

	txs, err := st.ListFinanceTransactions(ctx, pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	manualCount := 0
	for _, tx := range txs {
		if tx.ID == manual.ID {
			manualCount++
			if tx.Note.String != "manual income" || tx.AmountCents != 12345 || tx.SourceType != "manual" {
				t.Fatalf("manual transaction was mutated: %+v", tx)
			}
		}
	}
	if manualCount != 1 {
		t.Fatalf("manual transaction count = %d, want 1", manualCount)
	}
}

func TestComputeFinanceSummary_CalculatesCleanerMargin(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	month := "2026-04"
	tDate := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)

	bookingCat := categoryIDByCode(t, st, pid, "booking_income")
	cleaningCat := categoryIDByCode(t, st, pid, "cleaning_salary")

	_, err := st.CreateFinanceTransaction(context.Background(), &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: tDate,
		Direction:       "incoming",
		AmountCents:     100000,
		CategoryID:      sql.NullInt64{Int64: bookingCat, Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFinanceTransaction(context.Background(), &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: tDate,
		Direction:       "outgoing",
		AmountCents:     40000,
		CategoryID:      sql.NullInt64{Int64: cleaningCat, Valid: true},
		SourceType:      "cleaning_salary",
		SourceReference: sql.NullString{String: month, Valid: true},
		IsAutoGenerated: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	sum, err := st.ComputeFinanceSummary(context.Background(), pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MonthlyPropertyIncomeCents != 100000 {
		t.Fatalf("monthly property income = %d, want 100000", sum.MonthlyPropertyIncomeCents)
	}
	if sum.CleanerExpenseCents != 40000 {
		t.Fatalf("cleaner expense = %d, want 40000", sum.CleanerExpenseCents)
	}
	if sum.CleanerMargin < 0.399 || sum.CleanerMargin > 0.401 {
		t.Fatalf("cleaner margin = %f, want ~0.4", sum.CleanerMargin)
	}
}

func TestOpenFinanceMonth_PositiveTimezoneKeepsTargetMonth(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.FixedZone("UTC+2", 2*60*60)
	month := "2026-04"

	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "TZ recurring",
		CategoryID:    sql.NullInt64{},
		AmountCents:   1234,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.ID == 0 {
		t.Fatalf("rule id missing")
	}

	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListFinanceTransactions(context.Background(), pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected recurring row in month %s", month)
	}
}

// TestOpenFinanceMonth_PurgesOrphanRecurringTransactions reproduces the bug
// where deactivating a recurring rule (and replacing it with a new one) left
// the old auto-generated finance_transaction in place across re-opens. After
// the fix, OpenFinanceMonth deletes any recurring-rule transactions whose
// source rule is no longer active or no longer covers the month.
func TestOpenFinanceMonth_PurgesOrphanRecurringTransactions(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.UTC
	month := "2026-04"
	ctx := context.Background()

	old, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Mortgage old",
		AmountCents:   30076,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2025-01",
		EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenFinanceMonth(ctx, pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	// Deactivate the old rule and add a replacement at a new amount.
	deactivated := false
	if _, err := st.UpdateFinanceRecurringRule(ctx, pid, old.ID, nil, nil, nil, nil, nil, nil, nil, nil, &deactivated); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Mortgage new",
		AmountCents:   39470,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-04",
		EffectiveFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.OpenFinanceMonth(ctx, pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	txs, err := st.ListFinanceTransactions(ctx, pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, tx := range txs {
		if tx.SourceType == "recurring_rule" {
			total += tx.AmountCents
		}
	}
	if total != 39470 {
		t.Fatalf("recurring total cents = %d, want 39470 (only the active replacement rule should remain)", total)
	}
}

// TestDeleteFinanceRecurringRule_CascadesTransactions verifies that deleting
// a recurring rule also removes every auto-generated finance_transaction
// produced by it, regardless of source_reference_id format.
func TestDeleteFinanceRecurringRule_CascadesTransactions(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()

	rule, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet",
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-01",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"2026-01", "2026-02", "2026-03"} {
		if _, err := st.OpenFinanceMonth(ctx, pid, m, nil, time.UTC); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.DeleteFinanceRecurringRule(ctx, pid, rule.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetFinanceRecurringRuleByID(ctx, pid, rule.ID); err == nil {
		t.Fatal("rule still present after delete")
	}
	for _, m := range []string{"2026-01", "2026-02", "2026-03"} {
		txs, err := st.ListFinanceTransactions(ctx, pid, m, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range txs {
			if tx.SourceType == "recurring_rule" {
				t.Fatalf("month %s still has recurring_rule transaction id=%d", m, tx.ID)
			}
		}
	}
}

func TestOpenFinanceMonth_PositiveTimezoneKeepsTargetMonth_AfterPurge(t *testing.T) {
	// Smoke test: ensure existing positive-timezone test still passes after
	// the orphan purge is added (regression guard).
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.FixedZone("UTC+2", 2*60*60)
	month := "2026-04"
	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "TZ recurring",
		AmountCents:   1234,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil || rule.ID == 0 {
		t.Fatalf("rule create failed: %v", err)
	}
	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
}

func TestFindOrCreateOccupancyForPayoutStayDates_CreatesHistoricalStay(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.FixedZone("UTC+2", 2*60*60)

	occ, err := st.FindOrCreateOccupancyForPayoutStayDates(
		context.Background(),
		pid,
		"BP-1001",
		"2025-01-10",
		"2025-01-12",
		"Jane Guest",
		loc,
	)
	if err != nil {
		t.Fatal(err)
	}
	if occ == nil {
		t.Fatalf("expected occupancy to be created")
	}
	if occ.SourceType != "booking_payout" {
		t.Fatalf("source_type=%q want booking_payout", occ.SourceType)
	}
	if occ.SourceEventUID != "booking_payout:BP-1001" {
		t.Fatalf("source_event_uid=%q want booking_payout:BP-1001", occ.SourceEventUID)
	}
	if got := occ.StartAt.In(loc).Format("2006-01-02"); got != "2025-01-10" {
		t.Fatalf("start date=%s want 2025-01-10", got)
	}
	if got := occ.EndAt.In(loc).Format("2006-01-02"); got != "2025-01-12" {
		t.Fatalf("end date=%s want 2025-01-12", got)
	}
	if !occ.GuestDisplayName.Valid || occ.GuestDisplayName.String != "Jane Guest" {
		t.Fatalf("guest_display_name=%v want Jane Guest", occ.GuestDisplayName)
	}
}

func TestFindOrCreateOccupancyForPayoutStayDates_ReusesExistingStay(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.UTC
	ctx := context.Background()

	runID, err := st.StartOccupancySyncRun(ctx, pid, "test")
	if err != nil {
		t.Fatal(err)
	}
	existing := &Occupancy{
		PropertyID:     pid,
		SourceType:     "booking_ics",
		SourceEventUID: "ics-uid-1",
		StartAt:        time.Date(2025, 2, 3, 0, 0, 0, 0, loc),
		EndAt:          time.Date(2025, 2, 6, 0, 0, 0, 0, loc),
		Status:         "active",
		RawSummary:     sql.NullString{String: "ICS guest", Valid: true},
		ContentHash:    "ics-hash-1",
	}
	if err := st.UpsertOccupancy(ctx, existing, runID); err != nil {
		t.Fatal(err)
	}
	existingSaved, err := st.GetOccupancyBySourceEventUID(ctx, pid, "ics-uid-1")
	if err != nil || existingSaved == nil {
		t.Fatalf("expected existing occupancy err=%v", err)
	}

	occ, err := st.FindOrCreateOccupancyForPayoutStayDates(
		ctx,
		pid,
		"BP-2002",
		"2025-02-03",
		"2025-02-06",
		"Guest Changed Name",
		loc,
	)
	if err != nil {
		t.Fatal(err)
	}
	if occ == nil {
		t.Fatalf("expected existing occupancy")
	}
	if occ.ID != existingSaved.ID {
		t.Fatalf("occupancy id=%d want existing id=%d", occ.ID, existingSaved.ID)
	}
	payoutSynthetic, err := st.GetOccupancyBySourceEventUID(ctx, pid, "booking_payout:BP-2002")
	if err != nil {
		t.Fatal(err)
	}
	if payoutSynthetic != nil {
		t.Fatalf("did not expect synthetic payout occupancy when matching occupancy exists")
	}
}

func TestFindOrCreateOccupancyForStatementStayDates_CreatesStatementStayAndSupersedesGenericICS(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()
	loc := time.UTC
	runID, err := st.StartOccupancySyncRun(ctx, pid, "test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 7; i <= 9; i++ {
		start := time.Date(2026, 8, i, 0, 0, 0, 0, time.UTC)
		if err := st.UpsertOccupancy(ctx, &Occupancy{
			PropertyID:     pid,
			SourceType:     "booking_ics",
			SourceEventUID: "ics-split-202608" + fmt.Sprintf("%02d", i),
			StartAt:        start,
			EndAt:          start.AddDate(0, 0, 1),
			Status:         "active",
			RawSummary:     sql.NullString{String: "CLOSED - Not available", Valid: true},
			ContentHash:    fmt.Sprintf("h-%d", i),
		}, runID); err != nil {
			t.Fatal(err)
		}
	}

	occ, err := st.FindOrCreateOccupancyForStatementStayDates(ctx, pid, "ST-3003", "2026-08-07", "2026-08-10", "August Guest", loc)
	if err != nil {
		t.Fatal(err)
	}
	if occ == nil {
		t.Fatal("expected statement occupancy")
	}
	if occ.SourceType != "booking_statement" {
		t.Fatalf("source_type=%q want booking_statement", occ.SourceType)
	}
	if occ.SourceEventUID != "booking_statement:ST-3003" {
		t.Fatalf("source uid=%q", occ.SourceEventUID)
	}
	if err := st.SupersedeGenericICSBlocksForFinanceStayDates(ctx, pid, "2026-08-07", "2026-08-10", loc, occ.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, pid, "", loc, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	activeStatement := 0
	deletedICS := 0
	for _, row := range rows {
		switch {
		case row.ID == occ.ID && row.Status == "active":
			activeStatement++
		case row.SourceType == "booking_ics" && row.Status == "deleted_from_source":
			deletedICS++
		}
	}
	if activeStatement != 1 {
		t.Fatalf("active statement rows=%d want 1", activeStatement)
	}
	if deletedICS != 3 {
		t.Fatalf("deleted generic ics rows=%d want 3", deletedICS)
	}
}
