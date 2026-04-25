package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
)

// setupNukiPinFixture provisions an owner user, a property, one occupancy, a
// generated Nuki access code with a plaintext PIN, and returns everything the
// test needs to exercise reveal-PIN paths end-to-end.
func setupNukiPinFixture(t *testing.T) (*store.Store, *httptest.Server, int64, int64, string, string) {
	t.Helper()
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
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
	occRunID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, &store.Occupancy{
		PropertyID: prop.ID, SourceType: "booking_ics", SourceEventUID: "nuki-pin-occ", StartAt: start, EndAt: end, Status: "active", RawSummary: sql.NullString{String: "Guest X", Valid: true}, ContentHash: "h1",
	}, occRunID); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOccupancySyncRun(ctx, occRunID, "success", nil, nil, 1, 1); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "nuki-pin-occ")
	if err != nil || occ == nil {
		t.Fatalf("expected occupancy, err=%v", err)
	}
	nukiRunID, err := st.StartNukiSyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &store.NukiAccessCode{
		PropertyID:        prop.ID,
		OccupancyID:       occ.ID,
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
	code, err := st.GetNukiCodeByOccupancyID(ctx, prop.ID, occ.ID)
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
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
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

// ----- H2 occupancy-export header vs. query token -----

func setupOccupancyExportFixture(t *testing.T) (*httptest.Server, int64, string) {
	t.Helper()
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "export-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Villa Export", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	raw, h, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateOccupancyAPIToken(ctx, prop.ID, h, nil); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, prop.ID, raw
}

func TestOccupancyExport_AuthorizationBearerSucceeds(t *testing.T) {
	ts, pid, tok := setupOccupancyExportFixture(t)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/properties/%d/occupancy-export", ts.URL, pid), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	if res.Header.Get("Warning") != "" {
		t.Fatalf("header path must not emit a deprecation Warning header, got %q", res.Header.Get("Warning"))
	}
	var payload struct {
		Occupancies []interface{} `json:"occupancies"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
}

func TestOccupancyExport_XExportTokenSucceeds(t *testing.T) {
	ts, pid, tok := setupOccupancyExportFixture(t)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/properties/%d/occupancy-export", ts.URL, pid), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Export-Token", tok)
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
}

func TestOccupancyExport_QueryTokenEmitsWarning(t *testing.T) {
	ts, pid, tok := setupOccupancyExportFixture(t)
	url := fmt.Sprintf("%s/api/properties/%d/occupancy-export?token=%s", ts.URL, pid, tok)
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (legacy ?token= must be rejected per PMS_11/T2.6)", res.StatusCode)
	}
}

func TestOccupancyExport_NoTokenReturns401(t *testing.T) {
	ts, pid, _ := setupOccupancyExportFixture(t)
	res, err := http.Get(fmt.Sprintf("%s/api/properties/%d/occupancy-export", ts.URL, pid))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", res.StatusCode)
	}
}
