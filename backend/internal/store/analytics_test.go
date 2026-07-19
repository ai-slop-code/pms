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
		INSERT INTO finance_bookings
			(property_id, reference_number, payout_id, row_type, check_in_date, check_out_date,
			 guest_name, reservation_status, currency, payment_status,
			 amount_cents, commission_cents, payment_service_fee_cents, net_cents,
			 payout_date, transaction_id, occupancy_id, raw_payout_row_json, created_at, updated_at)
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

func insertNamedStayForAnalytics(t *testing.T, st *Store, pid int64, name, stayType, checkIn, checkOut string, manualRevenue *int64, review string) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	if review == "" {
		review = "confirmed"
	}
	var revenue interface{}
	var currency interface{}
	if manualRevenue != nil {
		revenue = *manualRevenue
		currency = "EUR"
	}
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, manual_revenue_cents, manual_revenue_currency, review_status, nuki_generation_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'active', 1, ?, ?, ?, 'not_applicable', ?, ?)`, pid, name, stayType, checkIn, checkOut, revenue, currency, review, now, now)
	if err != nil {
		t.Fatalf("insert named stay: %v", err)
	}
	id, _ := res.LastInsertId()
	start, _ := time.Parse("2006-01-02", checkIn)
	end, _ := time.Parse("2006-01-02", checkOut)
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		if _, err := st.DB.ExecContext(ctx, `INSERT INTO named_stay_nights (property_id, named_stay_id, local_night_date, active, created_at) VALUES (?, ?, ?, 1, ?)`, pid, id, d.Format("2006-01-02"), now); err != nil {
			t.Fatalf("insert named night: %v", err)
		}
	}
	return id
}

func TestAnalyticsStage9_NamedStaySemanticsExcludeRawAndUnfundedExternal(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := st.DB.ExecContext(ctx, `
		INSERT INTO raw_booking_blocks (property_id, source_type, source_event_uid, check_in_date, check_out_date, status, content_hash, imported_at, last_synced_at, created_at, updated_at)
		VALUES (?, 'booking_ics', 'raw-stage9', '2026-07-01', '2026-07-04', 'active', 'raw', ?, ?, ?, ?)`, pid, now, now, now, now)
	if err != nil {
		t.Fatalf("insert raw block: %v", err)
	}
	insertNamedStayForAnalytics(t, st, pid, "Booking Guest", "booking_com", "2026-07-05", "2026-07-07", nil, "confirmed")
	insertNamedStayForAnalytics(t, st, pid, "No Revenue External", "external", "2026-07-08", "2026-07-10", nil, "confirmed")
	manual := int64(18000)
	manualID := insertNamedStayForAnalytics(t, st, pid, "Manual External", "external", "2026-07-11", "2026-07-13", &manual, "confirmed")
	insertNamedStayForAnalytics(t, st, pid, "Maintenance", "maintenance", "2026-07-14", "2026-07-15", nil, "confirmed")
	insertNamedStayForAnalytics(t, st, pid, "Review Booking", "booking_com", "2026-07-16", "2026-07-17", nil, "needs_review")

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	stays, err := st.ListActiveOccupanciesInDateRange(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got := NightsSoldInRange(stays, from, to); got != 4 {
		t.Fatalf("sold nights=%d want 4", got)
	}
	ids := map[int64]bool{}
	for _, stay := range stays {
		ids[stay.ID] = true
	}
	if !ids[manualID] {
		t.Fatalf("manual-revenue external stay missing from sold set: %+v", stays)
	}

	blockers, err := st.ListClosedOccupanciesInDateRange(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got := ClosedNightsInRange(blockers, from, to); got != 4 {
		t.Fatalf("availability blockers=%d want 4", got)
	}

	gross, net, _, _, matched, err := st.SumPayoutGrossNetForStays(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if gross != manual || net != manual || len(matched) != 1 || matched[0] != manualID {
		t.Fatalf("manual revenue gross=%d net=%d matched=%v", gross, net, matched)
	}
}

func TestAnalyticsUsesNamedStayNightsWhenStayRangeDiverges(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()
	soldID := insertNamedStayForAnalytics(t, st, pid, "Diverged Booking", StayTypeBookingCom, "2026-08-01", "2026-08-04", nil, "confirmed")
	closedID := insertNamedStayForAnalytics(t, st, pid, "Diverged Maintenance", StayTypeMaintenance, "2026-08-05", "2026-08-08", nil, "confirmed")
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM named_stay_nights WHERE named_stay_id = ? AND local_night_date = '2026-08-02'`, soldID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM named_stay_nights WHERE named_stay_id = ? AND local_night_date = '2026-08-06'`, closedID); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	stays, err := st.ListActiveOccupanciesInDateRange(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got := NightsSoldInRange(stays, from, to); got != 2 {
		t.Fatalf("sold nights=%d want 2 active night rows", got)
	}
	closed, err := st.ListClosedOccupanciesInDateRange(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got := ClosedNightsInRange(closed, from, to); got != 2 {
		t.Fatalf("closed nights=%d want 2 active night rows", got)
	}
}

func TestAnalyticsBoundaryCrossingStayUsesCompleteNightSet(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()
	revenue := int64(12000)
	insertNamedStayForAnalytics(t, st, pid, "Boundary External", StayTypeExternal, "2026-07-30", "2026-08-03", &revenue, "confirmed")

	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	stays, err := st.ListActiveOccupanciesInDateRange(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(stays) != 1 || len(stays[0].NightDates) != 4 {
		t.Fatalf("stays=%+v want one stay with all 4 active nights", stays)
	}
	if got := NightsSoldInRange(stays, from, to); got != 2 {
		t.Fatalf("sold nights=%d want 2 report-range nights", got)
	}
	if got := ExternalSaleRevenueCentsInRange(stays, from, to); got != 6000 {
		t.Fatalf("partial-range external revenue=%d want 6000", got)
	}

	buckets, err := st.ListLengthOfStayBuckets(ctx, pid, from, to)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, bucket := range buckets {
		counts[bucket.Bucket] = bucket.Count
	}
	if counts["4-5"] != 1 || counts["2"] != 0 {
		t.Fatalf("length buckets=%v want boundary stay in 4-5", counts)
	}
}

func TestADRByDimension_ExcludesNeedsReviewAndUsesNamedStayNights(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	confirmedID := insertNamedStayForAnalytics(t, st, pid, "Confirmed Booking", StayTypeBookingCom, "2026-09-01", "2026-09-04", nil, "confirmed")
	reviewID := insertNamedStayForAnalytics(t, st, pid, "Review Booking", StayTypeBookingCom, "2026-09-05", "2026-09-09", nil, "needs_review")
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM named_stay_nights WHERE named_stay_id = ? AND local_night_date = '2026-09-02'`, confirmedID); err != nil {
		t.Fatal(err)
	}
	insertPayout(t, st, pid, "ADR-CONFIRMED", nil, "2026-09-01", "2026-09-10", 30000, 0, 0, 30000)
	insertPayout(t, st, pid, "ADR-REVIEW", nil, "2026-09-05", "2026-09-10", 90000, 0, 0, 90000)
	if _, err := st.DB.ExecContext(ctx, `UPDATE finance_bookings SET named_stay_id = ? WHERE property_id = ? AND reference_number = 'ADR-CONFIRMED'`, confirmedID, pid); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE finance_bookings SET named_stay_id = ? WHERE property_id = ? AND reference_number = 'ADR-REVIEW'`, reviewID, pid); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := st.ADRByDimension(ctx, pid, from, to, "month", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Bucket != "2026-09" || rows[0].GrossCents != 30000 || rows[0].MatchedNights != 2 {
		t.Fatalf("ADR rows=%+v want only confirmed revenue and its 2 active named-stay nights", rows)
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
