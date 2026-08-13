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

	"pms/backend/internal/store"
)

func TestFinanceRevenueRecognitionEndpoint_ReturnsProratedGross(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "revenue-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Revenue", "Europe/Bratislava", "en")
	if err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: prop.ID, DisplayName: "Revenue Guest", StayType: store.StayTypeBookingCom,
		CheckInDate: "2026-01-31", CheckOutDate: "2026-02-03", SourceReference: "REVENUE-API",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateBookingPayout(ctx, &store.FinanceBookingPayout{
		PropertyID:      prop.ID,
		ReferenceNumber: "REVENUE-API",
		GuestName:       sql.NullString{String: "Revenue Guest", Valid: true},
		CheckInDate:     sql.NullString{String: "2026-01-31", Valid: true},
		CheckOutDate:    sql.NullString{String: "2026-02-03", Valid: true},
		AmountCents:     sql.NullInt64{Int64: 10000, Valid: true},
		NetCents:        8000,
		PayoutDate:      time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		NamedStayID:     sql.NullInt64{Int64: stay.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE finance_bookings SET has_payout_data = 1 WHERE property_id = ? AND reference_number = 'REVENUE-API'`, prop.ID); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "revenue-owner@example.com", "secret123")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/revenue-recognition?month=2026-02", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", res.StatusCode, string(raw))
	}
	var payload struct {
		GrossRevenueCents int `json:"gross_revenue_cents"`
		Bookings          []struct {
			ReferenceNumber      string `json:"reference_number"`
			RecognizedGrossCents int    `json:"recognized_gross_cents"`
			Unmatched            bool   `json:"unmatched"`
		} `json:"bookings"`
		Excluded []json.RawMessage `json:"excluded_bookings"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.GrossRevenueCents != 6666 || len(payload.Bookings) != 1 || payload.Bookings[0].RecognizedGrossCents != 6666 || payload.Bookings[0].Unmatched {
		t.Fatalf("unexpected response: %+v", payload)
	}
	if payload.Excluded == nil {
		t.Fatal("excluded_bookings must be an array")
	}
}

func TestFinanceRevenueRecognitionEndpoint_RejectsMalformedMonth(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "revenue-month@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Revenue Month", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "revenue-month@example.com", "secret123")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/revenue-recognition?month=2026-1", nil)
	req.Header.Set("X-PMS-Client", "test")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d want 400 body=%s", res.StatusCode, string(raw))
	}
}
