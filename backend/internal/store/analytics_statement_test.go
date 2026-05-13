package store

import (
	"context"
	"testing"
	"time"

	"pms/backend/internal/testutil"
)

// insertStatementBooking inserts a finance_bookings row that simulates a
// successfully-merged Booking.com Statement entry. Covers all FEAT-05
// query inputs: booked_on, check_in_date, check_out_date, status,
// persons, room_nights, amount_cents, commission_cents.
func insertStatementBooking(t *testing.T, st *Store, pid int64, ref, bookedOn, checkIn, checkOut, status string, persons, nights int, amount, commission int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(context.Background(), `
		INSERT INTO finance_bookings
			(property_id, reference_number, source_channel,
			 has_payout_data, has_statement_data,
			 booked_on, check_in_date, check_out_date,
			 guest_name, reservation_status, currency, payment_status,
			 amount_cents, commission_cents, payment_service_fee_cents, net_cents,
			 persons, rooms, room_nights,
			 payout_date, row_type, status,
			 created_at, updated_at)
		VALUES (?, ?, 'booking_com',
			0, 1,
			?, ?, ?,
			'Test Guest', ?, 'EUR', 'paid',
			?, ?, 0, 0,
			?, 1, ?,
			?, 'stay', ?,
			?, ?)`,
		pid, ref,
		bookedOn, checkIn, checkOut,
		status,
		amount, commission,
		persons, nights,
		bookedOn, status,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert statement booking %s: %v", ref, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func defaultStatementWindow() (time.Time, time.Time) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return from, to
}

// --- Cancellation cohorts ---

func TestListCancellationByBookingCohort_ExcludesOtherStatusesFromRate(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// April 2026 booking cohort: 2 active, 1 cancelled, 1 modified (other),
	// 1 no-show (other) → rate = 1 / (1+2) = 0.333…
	insertStatementBooking(t, st, pid, "B1", "2026-04-02", "2026-04-20", "2026-04-22", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "B2", "2026-04-15", "2026-05-01", "2026-05-03", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "B3", "2026-04-20", "2026-05-10", "2026-05-12", "CANCELLED", 2, 2, 0, 0)
	insertStatementBooking(t, st, pid, "B4", "2026-04-25", "2026-05-15", "2026-05-17", "MODIFIED", 2, 2, 25000, 3500)
	insertStatementBooking(t, st, pid, "B5", "2026-04-28", "2026-05-20", "2026-05-22", "NO_SHOW", 2, 2, 0, 0)

	// May 2026 booking cohort: just one active row.
	insertStatementBooking(t, st, pid, "B6", "2026-05-05", "2026-06-01", "2026-06-03", "OK", 2, 2, 30000, 4500)

	from, to := defaultStatementWindow()
	rows, err := st.ListCancellationByBookingCohort(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 cohorts, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Month != "2026-04" {
		t.Fatalf("first month = %q, want 2026-04", rows[0].Month)
	}
	if rows[0].Active != 2 || rows[0].Cancelled != 1 || rows[0].Other != 2 {
		t.Fatalf("april counts: %+v", rows[0])
	}
	wantRate := 1.0 / 3.0
	if delta := rows[0].Rate - wantRate; delta > 1e-9 || delta < -1e-9 {
		t.Fatalf("april rate = %v, want %v", rows[0].Rate, wantRate)
	}
	if rows[1].Month != "2026-05" || rows[1].Active != 1 || rows[1].Cancelled != 0 || rows[1].Rate != 0 {
		t.Fatalf("may cohort: %+v", rows[1])
	}
}

func TestListCancellationByArrivalCohort_GroupsByCheckInMonth(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// All booked in April but arrive across months.
	insertStatementBooking(t, st, pid, "A1", "2026-04-01", "2026-04-20", "2026-04-22", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "A2", "2026-04-02", "2026-04-25", "2026-04-27", "CANCELLED", 2, 2, 0, 0)
	insertStatementBooking(t, st, pid, "A3", "2026-04-03", "2026-05-10", "2026-05-12", "OK", 2, 2, 30000, 4500)

	from, to := defaultStatementWindow()
	rows, err := st.ListCancellationByArrivalCohort(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 cohorts, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Month != "2026-04" || rows[0].Active != 1 || rows[0].Cancelled != 1 {
		t.Fatalf("april arrival cohort: %+v", rows[0])
	}
	if rows[1].Month != "2026-05" || rows[1].Active != 1 || rows[1].Cancelled != 0 {
		t.Fatalf("may arrival cohort: %+v", rows[1])
	}
}

// --- Lead time histogram ---

func TestListLeadTimeStatementBuckets_AssignsActiveStaysAndExcludesCancelled(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// Lead-time spread (active stays):
	// 0 days → 0-3, 2 days → 0-3, 10 days → 4-14, 30 days → 15-45, 100 days → 46+
	insertStatementBooking(t, st, pid, "L0", "2026-04-10", "2026-04-10", "2026-04-12", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "L1", "2026-04-08", "2026-04-10", "2026-04-12", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "L2", "2026-04-01", "2026-04-11", "2026-04-13", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "L3", "2026-04-01", "2026-05-01", "2026-05-03", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "L4", "2026-04-01", "2026-07-10", "2026-07-12", "OK", 2, 2, 20000, 3000)
	// Cancelled with the same lead time as L4 must be excluded.
	insertStatementBooking(t, st, pid, "LX", "2026-04-01", "2026-07-10", "2026-07-12", "CANCELLED", 2, 2, 0, 0)

	from, to := defaultStatementWindow()
	buckets, err := st.ListLeadTimeStatementBuckets(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"0-3": 2, "4-14": 1, "15-45": 1, "46+": 1}
	if len(buckets) != 4 {
		t.Fatalf("want 4 buckets, got %d", len(buckets))
	}
	for _, b := range buckets {
		if b.Count != want[b.Bucket] {
			t.Errorf("bucket %s = %d, want %d", b.Bucket, b.Count, want[b.Bucket])
		}
	}
}

// --- Persons distribution + ADR by guests ---

func TestListPersonsDistribution_ExcludesNullPersonsAndComputesADR(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// 2 stays of 2 persons, 1 stay of 4 persons; one row with 0 persons must be excluded.
	insertStatementBooking(t, st, pid, "P1", "2026-04-01", "2026-04-10", "2026-04-12", "OK", 2, 2, 20000, 3000) // ADR=10000
	insertStatementBooking(t, st, pid, "P2", "2026-04-02", "2026-04-15", "2026-04-18", "OK", 2, 3, 30000, 4500) // ADR component
	insertStatementBooking(t, st, pid, "P3", "2026-04-03", "2026-04-20", "2026-04-23", "OK", 4, 3, 60000, 9000)
	insertStatementBooking(t, st, pid, "P0", "2026-04-04", "2026-04-25", "2026-04-27", "OK", 0, 2, 99999, 0) // excluded

	from, to := defaultStatementWindow()
	rows, err := st.ListPersonsDistribution(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 persons buckets, got %d (%+v)", len(rows), rows)
	}
	if rows[0].Persons != 2 || rows[0].Stays != 2 || rows[0].RoomNights != 5 || rows[0].GrossCents != 50000 {
		t.Fatalf("persons=2 bucket: %+v", rows[0])
	}
	if rows[0].ADRCents != 50000/5 {
		t.Fatalf("persons=2 ADR = %d, want 10000", rows[0].ADRCents)
	}
	if rows[1].Persons != 4 || rows[1].Stays != 1 || rows[1].ADRCents != 60000/3 {
		t.Fatalf("persons=4 bucket: %+v", rows[1])
	}
}

// --- Commission rate trend + per stay ---

func TestListCommissionRateTrend_WeightsByGross(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	// April: 10000@1500 and 30000@6000 → total 8000/40000 = 0.20
	insertStatementBooking(t, st, pid, "C1", "2026-04-01", "2026-04-10", "2026-04-12", "OK", 2, 2, 10000, 1500)
	insertStatementBooking(t, st, pid, "C2", "2026-04-15", "2026-04-20", "2026-04-22", "OK", 2, 2, 30000, 6500)
	// Cancelled row in april must be ignored.
	insertStatementBooking(t, st, pid, "CX", "2026-04-15", "2026-04-25", "2026-04-27", "CANCELLED", 2, 2, 99999, 99999)
	// May: single stay 20000@3000 → 0.15
	insertStatementBooking(t, st, pid, "C3", "2026-05-05", "2026-05-15", "2026-05-17", "OK", 2, 2, 20000, 3000)

	from, to := defaultStatementWindow()
	rows, err := st.ListCommissionRateTrend(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 months, got %d (%+v)", len(rows), rows)
	}
	wantApril := float64(1500+6500) / float64(10000+30000)
	if rows[0].Month != "2026-04" || rows[0].Stays != 2 || rows[0].GrossCents != 40000 || rows[0].CommissionCents != 8000 {
		t.Fatalf("april trend row: %+v", rows[0])
	}
	if delta := rows[0].Rate - wantApril; delta > 1e-9 || delta < -1e-9 {
		t.Fatalf("april rate = %v, want %v", rows[0].Rate, wantApril)
	}
	if rows[1].Month != "2026-05" || rows[1].Rate != 0.15 {
		t.Fatalf("may trend row: %+v", rows[1])
	}
}

func TestListCommissionPerStay_OrdersByCheckInDateDesc(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	insertStatementBooking(t, st, pid, "S1", "2026-04-01", "2026-04-10", "2026-04-12", "OK", 2, 2, 20000, 3000)
	insertStatementBooking(t, st, pid, "S2", "2026-04-02", "2026-04-20", "2026-04-22", "OK", 2, 2, 40000, 6000)
	insertStatementBooking(t, st, pid, "SX", "2026-04-03", "2026-04-25", "2026-04-27", "CANCELLED", 2, 2, 99999, 99999)

	from, to := defaultStatementWindow()
	rows, err := st.ListCommissionPerStay(ctx, pid, from, to, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 stays, got %d (%+v)", len(rows), rows)
	}
	if rows[0].ReferenceNumber != "S2" || rows[1].ReferenceNumber != "S1" {
		t.Fatalf("order = [%s, %s], want [S2, S1]", rows[0].ReferenceNumber, rows[1].ReferenceNumber)
	}
	if rows[0].Rate != 0.15 || rows[1].Rate != 0.15 {
		t.Fatalf("rates = [%v, %v], want both 0.15", rows[0].Rate, rows[1].Rate)
	}
}

// --- Freshness helpers ---

func TestLastStatementBookedOn_ReturnsMaxAndNilWhenAbsent(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	got, err := st.LastStatementBookedOn(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("empty: want nil, got %v", *got)
	}

	insertStatementBooking(t, st, pid, "F1", "2026-04-01T08:00:00Z", "2026-04-10", "2026-04-12", "OK", 2, 2, 10000, 1500)
	insertStatementBooking(t, st, pid, "F2", "2026-04-20T18:30:00Z", "2026-04-25", "2026-04-27", "OK", 2, 2, 10000, 1500)
	insertStatementBooking(t, st, pid, "F3", "2026-04-15T12:00:00Z", "2026-04-30", "2026-05-02", "OK", 2, 2, 10000, 1500)

	got, err = st.LastStatementBookedOn(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("seeded: want value, got nil")
	}
	want := time.Date(2026, 4, 20, 18, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("max booked_on = %v, want %v", got.UTC(), want)
	}
}

func TestHasAnyStatementData_TogglesWithStatementRows(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	got, err := st.HasAnyStatementData(ctx, pid)
	if err != nil || got {
		t.Fatalf("empty: err=%v has=%v", err, got)
	}

	// A pure payout row (has_statement_data=0) must not flip the flag.
	insertPayout(t, st, pid, "PAY-1", nil, "2026-04-01", "2026-04-05", 30000, 4500, 200, 25300)
	if got, err := st.HasAnyStatementData(ctx, pid); err != nil || got {
		t.Fatalf("payout-only: err=%v has=%v", err, got)
	}

	insertStatementBooking(t, st, pid, "ST-1", "2026-04-01", "2026-04-10", "2026-04-12", "OK", 2, 2, 10000, 1500)
	if got, err := st.HasAnyStatementData(ctx, pid); err != nil || !got {
		t.Fatalf("after seed: err=%v has=%v", err, got)
	}
}

func TestGetAnalyticsFreshness_PopulatesLastStatementDateAndFlag(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := seedAnalyticsProperty(t, st)
	ctx := context.Background()

	f, err := st.GetAnalyticsFreshness(ctx, pid)
	if err != nil || f == nil {
		t.Fatalf("empty: err=%v f=%v", err, f)
	}
	if f.HasStatementData || f.LastStatementBookedOn != nil {
		t.Fatalf("empty freshness has statement data: %+v", f)
	}

	insertStatementBooking(t, st, pid, "FR-1", "2026-04-01T10:00:00Z", "2026-04-10", "2026-04-12", "OK", 2, 2, 10000, 1500)
	f, err = st.GetAnalyticsFreshness(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasStatementData {
		t.Fatal("HasStatementData = false after seeding statement row")
	}
	if f.LastStatementBookedOn == nil || f.LastStatementBookedOn.UTC().Format(time.RFC3339) != "2026-04-01T10:00:00Z" {
		t.Fatalf("LastStatementBookedOn = %v, want 2026-04-01T10:00:00Z", f.LastStatementBookedOn)
	}
}
