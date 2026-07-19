package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/store"
)

// seedGuestHeatmapFixtures provisions a property with two guest unlock
// rows on the same day at distinct hours, plus one row outside the
// requested range, so the handler test can verify both bucketing and
// range filtering.
func seedGuestHeatmapFixtures(t *testing.T) (string, []*http.Cookie, int64) {
	t.Helper()
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "guest-heatmap-owner@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Heatmap Prop", "UTC", "en")
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
		SourceEventUID: "uid-h",
		StartAt:        time.Date(2026, 4, 9, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		ContentHash:    "h",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, prop.ID, "uid-h")
	if err != nil {
		t.Fatal(err)
	}
	nowText := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, source_channel, source_reference, review_status, nuki_generation_status, created_at, updated_at)
		VALUES (?, 'Heatmap Guest', 'booking_com', '2026-04-09', '2026-04-12', 'active', 1, 'booking_ics', 'uid-h', 'confirmed', 'generated', ?, ?)`, prop.ID, nowText, nowText)
	if err != nil {
		t.Fatal(err)
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO occupancy_stay_migration_map (old_occupancy_id, property_id, named_stay_id, migration_kind, notes, created_at)
		VALUES (?, ?, ?, 'named_stay', 'test_fixture', ?)`, row.ID, prop.ID, stayID, nowText); err != nil {
		t.Fatal(err)
	}

	// Two unlock rows in range (different days so each gets a row), plus
	// one outside the range that must be excluded.
	insert := func(day string, ts time.Time) {
		t.Helper()
		if err := st.UpsertNukiGuestDailyEntry(ctx, &store.NukiGuestDailyEntry{
			PropertyID:         prop.ID,
			OccupancyID:        sql.NullInt64{Int64: row.ID, Valid: true},
			DayDate:            day,
			FirstEntryAt:       ts,
			NukiEventReference: sql.NullString{String: "evt-" + day, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
	insert("2026-04-10", time.Date(2026, 4, 10, 14, 5, 0, 0, time.UTC))
	insert("2026-04-11", time.Date(2026, 4, 11, 18, 0, 0, 0, time.UTC))
	insert("2026-03-15", time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC))

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "guest-heatmap-owner@example.com", "secret123")
	return ts.URL, cookies, prop.ID
}

func TestGetAnalyticsGuestCheckinHeatmap_BucketsAndRange(t *testing.T) {
	base, cookies, pid := seedGuestHeatmapFixtures(t)
	url := base + "/api/properties/" + strconv.FormatInt(pid, 10) +
		"/analytics/guest-checkin-heatmap?from=2026-04-01&to=2026-04-30"

	var payload struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Buckets []struct {
			Hour  int `json:"hour"`
			Count int `json:"count"`
		} `json:"buckets"`
	}
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodGet, url, cookies, nil, &payload)
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if len(payload.Buckets) != 24 {
		t.Fatalf("buckets=%d want 24", len(payload.Buckets))
	}
	for i, b := range payload.Buckets {
		if b.Hour != i {
			t.Fatalf("buckets[%d].hour=%d want %d", i, b.Hour, i)
		}
	}
	if payload.Buckets[14].Count != 1 {
		t.Fatalf("hour 14 count=%d want 1", payload.Buckets[14].Count)
	}
	if payload.Buckets[18].Count != 1 {
		t.Fatalf("hour 18 count=%d want 1", payload.Buckets[18].Count)
	}
	if payload.Buckets[9].Count != 0 {
		t.Fatalf("hour 9 count=%d want 0 (out-of-range row must be excluded)", payload.Buckets[9].Count)
	}
}

func TestGetAnalyticsGuestCheckinHeatmap_RejectsBadRange(t *testing.T) {
	base, cookies, pid := seedGuestHeatmapFixtures(t)
	url := base + "/api/properties/" + strconv.FormatInt(pid, 10) +
		"/analytics/guest-checkin-heatmap?from=2026-04-30&to=2026-04-01"
	status := doAuthedJSONRequest(t, &http.Client{}, http.MethodGet, url, cookies, nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 for inverted range", status)
	}
}

func TestGetAnalyticsGuestCheckinHeatmap_RequiresAuth(t *testing.T) {
	base, _, pid := seedGuestHeatmapFixtures(t)
	url := base + "/api/properties/" + strconv.FormatInt(pid, 10) +
		"/analytics/guest-checkin-heatmap"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 without cookies", resp.StatusCode)
	}
}
