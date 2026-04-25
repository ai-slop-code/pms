package nuki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListKeypadCodes_MergesRowsByExternalIDPreferringValidity(t *testing.T) {
	from := time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path + "?" + r.URL.RawQuery {
		case "/smartlock/1/auth?":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "abc", "name": "Booking-Maros2", "enabled": true},
			})
		case "/smartlock/1/auth?enabled=true":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":               "abc",
					"name":             "Booking-Maros2",
					"enabled":          true,
					"allowedFromDate":  from.Format(time.RFC3339),
					"allowedUntilDate": until.Format(time.RFC3339),
				},
			})
		default:
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
		}
	}))
	defer srv.Close()

	c := &httpClient{
		baseURL: srv.URL,
		http:    srv.Client(),
	}
	rows, err := c.ListKeypadCodes(context.Background(), Credentials{APIToken: "x", SmartLockID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].ExternalID != "abc" {
		t.Fatalf("external=%q want abc", rows[0].ExternalID)
	}
	if rows[0].ValidFrom == nil || rows[0].ValidUntil == nil {
		t.Fatalf("expected validity window to be merged from richer variant")
	}
}
