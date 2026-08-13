package occupancy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

func newTestStore(t *testing.T) *store.Store {
	return &store.Store{DB: testutil.OpenTestDB(t)}
}

func createPropertyWithICS(t *testing.T, st *store.Store, url string) int64 {
	t.Helper()
	ctx := context.Background()
	hash := testutil.FastPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "owner@sync.test", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "SyncTest", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertySecrets(ctx, p.ID, &url, nil, nil); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestSyncPropertyWritesCanonicalRawStateOnly(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:uid-1@t
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260525
SUMMARY:Initial
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(payload, "\n", "\r\n")))
	}))
	defer srv.Close()

	propertyID := createPropertyWithICS(t, st, srv.URL)
	if err := (&Service{Store: st, HTTP: srv.Client()}).SyncProperty(context.Background(), propertyID, "manual"); err != nil {
		t.Fatal(err)
	}
	var summary, status string
	var nights int
	if err := st.DB.QueryRow(`SELECT raw_summary, status FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = 'uid-1@t'`, propertyID).Scan(&summary, &status); err != nil {
		t.Fatal(err)
	}
	if summary != "Initial" || status != "active" {
		t.Fatalf("raw block summary/status=%q/%q", summary, status)
	}
	if err := st.DB.QueryRow(`
		SELECT COUNT(*) FROM raw_booking_block_nights n
		JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id
		WHERE b.property_id = ? AND n.active = 1`, propertyID).Scan(&nights); err != nil {
		t.Fatal(err)
	}
	if nights != 3 {
		t.Fatalf("raw nights=%d want 3", nights)
	}
	var legacyTables int
	if err := st.DB.QueryRow(`
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN ('occupancies', 'occupancy_nights')`).Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatalf("legacy occupancy tables=%d want 0", legacyTables)
	}
}

func TestSyncPropertyParseErrorsApplyNoMutation(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:valid-1@t
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260525
END:VEVENT
BEGIN:VEVENT
UID:broken-1@t
SUMMARY:Missing DTSTART
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(payload, "\n", "\r\n")))
	}))
	defer srv.Close()
	propertyID := createPropertyWithICS(t, st, srv.URL)
	ctx := context.Background()
	if err := (&Service{Store: st, HTTP: srv.Client()}).SyncProperty(ctx, propertyID, "manual"); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListOccupancySyncRuns(ctx, propertyID, 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("sync runs=%v err=%v", runs, err)
	}
	if runs[0].Status != StatusPartialNoMutation {
		t.Fatalf("status=%q want %q", runs[0].Status, StatusPartialNoMutation)
	}
	var rawBlocks int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_blocks WHERE property_id = ?`, propertyID).Scan(&rawBlocks); err != nil {
		t.Fatal(err)
	}
	if rawBlocks != 0 {
		t.Fatalf("partial parse wrote %d raw blocks", rawBlocks)
	}
}

func TestSyncPropertyPropertyLeaseSkipsConcurrentSync(t *testing.T) {
	st := newTestStore(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	requests := 0
	payload := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:lease@t
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260523
END:VEVENT
END:VCALENDAR
`, "\n", "\r\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	propertyID := createPropertyWithICS(t, st, srv.URL)
	service := &Service{Store: st, HTTP: srv.Client()}
	ctx := context.Background()
	firstDone := make(chan error, 1)
	go func() { firstDone <- service.SyncProperty(ctx, propertyID, "manual") }()
	<-started
	if err := service.SyncProperty(ctx, propertyID, "manual"); err == nil || !strings.Contains(err.Error(), "sync_already_running") {
		close(release)
		t.Fatalf("second sync error=%v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("requests=%d want 1", requests)
	}
}
