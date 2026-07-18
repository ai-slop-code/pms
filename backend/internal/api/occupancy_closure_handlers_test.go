package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/cleaningcalendar"
	"pms/backend/internal/store"
)

type failingCleaningCalendarClient struct{}

func (f failingCleaningCalendarClient) Configured() bool { return true }

func (f failingCleaningCalendarClient) ListEvents(ctx context.Context, calendarID string, timeMin, timeMax time.Time) ([]cleaningcalendar.GoogleCalendarEvent, error) {
	return nil, nil
}

func (f failingCleaningCalendarClient) UpsertEvent(ctx context.Context, event cleaningcalendar.CalendarEventPayload, googleEventID string) (string, error) {
	return "", errors.New("unexpected upsert")
}

func (f failingCleaningCalendarClient) DeleteEvent(ctx context.Context, calendarID, googleEventID string) error {
	return errors.New("google delete failed")
}

// seedClosureFixtures creates a property + one upserted occupancy and returns
// (server URL, login cookies, propertyID, occupancyID).
func seedClosureFixtures(t *testing.T) (string, []*http.Cookie, int64, int64) {
	t.Helper()
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "closure-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Closure Prop", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	occ := &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "uid-handler",
		StartAt:        time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		RawSummary:     sql.NullString{String: "X", Valid: true},
		ContentHash:    "h",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "uid-handler")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)

	cookies := loginCookies(t, ts.URL, "closure-owner@example.com", "secret123")
	return ts.URL, cookies, prop.ID, row.ID
}

func closureURL(base string, pid, occID int64, suffix string) string {
	return base + "/api/properties/" + strconv.FormatInt(pid, 10) +
		"/occupancies/" + strconv.FormatInt(occID, 10) + "/" + suffix
}

func TestPostOccupancyClose_HappyPath(t *testing.T) {
	base, cookies, pid, occID := seedClosureFixtures(t)
	body := strings.NewReader(`{"reason":"owner trip","category":"owner_stay"}`)
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "close"), cookies, body, nil)
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("close status=%d want 2xx", status)
	}

	// Second close → 409 ErrOccupancyAlreadyLabelled.
	body2 := strings.NewReader(`{"reason":"again","category":"owner_stay"}`)
	status = doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "close"), cookies, body2, nil)
	if status != http.StatusConflict {
		t.Fatalf("re-close status=%d want 409", status)
	}
}

func TestPostOccupancyClose_InvalidCategory(t *testing.T) {
	base, cookies, pid, occID := seedClosureFixtures(t)
	body := strings.NewReader(`{"reason":"x","category":"bogus"}`)
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "close"), cookies, body, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestPostOccupancyExternalSale_HappyPathAndReopen(t *testing.T) {
	base, cookies, pid, occID := seedClosureFixtures(t)
	body, _ := json.Marshal(map[string]interface{}{
		"net_amount_cents": 12000,
		"currency":         "EUR",
		"channel":          "airbnb",
		"reason":           "Airbnb walk-in",
	})
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "external-sale"), cookies, bytes.NewReader(body), nil)
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("external-sale status=%d want 2xx", status)
	}

	// Reopen.
	status = doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "reopen"), cookies, nil, nil)
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Fatalf("reopen status=%d want 2xx", status)
	}

	// Reopening an unlabelled row → 404.
	status = doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "reopen"), cookies, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("reopen-unlabelled status=%d want 404", status)
	}
}

func TestPostOccupancyExternalSale_NegativeAmountRejected(t *testing.T) {
	base, cookies, pid, occID := seedClosureFixtures(t)
	body := strings.NewReader(`{"net_amount_cents":-1,"currency":"EUR","channel":"direct"}`)
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, occID, "external-sale"), cookies, body, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
}

func TestPostOccupancyClose_UnknownIDReturns404(t *testing.T) {
	base, cookies, pid, _ := seedClosureFixtures(t)
	body := strings.NewReader(`{"reason":"x","category":"owner_stay"}`)
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(base, pid, 999999, "close"), cookies, body, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status=%d want 404", status)
	}
}

func TestPostOccupancyClose_RequiresAuth(t *testing.T) {
	base, _, pid, occID := seedClosureFixtures(t)
	body := strings.NewReader(`{"reason":"x","category":"owner_stay"}`)
	req, _ := http.NewRequest(http.MethodPost, closureURL(base, pid, occID, "close"), body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PMS-Client", "test")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", res.StatusCode)
	}
}

func TestPostOccupancyCleaningCalendarExclude_PartialFailure(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "cleaning-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Cleaning Prop", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	calendarID := "cleaning@example.com"
	if _, err := st.UpdateGoogleCleaningSettings(ctx, prop.ID, store.CleaningCalendarSettingsPatch{Enabled: &enabled, CalendarID: &calendarID}); err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, prop.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	occ := &store.Occupancy{
		PropertyID:     prop.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "uid-cleaning-handler",
		StartAt:        time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		RawSummary:     sql.NullString{String: "X", Valid: true},
		ContentHash:    "h",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "uid-cleaning-handler")
	if err != nil || row == nil {
		t.Fatalf("get occupancy: %v", err)
	}
	if _, err := st.UpsertCleaningCalendarEvent(ctx, &store.CleaningCalendarEvent{
		PropertyID:       prop.ID,
		OccupancyID:      row.ID,
		GoogleCalendarID: calendarID,
		GoogleEventID:    sql.NullString{String: "google-event", Valid: true},
		CleaningDate:     "2026-06-12",
		StartsAt:         time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		EndsAt:           time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC),
		Title:            "Upratovanie: Bez Hosta",
		Status:           store.CleaningCalendarStatusSynced,
	}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		Store:      st,
		SessionTTL: time.Hour,
		CleaningCalendar: &cleaningcalendar.Service{
			Store:  st,
			Client: failingCleaningCalendarClient{},
			Now:    func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) },
		},
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "cleaning-owner@example.com", "secret123")
	var res actionResponse
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodPost,
		closureURL(ts.URL, prop.ID, row.ID, "cleaning-calendar/exclude"), cookies,
		strings.NewReader(`{"reason":"owner will clean"}`), &res)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if res.OK || !strings.Contains(res.Error, "cleaning calendar exclusion saved, cleaning calendar failed: google delete failed") {
		t.Fatalf("response=%+v", res)
	}
	row, err = st.GetOccupancyByID(ctx, prop.ID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.CleaningCalendarExcluded {
		t.Fatal("exclusion was not saved before reconciliation failure")
	}
}
