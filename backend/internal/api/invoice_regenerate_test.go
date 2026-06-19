package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
)

func TestRegenerateInvoice_RefreshesAmountFromLinkedPayout(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "invoice-regen-amount@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Regen Amount", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name":    "Regen Amount s.r.o.",
		"billing_address": "Main 1",
		"city":            "Bratislava",
		"postal_code":     "81101",
		"country":         "Slovakia",
	}); err != nil {
		t.Fatal(err)
	}

	payoutDate := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	if err := st.CreateBookingPayout(ctx, &store.FinanceBookingPayout{
		PropertyID:      prop.ID,
		ReferenceNumber: "INV-REGEN-REF",
		NetCents:        5226,
		AmountCents:     sql.NullInt64{Int64: 5194, Valid: true}, // stale statement figure (51.94)
		PayoutDate:      payoutDate,
		CheckInDate:     sql.NullString{String: "2026-05-04", Valid: true},
		CheckOutDate:    sql.NullString{String: "2026-05-05", Valid: true},
		GuestName:       sql.NullString{String: "Test Guest", Valid: true},
		RawRowJSON:      sql.NullString{String: `{"amount":"56.89","booking number":"INV-REGEN-REF"}`, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	payout, err := st.GetBookingPayoutByReference(ctx, prop.ID, "INV-REGEN-REF")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "invoice-regen-amount@example.com", "secret123")

	createBody := map[string]interface{}{
		"booking_payout_id":   payout.ID,
		"language":            "en",
		"issue_date":          "2026-05-08",
		"taxable_supply_date": "2026-05-08",
		"due_date":            "2026-05-08",
		"stay_start_date":     "2026-05-04",
		"stay_end_date":       "2026-05-05",
		"amount_total_cents":  5194,
		"payment_note":        "Already paid via Booking.com.",
		"customer": map[string]string{
			"name":           "Test Guest",
			"address_line_1": "Somewhere 1",
			"city":           "Bratislava",
			"postal_code":    "81101",
			"country":        "Slovakia",
		},
	}
	rawCreate, _ := json.Marshal(createBody)
	var created struct {
		Invoice invoiceRow `json:"invoice"`
	}
	status := doAuthedJSONRequest(
		t, &http.Client{}, http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices",
		cookies, bytes.NewReader(rawCreate), &created,
	)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d want 201", status)
	}

	// Legacy row stored the statement figure instead of payout CSV Amount.
	if _, err := st.DB.ExecContext(ctx,
		`UPDATE invoices SET amount_total_cents = ? WHERE id = ?`, 5194, created.Invoice.ID); err != nil {
		t.Fatal(err)
	}

	var regen struct {
		Invoice invoiceRow `json:"invoice"`
	}
	status = doAuthedJSONRequest(
		t, &http.Client{}, http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices/"+strconv.FormatInt(created.Invoice.ID, 10)+"/regenerate",
		cookies, nil, &regen,
	)
	if status != http.StatusOK {
		t.Fatalf("regenerate status=%d want 200", status)
	}
	if regen.Invoice.AmountTotalCents != 5689 {
		t.Fatalf("regenerate amount=%d want 5689", regen.Invoice.AmountTotalCents)
	}
}
