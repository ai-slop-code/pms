package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"pms/backend/internal/testutil"
)

// seedAnalyticsProperty creates a property with a known timezone for
// deterministic date computation in tests.
func seedAnalyticsProperty(t *testing.T, st *Store) int64 {
	t.Helper()
	pid := setupFinanceProperty(t, st)
	// Ensure the timezone is UTC (setupFinanceProperty already sets it).
	return pid
}

func insertOccupancy(t *testing.T, st *Store, pid int64, uid, start, end, status, guest, importedAt string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if importedAt == "" {
		importedAt = now
	}
	res, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO occupancies
			(property_id, source_type, source_event_uid, start_at, end_at, status,
			 raw_summary, guest_display_name, content_hash, imported_at, last_synced_at)
		VALUES (?, 'booking_ics', ?, ?, ?, ?, NULL, ?, ?, ?, ?)`,
		pid, uid, start, end, status, guest, "hash-"+uid, importedAt, now)
	if err != nil {
		t.Fatalf("insert occupancy: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertPayout(t *testing.T, st *Store, pid int64, ref string, occID *int64, checkIn, payoutDate string, amount, commission, fee, net int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	var occ interface{}
	if occID != nil {
		occ = *occID
	}
	_, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO finance_booking_payouts
			(property_id, reference_number, payout_id, row_type, check_in_date, check_out_date,
			 guest_name, reservation_status, currency, payment_status,
			 amount_cents, commission_cents, payment_service_fee_cents, net_cents,
			 payout_date, transaction_id, occupancy_id, raw_row_json, created_at, updated_at)
		VALUES (?, ?, NULL, 'stay', ?, NULL, 'Test', 'ok', 'EUR', 'paid',
			?, ?, ?, ?, ?, NULL, ?, NULL, ?, ?)`,
		pid, ref, checkIn, amount, commission, fee, net, payoutDate, occ, now, now)
	if err != nil {
		t.Fatalf("insert payout: %v", err)
	}
}

func insertSyncRun(t *testing.T, st *Store, pid int64, status, finishedAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO occupancy_sync_runs
			(property_id, started_at, finished_at, status, trigger, events_seen, occupancies_upserted, created_at)
		VALUES (?, ?, ?, ?, 'manual', 0, 0, ?)`,
		pid, now, finishedAt, status, now)
	if err != nil {
		t.Fatalf("insert sync run: %v", err)
	}
}

// --- NormalizeGuestName ---

func TestNormalizeGuestName_FoldsDiacriticsAndCollapsesWhitespace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Novák", "novak"},
		{"NOVÁK", "novak"},
		{"  Jana   Nováková ", "jana novakova"},
		{"Jean-Luc Mélenchon", "jean-luc melenchon"},
		{"Łukasz", "lukasz"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		got := NormalizeGuestName(c.in)
		if got != c.want {
			t.Errorf("NormalizeGuestName(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// --- Freshness ---

func TestGetAnalyticsFreshness_EmitsLastSyncAndPayoutAndUnmatchedCount(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// No data yet → everything nil / 0.
	f, err := st.GetAnalyticsFreshness(ctx, pid)
	if err != nil || f == nil {
		t.Fatalf("empty: err=%v f=%v", err, f)
	}
	if f.LastICSSyncAt != nil || f.LastPayoutDate != nil || f.UnmatchedPayoutsCount != 0 {
		t.Fatalf("expected zero-state, got %+v", f)
	}

	// Seed sync runs (one success, one failure; the failure must be ignored).
	insertSyncRun(t, st, pid, "failure", "2026-04-01T10:00:00Z")
	insertSyncRun(t, st, pid, "success", "2026-04-10T12:00:00Z")
	insertSyncRun(t, st, pid, "success", "2026-04-05T12:00:00Z")

	// Two payouts: one matched (linked occupancy), one unmatched.
	occID := insertOccupancy(t, st, pid, "u1", "2026-03-10T15:00:00Z", "2026-03-13T10:00:00Z", "active", "Guest", "2026-02-01T00:00:00Z")
	insertPayout(t, st, pid, "R-1", &occID, "2026-03-10", "2026-03-20T00:00:00Z", 30000, 4500, 300, 25200)
	insertPayout(t, st, pid, "R-2", nil, "2026-03-15", "2026-03-25T00:00:00Z", 20000, 3000, 200, 16800)

	f, err = st.GetAnalyticsFreshness(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if f.LastICSSyncAt == nil || f.LastICSSyncAt.Year() != 2026 || f.LastICSSyncAt.Day() != 10 {
		t.Fatalf("last_ics_sync_at: %+v", f.LastICSSyncAt)
	}
	if f.LastPayoutDate == nil {
		t.Fatalf("last_payout_date nil")
	}
	if f.UnmatchedPayoutsCount != 1 {
		t.Fatalf("unmatched count: got %d want 1", f.UnmatchedPayoutsCount)
	}
}

// --- Forward-revenue invariants ---

func TestSumPayoutGrossNetForStays_MatchesByCheckInDate(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// Stay in February, payout received in March — must cohort in February window.
	occID := insertOccupancy(t, st, pid, "u1", "2026-02-10T15:00:00Z", "2026-02-13T10:00:00Z", "active", "Someone", "2026-01-15T00:00:00Z")
	insertPayout(t, st, pid, "REF-A", &occID, "2026-02-10", "2026-03-05T00:00:00Z", 30000, 4500, 500, 25000)

	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	gross, net, comm, fees, matchedIDs, err := st.SumPayoutGrossNetForStays(ctx, pid, feb, mar)
	if err != nil {
		t.Fatal(err)
	}
	if gross != 30000 || net != 25000 || comm != 4500 || fees != 500 {
		t.Fatalf("feb window: gross=%d net=%d comm=%d fees=%d", gross, net, comm, fees)
	}
	if len(matchedIDs) != 1 || matchedIDs[0] != occID {
		t.Fatalf("matched IDs: %+v", matchedIDs)
	}

	// Querying the March window should return zero.
	apr := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	gross2, _, _, _, ids2, err := st.SumPayoutGrossNetForStays(ctx, pid, mar, apr)
	if err != nil {
		t.Fatal(err)
	}
	if gross2 != 0 || len(ids2) != 0 {
		t.Fatalf("march window should be empty, got gross=%d ids=%+v", gross2, ids2)
	}
}

func TestTrailingADR_ReturnsZeroBelowMinimumMatchedNights(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// One short stay (3 nights) won't meet the 30-night floor.
	occID := insertOccupancy(t, st, pid, "u1", "2026-02-10T15:00:00Z", "2026-02-13T10:00:00Z", "active", "G", "2026-01-01T00:00:00Z")
	insertPayout(t, st, pid, "REF", &occID, "2026-02-10", "2026-02-20T00:00:00Z", 30000, 0, 0, 30000)

	asOf := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	adr, err := st.TrailingADR(ctx, pid, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if adr != 0 {
		t.Fatalf("expected 0 ADR below threshold, got %d", adr)
	}
}

// --- Cancellation counting ---

func TestListCancellationsInArrivalWindow_ExcludesActive(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	insertOccupancy(t, st, pid, "a1", "2026-03-10T15:00:00Z", "2026-03-13T10:00:00Z", "active", "Active", "2026-02-01T00:00:00Z")
	insertOccupancy(t, st, pid, "c1", "2026-03-20T15:00:00Z", "2026-03-22T10:00:00Z", "cancelled", "Cancelled", "2026-02-01T00:00:00Z")
	insertOccupancy(t, st, pid, "d1", "2026-03-25T15:00:00Z", "2026-03-27T10:00:00Z", "deleted_from_source", "Deleted", "2026-02-01T00:00:00Z")

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	cans, err := st.ListCancellationsInArrivalWindow(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(cans) != 2 {
		t.Fatalf("expected 2 cancellations, got %d", len(cans))
	}
	active, err := st.CountActiveArrivalsInWindow(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("expected 1 active arrival, got %d", active)
	}
}

// --- Gap nights ---

func TestListGapNights_ZeroForSameDayTurnover_OneForSingleEmptyNight(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// Same-day turnover: stay1 ends 2026-05-10, stay2 starts 2026-05-10 → no gap.
	// Single empty night: stay2 ends 2026-05-14, stay3 starts 2026-05-15 → gap 05-14.
	// Multi-night gap: stay3 ends 2026-05-20, stay4 starts 2026-05-25 → NOT a single-gap → ignored.
	insertOccupancy(t, st, pid, "s1", "2026-05-05T15:00:00Z", "2026-05-10T10:00:00Z", "active", "g1", "")
	insertOccupancy(t, st, pid, "s2", "2026-05-10T15:00:00Z", "2026-05-14T10:00:00Z", "active", "g2", "")
	insertOccupancy(t, st, pid, "s3", "2026-05-15T15:00:00Z", "2026-05-20T10:00:00Z", "active", "g3", "")
	insertOccupancy(t, st, pid, "s4", "2026-05-25T15:00:00Z", "2026-05-28T10:00:00Z", "active", "g4", "")

	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	gaps, err := st.ListGapNights(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].Date != "2026-05-14" {
		t.Fatalf("expected exactly one gap on 2026-05-14, got %+v", gaps)
	}
}

// --- Returning guests ---

func TestListReturningGuests_RejectsShortNamesAndRequiresRepeat(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// Short name (5 chars normalized) — must be ignored.
	insertOccupancy(t, st, pid, "a1", "2025-01-10T15:00:00Z", "2025-01-12T10:00:00Z", "active", "Anna", "")
	insertOccupancy(t, st, pid, "a2", "2026-01-10T15:00:00Z", "2026-01-12T10:00:00Z", "active", "Anna", "")

	// Long diacritic name — must fold and match.
	insertOccupancy(t, st, pid, "n1", "2025-02-10T15:00:00Z", "2025-02-13T10:00:00Z", "active", "Jana Nováková", "")
	insertOccupancy(t, st, pid, "n2", "2026-02-10T15:00:00Z", "2026-02-13T10:00:00Z", "active", "JANA NOVAKOVA", "")

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	rows, total, err := st.ListReturningGuests(ctx, pid, from, to, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected exactly one returning guest (long name), got %d: %+v", total, rows)
	}
	if rows[0].NormalizedName != "jana novakova" || rows[0].StayCount != 2 {
		t.Fatalf("unexpected returning guest row: %+v", rows[0])
	}
}

// --- Pace curve monotonicity ---

func TestPaceCurveForWindow_IsMonotonicNonDecreasing(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// Three bookings for May 2026 imported at different dates before the window.
	insertOccupancy(t, st, pid, "p1", "2026-05-10T15:00:00Z", "2026-05-12T10:00:00Z", "active", "A", "2026-03-01T00:00:00Z")
	insertOccupancy(t, st, pid, "p2", "2026-05-15T15:00:00Z", "2026-05-20T10:00:00Z", "active", "B", "2026-04-01T00:00:00Z")
	insertOccupancy(t, st, pid, "p3", "2026-05-25T15:00:00Z", "2026-05-27T10:00:00Z", "active", "C", "2026-04-25T00:00:00Z")

	winStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ty, _, err := st.PaceCurveForWindow(ctx, pid, winStart, winEnd)
	if err != nil {
		t.Fatal(err)
	}
	prev := -1
	for _, p := range ty {
		if p.Count < prev {
			t.Fatalf("pace curve must be monotonic non-decreasing; T=%d count=%d prev=%d", p.DaysBefore, p.Count, prev)
		}
		prev = p.Count
	}
	if ty[len(ty)-1].Count != 3 {
		t.Fatalf("final point must equal total bookings (3), got %d", ty[len(ty)-1].Count)
	}
}

// --- Yearly finance rollup parity ---

func TestYearlyFinanceRollup_MatchesLegacySummary(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()
	catID := categoryIDByCode(t, st, pid, "booking_income")
	outCat := categoryIDByCode(t, st, pid, "utilities")

	_, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		Direction:       "incoming",
		AmountCents:     50000,
		CategoryID:      sql.NullInt64{Int64: catID, Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),
		Direction:       "outgoing",
		AmountCents:     12000,
		CategoryID:      sql.NullInt64{Int64: outCat, Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	roll, err := st.YearlyFinanceRollup(ctx, pid, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if roll.IncomingCents != 50000 || roll.OutgoingCents != 12000 || roll.NetCents != 38000 {
		t.Fatalf("rollup mismatch: %+v", roll)
	}
}
