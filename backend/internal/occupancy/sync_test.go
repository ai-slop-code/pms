package occupancy

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"pms/backend/internal/auth"
	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

func newTestStore(t *testing.T) *store.Store {
	return &store.Store{DB: testutil.OpenTestDB(t)}
}

func createPropertyWithICS(t *testing.T, st *store.Store, url string) int64 {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
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

func TestSyncProperty_SetsUpdatedStatusWhenExistingStayChanges(t *testing.T) {
	st := newTestStore(t)
	var mu sync.Mutex
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(strings.ReplaceAll(payload, "\n", "\r\n")))
	}))
	defer srv.Close()

	pid := createPropertyWithICS(t, st, srv.URL)
	svc := &Service{Store: st, HTTP: srv.Client()}
	ctx := context.Background()

	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	o1, err := st.GetOccupancyBySourceEventUID(ctx, pid, "uid-1@t")
	if err != nil || o1 == nil {
		t.Fatalf("occ after first sync: %v %v", o1, err)
	}
	if o1.Status != "active" {
		t.Fatalf("first status=%s", o1.Status)
	}

	mu.Lock()
	payload = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:uid-1@t
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260525
SUMMARY:Changed summary
END:VEVENT
END:VCALENDAR
`
	mu.Unlock()

	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	o2, err := st.GetOccupancyBySourceEventUID(ctx, pid, "uid-1@t")
	if err != nil || o2 == nil {
		t.Fatalf("occ after second sync: %v %v", o2, err)
	}
	if o2.Status != "updated" {
		t.Fatalf("second status=%s want updated", o2.Status)
	}
}

func TestSyncProperty_ParseErrorsMarkRunPartial(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:valid-1@t
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260525
SUMMARY:Valid
END:VEVENT
BEGIN:VEVENT
UID:broken-1@t
DTSTAMP:20260401T120000Z
SUMMARY:Missing DTSTART
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(payload, "\n", "\r\n")))
	}))
	defer srv.Close()

	pid := createPropertyWithICS(t, st, srv.URL)
	svc := &Service{Store: st, HTTP: srv.Client()}
	ctx := context.Background()
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListOccupancySyncRuns(ctx, pid, 1)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs err=%v len=%d", err, len(runs))
	}
	if runs[0].Status != "partial" {
		t.Fatalf("run status=%s", runs[0].Status)
	}
	if !runs[0].ErrorMessage.Valid || runs[0].ErrorMessage.String == "" {
		t.Fatalf("expected parse error message, got %#v", runs[0].ErrorMessage)
	}
	if runs[0].EventsSeen != 2 {
		t.Fatalf("eventsSeen=%d", runs[0].EventsSeen)
	}
	if runs[0].OccupanciesUpserted != 1 {
		t.Fatalf("upserted=%d", runs[0].OccupanciesUpserted)
	}

	// Ensure the valid event is still stored despite the broken one.
	occ, err := st.GetOccupancyBySourceEventUID(ctx, pid, "valid-1@t")
	if err != nil || occ == nil {
		t.Fatalf("valid occupancy missing err=%v", err)
	}
}

func TestSyncProperty_UnchangedUpdatedReturnsToActive(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:uid-2@t
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260525
SUMMARY:A
END:VEVENT
END:VCALENDAR
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(payload, "\n", "\r\n")))
	}))
	defer srv.Close()

	pid := createPropertyWithICS(t, st, srv.URL)
	svc := &Service{Store: st, HTTP: srv.Client()}
	ctx := context.Background()
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}

	// mutate feed once => updated
	payload = strings.Replace(payload, "SUMMARY:A", "SUMMARY:B", 1)
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	o, err := st.GetOccupancyBySourceEventUID(ctx, pid, "uid-2@t")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "updated" {
		t.Fatalf("want updated, got %s", o.Status)
	}

	// unchanged on next sync => active
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	o, err = st.GetOccupancyBySourceEventUID(ctx, pid, "uid-2@t")
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != "active" {
		t.Fatalf("want active, got %s", o.Status)
	}

	// Ensure timestamps are sane for future stats usage
	if o.EndAt.Before(time.Now().AddDate(-5, 0, 0)) {
		t.Fatalf("unexpected end date %s", o.EndAt)
	}
}
