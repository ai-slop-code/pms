package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"pms/backend/internal/testutil"
)

func insertRevenueRecognitionBooking(t *testing.T, st *Store, propertyID int64, reference, checkIn, checkOut string, grossCents int, hasPayout bool, status string) {
	t.Helper()
	row := &FinanceBookingPayout{
		PropertyID:      propertyID,
		ReferenceNumber: reference,
		NetCents:        grossCents,
		PayoutDate:      time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		AmountCents:     sql.NullInt64{Int64: int64(grossCents), Valid: true},
		GuestName:       sql.NullString{String: reference + " Guest", Valid: true},
	}
	if checkIn != "" {
		row.CheckInDate = sql.NullString{String: checkIn, Valid: true}
	}
	if checkOut != "" {
		row.CheckOutDate = sql.NullString{String: checkOut, Valid: true}
	}
	if err := st.CreateBookingPayout(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	payoutFlag := 0
	if hasPayout {
		payoutFlag = 1
	}
	if _, err := st.DB.Exec(`UPDATE finance_bookings SET has_payout_data = ?, has_statement_data = ?, status = ? WHERE property_id = ? AND reference_number = ?`, payoutFlag, 1-payoutFlag, status, propertyID, reference); err != nil {
		t.Fatal(err)
	}
}

func TestComputeFinanceRevenueRecognition_ProrationConservesGross(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Fatal(err)
	}

	// Three nights: Jan 31, Feb 1, Feb 2. The first night receives the
	// remainder cent, and payout timing in March is irrelevant.
	insertRevenueRecognitionBooking(t, st, pid, "CROSS-MONTH", "2026-01-31", "2026-02-03", 10000, true, "OK")
	insertRevenueRecognitionBooking(t, st, pid, "STATEMENT-ONLY", "2026-02-10", "2026-02-11", 5000, false, "OK")

	jan, err := st.ComputeFinanceRevenueRecognition(context.Background(), pid, "2026-01", loc)
	if err != nil {
		t.Fatal(err)
	}
	feb, err := st.ComputeFinanceRevenueRecognition(context.Background(), pid, "2026-02", loc)
	if err != nil {
		t.Fatal(err)
	}
	if jan.GrossRevenueCents != 3334 || feb.GrossRevenueCents != 6666 {
		t.Fatalf("recognized gross jan=%d feb=%d, want 3334 and 6666", jan.GrossRevenueCents, feb.GrossRevenueCents)
	}
	if jan.GrossRevenueCents+feb.GrossRevenueCents != 10000 {
		t.Fatalf("allocated total=%d want 10000", jan.GrossRevenueCents+feb.GrossRevenueCents)
	}
	if len(feb.Bookings) != 1 || feb.Bookings[0].RecognizedNights != 2 || !feb.Bookings[0].Unmatched {
		t.Fatalf("unexpected February rows: %+v", feb.Bookings)
	}
}

func TestComputeFinanceRevenueRecognition_FlagsExceptionsAndInvalidWindows(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	insertRevenueRecognitionBooking(t, st, pid, "CANCELLED", "2026-04-30", "2026-05-02", 2500, true, "cancelled by guest")
	insertRevenueRecognitionBooking(t, st, pid, "NO-SHOW", "2026-04-07", "2026-04-08", 1500, true, "OK")
	if _, err := st.DB.Exec(`UPDATE finance_bookings SET outcome_override = 'no_show' WHERE property_id = ? AND reference_number = 'NO-SHOW'`, pid); err != nil {
		t.Fatal(err)
	}
	insertRevenueRecognitionBooking(t, st, pid, "MISSING-CHECKOUT", "2026-04-09", "", 5000, true, "OK")
	insertRevenueRecognitionBooking(t, st, pid, "REVERSED", "2026-04-12", "2026-04-10", 5000, true, "OK")

	report, err := st.ComputeFinanceRevenueRecognition(context.Background(), pid, "2026-04", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if report.GrossRevenueCents != 4000 || len(report.Bookings) != 2 {
		t.Fatalf("gross=%d bookings=%d, want 4000 and 2", report.GrossRevenueCents, len(report.Bookings))
	}
	flags := map[string]FinanceRevenueRecognitionBooking{}
	for _, row := range report.Bookings {
		flags[row.ReferenceNumber] = row
	}
	if !flags["CANCELLED"].Cancelled || !flags["NO-SHOW"].NoShow {
		t.Fatalf("exception flags missing: %+v", flags)
	}
	may, err := st.ComputeFinanceRevenueRecognition(context.Background(), pid, "2026-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if may.GrossRevenueCents != 0 || len(may.Bookings) != 0 {
		t.Fatalf("cancelled charge must stay in scheduled check-in month, got %+v", may)
	}
	if len(report.ExcludedBookings) != 2 {
		t.Fatalf("excluded=%+v want 2 rows", report.ExcludedBookings)
	}
	reasons := map[string]string{}
	for _, row := range report.ExcludedBookings {
		reasons[row.ReferenceNumber] = row.Reason
	}
	if reasons["MISSING-CHECKOUT"] != "missing_check_out" || reasons["REVERSED"] != "invalid_stay_window" {
		t.Fatalf("unexpected issue reasons: %+v", reasons)
	}
}
