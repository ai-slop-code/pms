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

func TestProformaAndInvoiceCanShareStayWithIndependentSeries(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	stay := createFinanceNamedStay(t, st, pid, "PF-REF", "2026-06-01", "2026-06-03")
	proforma := testInvoice(pid, stay.ID)
	proforma.DocumentType = "proforma"
	proforma.BookingReference = sql.NullString{String: "6429185079", Valid: true}
	proforma.GuestName = sql.NullString{String: "Guest Example", Valid: true}
	proforma.PriceDetails = sql.NullString{String: "Accommodation only", Valid: true}
	proforma.TaxableSupplyDate = time.Time{}
	proforma.DueDate = time.Time{}
	proforma.PaymentStatus = "not_applicable"
	proforma.PaymentNote = ""
	proforma.SupplierSnapshotJSON = `{"name":"Individual Owner","address_line_1":"Main 1","dic":"1234567890","vat_status":"non_vat_payer"}`
	proforma.AmountTotalCents = 123456
	created, err := st.CreateInvoice(ctx, proforma)
	if err != nil {
		t.Fatal(err)
	}
	if created.InvoiceNumber != "PF/P001/2026/0001" || created.PaymentStatus != "not_applicable" {
		t.Fatalf("created pro forma=%+v", created)
	}
	final := testInvoice(pid, stay.ID)
	final.SupplierSnapshotJSON = `{"name":"Owner"}`
	final.CustomerSnapshotJSON = `{"company_name":"Company"}`
	finalCreated, err := st.CreateInvoice(ctx, final)
	if err != nil {
		t.Fatal(err)
	}
	if finalCreated.InvoiceNumber != "P001/2026/0001" {
		t.Fatalf("final number=%q", finalCreated.InvoiceNumber)
	}
	var count int
	if err := st.DB.QueryRow(`SELECT count(*) FROM invoices WHERE property_id = ? AND named_stay_id = ?`, pid, stay.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("shared-stay invoice count=%d want 2", count)
	}
}
