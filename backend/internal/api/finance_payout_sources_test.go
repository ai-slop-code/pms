package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
)

// TestListBookingPayouts_ExposesSourceFlags verifies that the GET
// /finance/booking-payouts response carries per-row provenance signals
// (has_payout_data / has_statement_data) so the UI can render the Sources
// column with badges per FEAT-06.
func TestListBookingPayouts_ExposesSourceFlags(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "owner-sources@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Sources", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}

	mk := func(ref string, payoutDay int) {
		t.Helper()
		if err := st.CreateBookingPayout(ctx, &store.FinanceBookingPayout{
			PropertyID:      prop.ID,
			ReferenceNumber: ref,
			NetCents:        1000,
			PayoutDate:      time.Date(2026, 2, payoutDay, 10, 0, 0, 0, time.UTC),
			CheckInDate:     sql.NullString{String: "2026-02-01", Valid: true},
			CheckOutDate:    sql.NullString{String: "2026-02-02", Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("REF-PAYOUT-ONLY", 5)
	mk("REF-STATEMENT-ONLY", 6)
	mk("REF-MERGED", 7)

	if _, err := st.DB.ExecContext(ctx,
		`UPDATE finance_bookings SET has_payout_data = 1 WHERE reference_number = ?`,
		"REF-PAYOUT-ONLY"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx,
		`UPDATE finance_bookings SET has_statement_data = 1 WHERE reference_number = ?`,
		"REF-STATEMENT-ONLY"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx,
		`UPDATE finance_bookings SET has_payout_data = 1, has_statement_data = 1 WHERE reference_number = ?`,
		"REF-MERGED"); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "owner-sources@example.com", "secret123")
	req, _ := http.NewRequest(http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/booking-payouts?month=2026-02",
		nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d want 200 body=%s", res.StatusCode, string(raw))
	}

	var payload struct {
		Payouts []struct {
			ReferenceNumber  string `json:"reference_number"`
			HasPayoutData    bool   `json:"has_payout_data"`
			HasStatementData bool   `json:"has_statement_data"`
		} `json:"payouts"`
	}
	raw, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(raw))
	}
	want := map[string][2]bool{
		"REF-PAYOUT-ONLY":    {true, false},
		"REF-STATEMENT-ONLY": {false, true},
		"REF-MERGED":         {true, true},
	}
	got := map[string][2]bool{}
	for _, p := range payload.Payouts {
		got[p.ReferenceNumber] = [2]bool{p.HasPayoutData, p.HasStatementData}
	}
	for ref, exp := range want {
		if got[ref] != exp {
			t.Errorf("%s flags=%v want %v", ref, got[ref], exp)
		}
	}
}
