package nuki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestListKeypadCodes_UsesCanonicalEndpoint(t *testing.T) {
	from := time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/smartlock/1/auth" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected inventory request %s", r.URL.RequestURI())
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{
			"id": "abc", "name": "Booking-Maros2", "enabled": true,
			"allowedFromDate":  from.Format(time.RFC3339),
			"allowedUntilDate": until.Format(time.RFC3339),
		}})
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

func TestListSmartlockEvents_UsesRFC3339AndIDCursor(t *testing.T) {
	rows := make([]map[string]interface{}, 0, 51)
	for i := 0; i < 51; i++ {
		rows = append(rows, map[string]interface{}{
			"id": strconv.Itoa(i + 1), "date": time.Date(2026, 9, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"authId": "8200", "name": "unlock",
		})
	}
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if strings.Contains(q.Get("fromDate"), "1970") || q.Get("fromDate") == "" || q.Get("toDate") == "" {
			t.Errorf("invalid date bounds: %s", r.URL.RawQuery)
		}
		if q.Get("limit") != "50" || q.Get("offset") != "" {
			t.Errorf("unexpected paging query: %s", r.URL.RawQuery)
		}
		start := 0
		if cursor := q.Get("id"); cursor != "" {
			v, _ := strconv.Atoi(cursor)
			start = v
		}
		end := start + 50
		if end > len(rows) {
			end = len(rows)
		}
		_ = json.NewEncoder(w).Encode(rows[start:end])
	}))
	defer srv.Close()

	c := &httpClient{baseURL: srv.URL, http: srv.Client(), logFetchLimit: 500, logMaxPages: 10}
	events, err := c.ListSmartlockEvents(context.Background(), Credentials{APIToken: "x", SmartLockID: "1"}, time.Date(2026, 8, 31, 0, 0, 0, 123, time.UTC), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 51 || requests != 3 {
		t.Fatalf("events=%d requests=%d", len(events), requests)
	}
}
