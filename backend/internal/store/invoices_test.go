package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/testutil"
)

func testInvoice(propertyID, stayID int64) *Invoice {
	date := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	return &Invoice{
		PropertyID:           propertyID,
		NamedStayID:          sql.NullInt64{Int64: stayID, Valid: stayID > 0},
		Language:             "en",
		IssueDate:            date,
		TaxableSupplyDate:    date,
		DueDate:              date,
		StayStartDate:        date,
		StayEndDate:          date.AddDate(0, 0, 2),
		SupplierSnapshotJSON: `{}`,
		CustomerSnapshotJSON: `{}`,
		AmountTotalCents:     10000,
	}
}

func TestCreateInvoiceRequiresNamedStay(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)

	if _, err := st.CreateInvoice(context.Background(), testInvoice(pid, 0)); err == nil || !strings.Contains(err.Error(), "named_stay_id is required") {
		t.Fatalf("error=%v, want named_stay_id requirement", err)
	}
	var invoices, sequences int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM invoices WHERE property_id = ?`, pid).Scan(&invoices); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM invoice_sequences WHERE property_id = ?`, pid).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if invoices != 0 || sequences != 0 {
		t.Fatalf("invoices=%d sequences=%d, want no writes", invoices, sequences)
	}
}

func TestInvoiceFinanceLinkMustMatchPropertyAndStay(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	stay := createFinanceNamedStay(t, st, pid, "INV-STAY", "2026-06-01", "2026-06-03")
	otherStay := createFinanceNamedStay(t, st, pid, "INV-OTHER", "2026-06-04", "2026-06-05")
	if err := st.CreateBookingPayout(ctx, &FinanceBookingPayout{
		PropertyID: pid, ReferenceNumber: "INV-PAYOUT", NetCents: 10000,
		PayoutDate:  time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
		NamedStayID: sql.NullInt64{Int64: stay.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	payout, err := st.GetBookingPayoutByReference(ctx, pid, "INV-PAYOUT")
	if err != nil {
		t.Fatal(err)
	}

	mismatch := testInvoice(pid, otherStay.ID)
	mismatch.FinanceBookingPayoutID = sql.NullInt64{Int64: payout.ID, Valid: true}
	if _, err := st.CreateInvoice(ctx, mismatch); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error=%v, want stay mismatch", err)
	}

	valid := testInvoice(pid, stay.ID)
	valid.FinanceBookingPayoutID = sql.NullInt64{Int64: payout.ID, Valid: true}
	created, err := st.CreateInvoice(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 || created.NamedStayID.Int64 != stay.ID {
		t.Fatalf("created=%+v", created)
	}
}
