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

func TestPayoutInvoiceBillableCents_fallsBackToNet(t *testing.T) {
	p := &store.FinanceBookingPayout{NetCents: 5927, AmountCents: sql.NullInt64{}}
	if n := payoutInvoiceBillableCents(p); n != 5927 {
		t.Fatalf("got %d want 5927", n)
	}
}
