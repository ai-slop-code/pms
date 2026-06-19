package api

import (
	"database/sql"
	"testing"

	"pms/backend/internal/store"
)

func TestPayoutInvoiceBillableCents_prefersGrossAmount(t *testing.T) {
	p := &store.FinanceBookingPayout{
		NetCents:    5927,
		AmountCents: sql.NullInt64{Int64: 7226, Valid: true},
	}
	if n := payoutInvoiceBillableCents(p); n != 7226 {
		t.Fatalf("got %d want 7226", n)
	}
}

func TestPayoutInvoiceBillableCents_prefersPayoutCSVAmountOverStatementAmount(t *testing.T) {
	// Statement merge may have overwritten amount_cents with Original/Final amount;
	// invoice must still use payout CSV "Amount".
	p := &store.FinanceBookingPayout{
		NetCents:    5226,
		AmountCents: sql.NullInt64{Int64: 6598, Valid: true}, // statement figure
		RawRowJSON:  sql.NullString{String: `{"amount":"64.89","reference number":"6756848168"}`, Valid: true},
	}
	if n := payoutInvoiceBillableCents(p); n != 6489 {
		t.Fatalf("got %d want 6489 (payout CSV amount)", n)
	}
}

func TestPayoutInvoiceBillableCents_fallsBackToNet(t *testing.T) {
	p := &store.FinanceBookingPayout{NetCents: 5927, AmountCents: sql.NullInt64{}}
	if n := payoutInvoiceBillableCents(p); n != 5927 {
		t.Fatalf("got %d want 5927", n)
	}
}
