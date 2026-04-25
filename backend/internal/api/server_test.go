package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/auth"
	"pms/backend/internal/nuki"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

func testDB(t *testing.T) *store.Store {
	return &store.Store{DB: testutil.OpenTestDB(t)}
}

func loginCookies(t *testing.T, baseURL, email, password string) []*http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", res.StatusCode)
	}
	return res.Cookies()
}

func doAuthedJSONRequest(t *testing.T, client *http.Client, method, url string, cookies []*http.Cookie, body io.Reader, out interface{}) int {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, string(raw))
		}
	}
	return res.StatusCode
}

func TestLoginLogoutAndMe(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "a@example.com", hash, "owner"); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	body := map[string]string{"email": "a@example.com", "password": "secret123"}
	b, _ := json.Marshal(body)
	loginReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader(b))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("X-PMS-Client", "test")
	res, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", res.StatusCode)
	}
	var cookies []*http.Cookie
	for _, c := range res.Cookies() {
		cookies = append(cookies, c)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	client := &http.Client{}
	meRes, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer meRes.Body.Close()
	if meRes.StatusCode != http.StatusOK {
		t.Fatalf("me status %d", meRes.StatusCode)
	}

	logReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/logout", nil)
	logReq.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		logReq.AddCookie(c)
	}
	logRes, err := client.Do(logReq)
	if err != nil {
		t.Fatal(err)
	}
	defer logRes.Body.Close()
	if logRes.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status %d", logRes.StatusCode)
	}
}

func TestProtectedRouteWithoutAuth(t *testing.T) {
	st := testDB(t)
	srv := &Server{Store: st, SessionTTL: time.Hour}
	r := chi.NewRouter()
	r.Mount("/", srv.Routes())
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDashboardSummary_IncludesOnlyAuthorizedWidgets(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "dashboard-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Dashboard Test", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertySecrets(ctx, prop.ID, strPtr("https://example.com/calendar.ics"), strPtr("nuki-token"), strPtr("123456")); err != nil {
		t.Fatal(err)
	}
	occRunID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	end := start.Add(48 * time.Hour)
	if err := st.UpsertOccupancy(ctx, &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "dashboard-occ-1",
		StartAt:        start,
		EndAt:          end,
		Status:         "active",
		RawSummary:     sql.NullString{String: "Guest One", Valid: true},
		ContentHash:    "dashboard-occ-1",
	}, occRunID); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOccupancySyncRun(ctx, occRunID, "success", nil, nil, 1, 1); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "dashboard-occ-1")
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
		CodeLabel:         "booking-guest-one",
		AccessCodeMasked:  sql.NullString{String: "12**", Valid: true},
		GeneratedPINPlain: sql.NullString{String: "1234", Valid: true},
		ExternalNukiID:    sql.NullString{String: "nuki-ext-1", Valid: true},
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
	if err := st.CreateCleanerFeeHistoryRow(ctx, prop.ID, 3500, 500, time.Now().UTC().Add(-24*time.Hour), &owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCleaningDailyLog(ctx, &store.CleaningDailyLog{
		PropertyID:         prop.ID,
		DayDate:            time.Now().UTC().Format("2006-01-02"),
		FirstEntryAt:       sql.NullTime{Time: time.Now().UTC(), Valid: true},
		NukiEventReference: sql.NullString{String: "evt-1", Valid: true},
		CountedForSalary:   true,
	}); err != nil {
		t.Fatal(err)
	}
	cat, err := st.CreateFinanceCategory(ctx, prop.ID, "booking_income", "Booking Income", "incoming", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFinanceTransaction(ctx, &store.FinanceTransaction{
		PropertyID:      prop.ID,
		TransactionDate: time.Now().UTC(),
		Direction:       "incoming",
		AmountCents:     12345,
		CategoryID:      sql.NullInt64{Int64: cat.ID, Valid: true},
		Note:            sql.NullString{String: "dashboard", Valid: true},
		SourceType:      "manual",
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "dashboard-owner@example.com", "secret123")
	client := &http.Client{}
	var payload struct {
		Widgets map[string]json.RawMessage `json:"widgets"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodGet, ts.URL+"/api/dashboard/summary?property_id="+strconv.FormatInt(prop.ID, 10), cookies, nil, &payload)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	for _, key := range []string{"sync_status", "upcoming_stays", "active_nuki_codes", "cleaning_month", "finance_month"} {
		if _, ok := payload.Widgets[key]; !ok {
			t.Fatalf("expected widget %q to be present", key)
		}
	}
	var syncStatus map[string]string
	if err := json.Unmarshal(payload.Widgets["sync_status"], &syncStatus); err != nil {
		t.Fatal(err)
	}
	if syncStatus["occupancy"] != "ok" || syncStatus["nuki"] != "ok" {
		t.Fatalf("unexpected sync status %#v", syncStatus)
	}
	var upcoming []map[string]interface{}
	if err := json.Unmarshal(payload.Widgets["upcoming_stays"], &upcoming); err != nil {
		t.Fatal(err)
	}
	if len(upcoming) != 1 {
		t.Fatalf("upcoming stays len=%d want 1", len(upcoming))
	}
	var activeCodes []map[string]interface{}
	if err := json.Unmarshal(payload.Widgets["active_nuki_codes"], &activeCodes); err != nil {
		t.Fatal(err)
	}
	if len(activeCodes) != 1 {
		t.Fatalf("active nuki codes len=%d want 1", len(activeCodes))
	}
	var cleaningWidget map[string]interface{}
	if err := json.Unmarshal(payload.Widgets["cleaning_month"], &cleaningWidget); err != nil {
		t.Fatal(err)
	}
	if got := int(cleaningWidget["counted_days"].(float64)); got != 1 {
		t.Fatalf("counted_days=%d want 1", got)
	}
	var financeWidget map[string]interface{}
	if err := json.Unmarshal(payload.Widgets["finance_month"], &financeWidget); err != nil {
		t.Fatal(err)
	}
	if got := int(financeWidget["net"].(float64)); got != 12345 {
		t.Fatalf("finance net=%d want 12345", got)
	}
}

func TestDashboardSummary_OmitsUnauthorizedWidgets(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "dashboard-owner-2@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := st.CreateUser(ctx, "dashboard-manager@example.com", hash, "property_manager")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Dashboard Restricted", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertPropertyPermission(ctx, manager.ID, prop.ID, permissions.Finance, permissions.LevelRead); err != nil {
		t.Fatal(err)
	}
	cat, err := st.CreateFinanceCategory(ctx, prop.ID, "rent", "Rent", "incoming", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFinanceTransaction(ctx, &store.FinanceTransaction{
		PropertyID:      prop.ID,
		TransactionDate: time.Now().UTC(),
		Direction:       "incoming",
		AmountCents:     9900,
		CategoryID:      sql.NullInt64{Int64: cat.ID, Valid: true},
		SourceType:      "manual",
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "dashboard-manager@example.com", "secret123")
	client := &http.Client{}
	var payload struct {
		Widgets map[string]json.RawMessage `json:"widgets"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodGet, ts.URL+"/api/dashboard/summary?property_id="+strconv.FormatInt(prop.ID, 10), cookies, nil, &payload)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if _, ok := payload.Widgets["finance_month"]; !ok {
		t.Fatalf("expected finance_month widget")
	}
	for _, key := range []string{"sync_status", "upcoming_stays", "active_nuki_codes", "cleaning_month"} {
		if _, ok := payload.Widgets[key]; ok {
			t.Fatalf("did not expect widget %q", key)
		}
	}
}

func TestInvoices_CreateAndRegenerateVersionedPDF(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "invoice-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Invoice Test", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name":    "Invoice Test s.r.o.",
		"billing_address": "Main 1",
		"city":            "Bratislava",
		"postal_code":     "81101",
		"country":         "Slovakia",
		"ico":             "12345678",
		"dic":             "87654321",
	}); err != nil {
		t.Fatal(err)
	}

	dataDir := t.TempDir()
	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: dataDir}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "invoice-owner@example.com", "secret123")
	client := &http.Client{}
	createBody := map[string]interface{}{
		"language":            "en",
		"issue_date":          "2026-04-15",
		"taxable_supply_date": "2026-04-15",
		"due_date":            "2026-04-15",
		"stay_start_date":     "2026-04-20",
		"stay_end_date":       "2026-04-24",
		"amount_total_cents":  45678,
		"payment_note":        "Already paid via Booking.com.",
		"customer": map[string]string{
			"name":           "John Guest",
			"address_line_1": "Customer Street 2",
			"city":           "Prague",
			"postal_code":    "11000",
			"country":        "Czechia",
		},
	}
	rawCreate, _ := json.Marshal(createBody)
	var created struct {
		Invoice invoiceRow `json:"invoice"`
	}
	status := doAuthedJSONRequest(
		t,
		client,
		http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices",
		cookies,
		bytes.NewReader(rawCreate),
		&created,
	)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d want 201", status)
	}
	if created.Invoice.InvoiceNumber == "" {
		t.Fatalf("expected invoice number")
	}
	if created.Invoice.Version != 1 {
		t.Fatalf("version=%d want 1", created.Invoice.Version)
	}
	if created.Invoice.LatestFilePath == nil || *created.Invoice.LatestFilePath == "" {
		t.Fatalf("expected latest file path")
	}
	if _, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(*created.Invoice.LatestFilePath))); err != nil {
		t.Fatalf("expected invoice file to exist: %v", err)
	}

	var detail struct {
		Invoice invoiceRow `json:"invoice"`
	}
	status = doAuthedJSONRequest(
		t,
		client,
		http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices/"+strconv.FormatInt(created.Invoice.ID, 10),
		cookies,
		nil,
		&detail,
	)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d want 200", status)
	}
	if detail.Invoice.Files == nil || len(*detail.Invoice.Files) != 1 {
		t.Fatalf("expected one invoice file, got %#v", detail.Invoice.Files)
	}

	status = doAuthedJSONRequest(
		t,
		client,
		http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices/"+strconv.FormatInt(created.Invoice.ID, 10)+"/regenerate",
		cookies,
		nil,
		&detail,
	)
	if status != http.StatusOK {
		t.Fatalf("regenerate status=%d want 200", status)
	}
	if detail.Invoice.Version != 2 {
		t.Fatalf("version=%d want 2", detail.Invoice.Version)
	}

	status = doAuthedJSONRequest(
		t,
		client,
		http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices/"+strconv.FormatInt(created.Invoice.ID, 10),
		cookies,
		nil,
		&detail,
	)
	if status != http.StatusOK {
		t.Fatalf("detail after regenerate status=%d want 200", status)
	}
	if detail.Invoice.Files == nil || len(*detail.Invoice.Files) != 2 {
		t.Fatalf("expected two invoice files, got %#v", detail.Invoice.Files)
	}

	req, _ := http.NewRequest(
		http.MethodGet,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices/"+strconv.FormatInt(created.Invoice.ID, 10)+"/download",
		nil,
	)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("download status=%d want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); !strings.Contains(got, "application/pdf") {
		t.Fatalf("content-type=%q want pdf", got)
	}
}

func TestInvoices_DuplicateOccupancyRejected(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "invoice-owner-dup@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Invoice Duplicate", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name":    "Duplicate Test s.r.o.",
		"billing_address": "Main 1",
		"city":            "Bratislava",
		"postal_code":     "81101",
		"country":         "Slovakia",
	}); err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	if err := st.UpsertOccupancy(ctx, &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "invoice-occ-dup",
		StartAt:        start,
		EndAt:          start.Add(24 * time.Hour),
		Status:         "active",
		RawSummary:     sql.NullString{String: "Guest Dup", Valid: true},
		ContentHash:    "invoice-occ-dup",
	}, runID); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "invoice-occ-dup")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "invoice-owner-dup@example.com", "secret123")
	client := &http.Client{}

	create := func() int {
		body := map[string]interface{}{
			"occupancy_id":        occ.ID,
			"language":            "en",
			"issue_date":          "2026-04-15",
			"taxable_supply_date": "2026-04-15",
			"due_date":            "2026-04-15",
			"stay_start_date":     "2026-04-20",
			"stay_end_date":       "2026-04-24",
			"amount_total_cents":  10000,
			"customer": map[string]string{
				"name":           "Guest Dup",
				"address_line_1": "Somewhere 1",
				"city":           "Vienna",
				"postal_code":    "1010",
				"country":        "Austria",
			},
		}
		raw, _ := json.Marshal(body)
		return doAuthedJSONRequest(
			t,
			client,
			http.MethodPost,
			ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices",
			cookies,
			bytes.NewReader(raw),
			&map[string]interface{}{},
		)
	}

	if status := create(); status != http.StatusCreated {
		t.Fatalf("first create status=%d want 201", status)
	}
	if status := create(); status != http.StatusConflict {
		t.Fatalf("second create status=%d want 409", status)
	}
}

func TestInvoices_InvoiceCodePrefixFormatsNumber(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "invoice-code@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Invoice Code Prop", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	code := "APT01"
	if _, err := st.UpdateProperty(ctx, prop.ID, nil, nil, nil, &code, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name":    "Code Test s.r.o.",
		"billing_address": "Main 1",
		"city":            "Bratislava",
		"postal_code":     "81101",
		"country":         "Slovakia",
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "invoice-code@example.com", "secret123")
	client := &http.Client{}

	body := map[string]interface{}{
		"language":            "en",
		"issue_date":          "2026-04-15",
		"taxable_supply_date": "2026-04-15",
		"due_date":            "2026-04-15",
		"stay_start_date":     "2026-04-20",
		"stay_end_date":       "2026-04-24",
		"amount_total_cents":  100,
		"customer": map[string]string{
			"name":           "Guest",
			"address_line_1": "Somewhere 1",
			"city":           "Vienna",
			"postal_code":    "1010",
			"country":        "Austria",
		},
	}
	raw, _ := json.Marshal(body)
	var out struct {
		Invoice invoiceRow `json:"invoice"`
	}
	status := doAuthedJSONRequest(t, client, http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices", cookies, bytes.NewReader(raw), &out)
	if status != http.StatusCreated {
		t.Fatalf("create status=%d want 201", status)
	}
	if !strings.HasPrefix(out.Invoice.InvoiceNumber, "APT01/2026/") {
		t.Fatalf("invoice number=%q want prefix APT01/2026/", out.Invoice.InvoiceNumber)
	}
}

func TestInvoices_DuplicateBookingPayoutRejected(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateUser(ctx, "invoice-payout-dup@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Invoice Payout Dup", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name":    "Payout Dup s.r.o.",
		"billing_address": "Main 1",
		"city":            "Bratislava",
		"postal_code":     "81101",
		"country":         "Slovakia",
	}); err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	if err := st.UpsertOccupancy(ctx, &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "invoice-payout-dup-occ",
		StartAt:        start,
		EndAt:          start.Add(24 * time.Hour),
		Status:         "active",
		RawSummary:     sql.NullString{String: "Guest Payout", Valid: true},
		ContentHash:    "invoice-payout-dup-occ",
	}, runID); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "invoice-payout-dup-occ")
	if err != nil {
		t.Fatal(err)
	}
	payoutDate := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	if err := st.CreateBookingPayout(ctx, &store.FinanceBookingPayout{
		PropertyID:      prop.ID,
		ReferenceNumber: "PAYOUT-DUP-REF-1",
		NetCents:        12345,
		PayoutDate:      payoutDate,
		OccupancyID:     sql.NullInt64{Int64: occ.ID, Valid: true},
		CheckInDate:     sql.NullString{String: "2026-04-20", Valid: true},
		CheckOutDate:    sql.NullString{String: "2026-04-22", Valid: true},
		GuestName:       sql.NullString{String: "Dup Guest", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	payout, err := st.GetBookingPayoutByReference(ctx, prop.ID, "PAYOUT-DUP-REF-1")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "invoice-payout-dup@example.com", "secret123")
	client := &http.Client{}

	post := func() int {
		b := map[string]interface{}{
			"booking_payout_id":   payout.ID,
			"language":            "en",
			"issue_date":          "2026-04-15",
			"taxable_supply_date": "2026-04-15",
			"due_date":            "2026-04-15",
			"stay_start_date":     "2026-04-20",
			"stay_end_date":       "2026-04-22",
			"amount_total_cents":  12345,
			"customer": map[string]string{
				"name":           "Dup Guest",
				"address_line_1": "Somewhere 1",
				"city":           "Vienna",
				"postal_code":    "1010",
				"country":        "Austria",
			},
		}
		raw, _ := json.Marshal(b)
		return doAuthedJSONRequest(t, client, http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/invoices", cookies, bytes.NewReader(raw), &map[string]interface{}{})
	}
	if status := post(); status != http.StatusCreated {
		t.Fatalf("first create status=%d want 201", status)
	}
	if status := post(); status != http.StatusConflict {
		t.Fatalf("second create status=%d want 409", status)
	}
}

func strPtr(v string) *string {
	return &v
}

func TestGenerateNukiCode_RequiresPinName(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "owner-nuki@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Nuki Test", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour, Nuki: &nuki.Service{Store: st}}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	loginBody, _ := json.Marshal(map[string]string{"email": "owner-nuki@example.com", "password": "secret123"})
	loginReqN, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader(loginBody))
	loginReqN.Header.Set("Content-Type", "application/json")
	loginReqN.Header.Set("X-PMS-Client", "test")
	loginRes, err := http.DefaultClient.Do(loginReqN)
	if err != nil {
		t.Fatal(err)
	}
	defer loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", loginRes.StatusCode)
	}

	reqBody := strings.NewReader(`{"occupancy_id":123}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/nuki/codes/generate", reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range loginRes.Cookies() {
		req.AddCookie(c)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", res.StatusCode)
	}
	raw, _ := io.ReadAll(res.Body)
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got true")
	}
	if got, _ := payload["error"].(string); got != "pin_name required" {
		t.Fatalf("error=%q want pin_name required", got)
	}
}

func TestSaveNukiStayName_PersistsAndReflectsInOccupancyList(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "owner-stayname@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Stay Name Test", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	occ := &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "uid-stay-name-1",
		StartAt:        time.Now().UTC().Add(24 * time.Hour),
		EndAt:          time.Now().UTC().Add(48 * time.Hour),
		Status:         "active",
		RawSummary:     sql.NullString{String: "ICS Guest", Valid: true},
		ContentHash:    "h-stay-name-1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	savedOcc, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "uid-stay-name-1")
	if err != nil || savedOcc == nil {
		t.Fatalf("occupancy save failed err=%v", err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	loginBody, _ := json.Marshal(map[string]string{"email": "owner-stayname@example.com", "password": "secret123"})
	loginReqS, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader(loginBody))
	loginReqS.Header.Set("Content-Type", "application/json")
	loginReqS.Header.Set("X-PMS-Client", "test")
	loginRes, err := http.DefaultClient.Do(loginReqS)
	if err != nil {
		t.Fatal(err)
	}
	defer loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", loginRes.StatusCode)
	}

	patchBody := strings.NewReader(`{"pin_name":"Lubos"}`)
	patchReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/nuki/upcoming-stays/"+strconv.FormatInt(savedOcc.ID, 10),
		patchBody,
	)
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("X-PMS-Client", "test")
	for _, c := range loginRes.Cookies() {
		patchReq.AddCookie(c)
	}
	client := &http.Client{}
	patchRes, err := client.Do(patchReq)
	if err != nil {
		t.Fatal(err)
	}
	defer patchRes.Body.Close()
	if patchRes.StatusCode != http.StatusOK {
		t.Fatalf("patch status=%d want 200", patchRes.StatusCode)
	}
	var patchPayload map[string]interface{}
	rawPatch, _ := io.ReadAll(patchRes.Body)
	if err := json.Unmarshal(rawPatch, &patchPayload); err != nil {
		t.Fatal(err)
	}
	if ok, _ := patchPayload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true in patch response")
	}

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/occupancies?limit=10", nil)
	for _, c := range loginRes.Cookies() {
		listReq.AddCookie(c)
	}
	listRes, err := client.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d want 200", listRes.StatusCode)
	}
	var listPayload struct {
		Occupancies []struct {
			RawSummary string `json:"raw_summary"`
		} `json:"occupancies"`
	}
	rawList, _ := io.ReadAll(listRes.Body)
	if err := json.Unmarshal(rawList, &listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Occupancies) != 1 {
		t.Fatalf("occupancies len=%d want 1", len(listPayload.Occupancies))
	}
	if got := listPayload.Occupancies[0].RawSummary; got != "Lubos" {
		t.Fatalf("raw_summary=%q want Lubos", got)
	}
}

func TestCreateFinanceBookingPayoutStay_CreatesAndMapsOccupancy(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "owner-payout-create@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Payout Create Stay", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateBookingPayout(ctx, &store.FinanceBookingPayout{
		PropertyID:        prop.ID,
		ReferenceNumber:   "REF-CREATE-1",
		CheckInDate:       sql.NullString{String: "2026-01-22", Valid: true},
		CheckOutDate:      sql.NullString{String: "2026-01-23", Valid: true},
		GuestName:         sql.NullString{String: "Fatih Altuntas", Valid: true},
		NetCents:          5819,
		PayoutDate:        time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC),
		TransactionID:     sql.NullInt64{},
		OccupancyID:       sql.NullInt64{},
		AmountCents:       sql.NullInt64{Int64: 7226, Valid: true},
		CommissionCents:   sql.NullInt64{Int64: -1320, Valid: true},
		PaymentStatus:     sql.NullString{String: "paid", Valid: true},
		ReservationStatus: sql.NullString{String: "ok", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	loginBody, _ := json.Marshal(map[string]string{"email": "owner-payout-create@example.com", "password": "secret123"})
	loginReqP, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login", bytes.NewReader(loginBody))
	loginReqP.Header.Set("Content-Type", "application/json")
	loginReqP.Header.Set("X-PMS-Client", "test")
	loginRes, err := http.DefaultClient.Do(loginReqP)
	if err != nil {
		t.Fatal(err)
	}
	defer loginRes.Body.Close()
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", loginRes.StatusCode)
	}

	req, _ := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/booking-payouts/REF-CREATE-1/create-stay",
		nil,
	)
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range loginRes.Cookies() {
		req.AddCookie(c)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("status=%d want 200 body=%s", res.StatusCode, string(raw))
	}
	var payload struct {
		OK          bool  `json:"ok"`
		OccupancyID int64 `json:"occupancy_id"`
		Created     bool  `json:"created"`
	}
	raw, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.OccupancyID <= 0 || !payload.Created {
		t.Fatalf("unexpected response: %+v", payload)
	}

	payout, err := st.GetBookingPayoutByReference(ctx, prop.ID, "REF-CREATE-1")
	if err != nil {
		t.Fatal(err)
	}
	if !payout.OccupancyID.Valid || payout.OccupancyID.Int64 != payload.OccupancyID {
		t.Fatalf("payout occupancy=%v want %d", payout.OccupancyID, payload.OccupancyID)
	}
	occ, err := st.GetOccupancyByID(ctx, prop.ID, payload.OccupancyID)
	if err != nil {
		t.Fatal(err)
	}
	if occ.SourceType != "booking_payout" {
		t.Fatalf("source_type=%q want booking_payout", occ.SourceType)
	}

	// Second call should keep mapping and report created=false.
	req2, _ := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/booking-payouts/REF-CREATE-1/create-stay",
		nil,
	)
	req2.Header.Set("X-PMS-Client", "test")
	for _, c := range loginRes.Cookies() {
		req2.AddCookie(c)
	}
	res2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		raw2, _ := io.ReadAll(res2.Body)
		t.Fatalf("second status=%d want 200 body=%s", res2.StatusCode, string(raw2))
	}
	var payload2 struct {
		OK          bool  `json:"ok"`
		OccupancyID int64 `json:"occupancy_id"`
		Created     bool  `json:"created"`
	}
	raw2, _ := io.ReadAll(res2.Body)
	if err := json.Unmarshal(raw2, &payload2); err != nil {
		t.Fatal(err)
	}
	if !payload2.OK || payload2.OccupancyID != payload.OccupancyID || payload2.Created {
		t.Fatalf("unexpected second response: %+v", payload2)
	}
}
