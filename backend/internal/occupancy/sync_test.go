package occupancy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

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

func TestSyncProperty_PropertyLeaseSkipsConcurrentSync(t *testing.T) {
	st := newTestStore(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var mu sync.Mutex
	requests := 0
	payload := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:lease@t
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260522
DTEND;VALUE=DATE:20260523
SUMMARY:Initial
END:VEVENT
END:VCALENDAR
`, "\n", "\r\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	pid := createPropertyWithICS(t, st, srv.URL)
	svc := &Service{Store: st, HTTP: srv.Client()}
	ctx := context.Background()
	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.SyncProperty(ctx, pid, "manual") }()
	<-started
	if err := svc.SyncProperty(ctx, pid, "manual"); err == nil || !strings.Contains(err.Error(), "sync_already_running") {
		close(release)
		t.Fatalf("second sync err=%v want sync_already_running", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first sync: %v", err)
	}
	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("http requests=%d want 1 (second sync skipped before fetch)", gotRequests)
	}
	runs, err := st.ListOccupancySyncRuns(ctx, pid, 10)
	if err != nil {
		t.Fatal(err)
	}
	foundSkipped := false
	for _, r := range runs {
		if r.Status == "skipped" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("no skipped sync run recorded: %+v", runs)
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
	// PMS_19 §7.2 / §13.5: a partial parse must apply zero mutations and use
	// the explicit partial_no_mutation status.
	if runs[0].Status != StatusPartialNoMutation {
		t.Fatalf("run status=%s want %s", runs[0].Status, StatusPartialNoMutation)
	}
	if !runs[0].ErrorMessage.Valid || runs[0].ErrorMessage.String == "" {
		t.Fatalf("expected parse error message, got %#v", runs[0].ErrorMessage)
	}
	if runs[0].EventsSeen != 2 {
		t.Fatalf("eventsSeen=%d", runs[0].EventsSeen)
	}

	// The valid event must NOT be stored because the run applied no mutation.
	occ, err := st.GetOccupancyBySourceEventUID(ctx, pid, "valid-1@t")
	if err != nil {
		t.Fatal(err)
	}
	if occ != nil {
		t.Fatalf("expected no occupancy stored on partial_no_mutation, got %#v", occ)
	}
}

func TestSyncProperty_KeepsStableMultiNightBookingUnavailableBlocks(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:f119449a97461f913129e2bcf11ffaab@booking.com
DTSTAMP:20260706T131906Z
DTSTART;VALUE=DATE:20260706
DTEND;VALUE=DATE:20260709
SUMMARY:CLOSED - Not available
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
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].SourceEventUID != "f119449a97461f913129e2bcf11ffaab@booking.com" {
		t.Fatalf("uid=%q", rows[0].SourceEventUID)
	}
	if got := rows[0].StartAt.Format(time.RFC3339); got != "2026-07-06T00:00:00Z" {
		t.Fatalf("start=%s", got)
	}
	if got := rows[0].EndAt.Format(time.RFC3339); got != "2026-07-09T00:00:00Z" {
		t.Fatalf("end=%s", got)
	}
}

func TestSyncProperty_DoesNotAutoSplitMultiNightBlock(t *testing.T) {
	// PMS_19 §5.2 / §13.1: new sync code must never auto-split a block into
	// hidden generated guest stays. The block stays as one aggregate row that
	// covers each night exactly once.
	st := newTestStore(t)
	start := time.Now().UTC().AddDate(0, 2, 0)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)
	payload := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:f119449a97461f913129e2bcf11ffaab@booking.com
DTSTAMP:20260706T131906Z
DTSTART;VALUE=DATE:%s
DTEND;VALUE=DATE:%s
SUMMARY:CLOSED - Not available
END:VEVENT
END:VCALENDAR
`, start.Format("20060102"), end.Format("20060102"))
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
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, row := range rows {
		if row.Status == "active" || row.Status == "updated" {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active rows=%d want 1 (no auto-split)", active)
	}
	// Each night has exactly one active representation.
	for i := 0; i < 3; i++ {
		night := start.AddDate(0, 0, i).Format("2006-01-02")
		n, err := st.ActiveOccupancyNightCount(ctx, pid, night)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", night, n)
		}
	}
}

func TestSyncProperty_PreservesManualNightSplit(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:august-block@booking.com
DTSTAMP:20260706T131906Z
DTSTART;VALUE=DATE:20260807
DTEND;VALUE=DATE:20260811
SUMMARY:CLOSED - Not available
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
	occ, err := st.GetOccupancyBySourceEventUID(ctx, pid, "august-block@booking.com")
	if err != nil || occ == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, occ == nil)
	}
	if err := st.CloseOccupancyNight(ctx, pid, occ.ID, 1, "2026-08-10", "closed", "maintenance"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	// The closed night must survive re-sync as exactly one active closed row,
	// and every night still has exactly one active representation.
	closedManual := 0
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.SourceEventUID == "manual_split:august-block@booking.com:closed:20260810" &&
			row.Status == "active" && row.ClosureState.Valid && row.ClosureState.String == store.ClosureStateClosed {
			closedManual++
		}
	}
	if closedManual != 1 {
		t.Fatalf("closed manual rows=%d want 1", closedManual)
	}
	for _, d := range []string{"2026-08-07", "2026-08-08", "2026-08-09", "2026-08-10"} {
		n, err := st.ActiveOccupancyNightCount(ctx, pid, d)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", d, n)
		}
	}
}

func TestSyncProperty_PreservesManualNightlySplit(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:july-merged@booking.com
DTSTAMP:20260706T131906Z
DTSTART;VALUE=DATE:20260730
DTEND;VALUE=DATE:20260801
SUMMARY:CLOSED - Not available
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
	occ, err := st.GetOccupancyBySourceEventUID(ctx, pid, "july-merged@booking.com")
	if err != nil || occ == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, occ == nil)
	}
	if err := st.SplitOccupancyIntoNights(ctx, pid, occ.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	activeManual := 0
	for _, row := range rows {
		if strings.HasPrefix(row.SourceEventUID, "manual_split:july-merged@booking.com:night:") && row.Status == "active" {
			activeManual++
		}
	}
	if activeManual != 2 {
		t.Fatalf("active manual rows=%d want 2", activeManual)
	}
	for _, d := range []string{"2026-07-30", "2026-07-31"} {
		n, err := st.ActiveOccupancyNightCount(ctx, pid, d)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", d, n)
		}
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
