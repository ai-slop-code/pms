package api

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/store"
)

// setupNukiPinFixture provisions an owner user, a property, one named stay, a
// generated Nuki access code with a plaintext PIN, and returns everything the
// test needs to exercise reveal-PIN paths end-to-end.
func setupNukiPinFixture(t *testing.T) (*store.Store, *httptest.Server, int64, int64, string, string) {
	t.Helper()
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "nuki-pin-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Villa Test", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	end := start.Add(48 * time.Hour)
	nowText := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, source_channel, source_reference, review_status, nuki_generation_status, created_at, updated_at)
		VALUES (?, 'Guest X', 'booking_com', ?, ?, 'active', 1, 'booking_ics', 'nuki-pin-occ', 'confirmed', 'generated', ?, ?)`,
		prop.ID, start.UTC().Format("2006-01-02"), end.UTC().Format("2006-01-02"), nowText, nowText)
	if err != nil {
		t.Fatal(err)
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	nukiRunID, err := st.StartNukiSyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &store.NukiAccessCode{
		PropertyID:        prop.ID,
		NamedStayID:       sql.NullInt64{Int64: stayID, Valid: true},
		CodeLabel:         "booking-pin-reveal",
		AccessCodeMasked:  sql.NullString{String: "98**", Valid: true},
		GeneratedPINPlain: sql.NullString{String: "9876", Valid: true},
		ExternalNukiID:    sql.NullString{String: "nuki-ext-9", Valid: true},
		ValidFrom:         start,
		ValidUntil:        end,
		Status:            "generated",
		LastSyncRunID:     sql.NullInt64{Int64: nukiRunID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishNukiSyncRun(ctx, nukiRunID, "success", nil, 1, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(ctx, prop.ID, stayID)
	if err != nil || code == nil {
		t.Fatalf("expected nuki code, err=%v", err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return st, ts, prop.ID, code.ID, "nuki-pin-owner@example.com", "secret123"
}

func TestListNukiUpcomingStaysDoesNotLeakPIN(t *testing.T) {
	_, ts, pid, _, email, pw := setupNukiPinFixture(t)
	cookies := loginCookies(t, ts.URL, email, pw)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/properties/"+strconv.FormatInt(pid, 10)+"/nuki/upcoming-stays", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("9876")) {
		t.Fatalf("list response leaked plaintext PIN: %s", string(body))
	}
	if bytes.Contains(body, []byte(`"generated_pin"`)) {
		t.Fatalf("list response still includes generated_pin field: %s", string(body))
	}
	if bytes.Contains(body, []byte(`"occupancy_id"`)) || bytes.Contains(body, []byte(`"legacy_occupancy_id"`)) {
		t.Fatalf("list response includes a legacy stay identity: %s", string(body))
	}
	if bytes.Contains(body, []byte(`"occupancy_status"`)) || !bytes.Contains(body, []byte(`"stay_status":"active"`)) {
		t.Fatalf("list response does not expose canonical stay_status: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"generated_masked":"98**"`)) {
		t.Fatalf("expected masked code in response: %s", string(body))
	}
}

func TestRevealNukiCodePIN_OwnerSucceeds(t *testing.T) {
	_, ts, pid, codeID, email, pw := setupNukiPinFixture(t)
	cookies := loginCookies(t, ts.URL, email, pw)
	var payload struct {
		PIN string `json:"pin"`
	}
	url := fmt.Sprintf("%s/api/properties/%d/nuki/codes/%d/reveal-pin", ts.URL, pid, codeID)
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodGet, url, cookies, nil, &payload)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if payload.PIN != "9876" {
		t.Fatalf("pin=%q want 9876", payload.PIN)
	}
}

func TestRevealNukiCodePIN_WritesAuditEntry(t *testing.T) {
	st, ts, pid, codeID, email, pw := setupNukiPinFixture(t)
	cookies := loginCookies(t, ts.URL, email, pw)
	url := fmt.Sprintf("%s/api/properties/%d/nuki/codes/%d/reveal-pin", ts.URL, pid, codeID)
	if status := doAuthedJSONRequest(t, &http.Client{}, http.MethodGet, url, cookies, nil, nil); status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	rows, err := st.DB.QueryContext(
		context.Background(),
		"SELECT action, entity_type, entity_id, outcome FROM api_audit_logs WHERE action = ? AND entity_id = ?",
		"nuki_reveal_pin", strconv.FormatInt(codeID, 10),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		var a, et, eid, outcome string
		if err := rows.Scan(&a, &et, &eid, &outcome); err != nil {
			t.Fatal(err)
		}
		if a != "nuki_reveal_pin" || et != "nuki_access_code" || outcome != "success" {
			t.Fatalf("unexpected audit row: %s/%s/%s/%s", a, et, eid, outcome)
		}
		n++
	}
	if n != 1 {
		t.Fatalf("expected 1 audit row, got %d", n)
	}
}

func TestRevealNukiCodePIN_ReadOnlyUserForbidden(t *testing.T) {
	st, ts, pid, codeID, _, _ := setupNukiPinFixture(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	ro, err := st.CreateUser(ctx, "pin-reader@example.com", hash, "read_only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertPropertyPermission(ctx, ro.ID, pid, "nuki_access", "read"); err != nil {
		t.Fatal(err)
	}
	cookies := loginCookies(t, ts.URL, "pin-reader@example.com", "secret123")
	url := fmt.Sprintf("%s/api/properties/%d/nuki/codes/%d/reveal-pin", ts.URL, pid, codeID)
	if status := doAuthedJSONRequest(t, &http.Client{}, http.MethodGet, url, cookies, nil, nil); status != http.StatusForbidden {
		t.Fatalf("status=%d want 403", status)
	}
}
