package occupancy

import (
	"context"
	"database/sql"
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

func TestSyncProperty_SplitsExpandedBookingUnavailableBlockWithGeneratedGuestCode(t *testing.T) {
	st := newTestStore(t)
	payload := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:f119449a97461f913129e2bcf11ffaab@booking.com
DTSTAMP:20260706T131906Z
DTSTART;VALUE=DATE:20260706
DTEND;VALUE=DATE:20260707
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
	occ, err := st.GetOccupancyBySourceEventUID(ctx, pid, "f119449a97461f913129e2bcf11ffaab@booking.com")
	if err != nil || occ == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, occ == nil)
	}
	if err := st.UpdateOccupancyGuestDisplayName(ctx, pid, occ.ID, ptrStr("Lenka")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &store.NukiAccessCode{
		PropertyID:       pid,
		OccupancyID:      occ.ID,
		CodeLabel:        "Booking-Lenka",
		ExternalNukiID:   sql.NullString{String: "ext-lenka", Valid: true},
		ValidFrom:        time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		ValidUntil:       time.Date(2026, 7, 7, 7, 0, 0, 0, time.UTC),
		Status:           "generated",
		AccessCodeMasked: sql.NullString{String: "******", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	payload = strings.Replace(payload, "DTEND;VALUE=DATE:20260707", "DTEND;VALUE=DATE:20260709", 1)
	if err := svc.SyncProperty(ctx, pid, "manual"); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	active := make([]store.Occupancy, 0)
	for _, row := range rows {
		if row.Status == "active" || row.Status == "updated" {
			active = append(active, row)
		}
	}
	if len(active) != 3 {
		t.Fatalf("active rows=%d want 3", len(active))
	}
	wantStarts := []string{"2026-07-06T00:00:00Z", "2026-07-07T00:00:00Z", "2026-07-08T00:00:00Z"}
	for i, row := range active {
		if row.SourceEventUID == "f119449a97461f913129e2bcf11ffaab@booking.com" {
			t.Fatalf("row %d kept unsplit uid", i)
		}
		if got := row.StartAt.Format(time.RFC3339); got != wantStarts[i] {
			t.Fatalf("row %d start=%s want %s", i, got, wantStarts[i])
		}
		if got := row.EndAt.Sub(row.StartAt); got != 24*time.Hour {
			t.Fatalf("row %d duration=%s want 24h", i, got)
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
	rows, err := st.ListOccupancies(ctx, pid, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	activeOriginal := 0
	closedManual := 0
	for _, row := range rows {
		if row.SourceEventUID == "august-block@booking.com" && (row.Status == "active" || row.Status == "updated") {
			activeOriginal++
		}
		if row.SourceEventUID == "manual_split:august-block@booking.com:closed:20260810" && row.Status == "active" && row.ClosureState.Valid && row.ClosureState.String == store.ClosureStateClosed {
			closedManual++
		}
	}
	if activeOriginal != 0 {
		t.Fatalf("active original rows=%d want 0", activeOriginal)
	}
	if closedManual != 1 {
		t.Fatalf("closed manual rows=%d want 1", closedManual)
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
	activeOriginal := 0
	activeManual := 0
	for _, row := range rows {
		if row.SourceEventUID == "july-merged@booking.com" && (row.Status == "active" || row.Status == "updated") {
			activeOriginal++
		}
		if strings.HasPrefix(row.SourceEventUID, "manual_split:july-merged@booking.com:night:") && row.Status == "active" {
			activeManual++
		}
	}
	if activeOriginal != 0 {
		t.Fatalf("active original rows=%d want 0", activeOriginal)
	}
	if activeManual != 2 {
		t.Fatalf("active manual rows=%d want 2", activeManual)
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
