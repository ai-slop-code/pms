package cleaningcalendar

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

type fakeCalendarClient struct {
	configured bool
	upserts    []CalendarEventPayload
	deletes    []string
	events     []GoogleCalendarEvent
}

func (f *fakeCalendarClient) Configured() bool { return f.configured }

func (f *fakeCalendarClient) ListEvents(ctx context.Context, calendarID string, timeMin, timeMax time.Time) ([]GoogleCalendarEvent, error) {
	return f.events, nil
}

func (f *fakeCalendarClient) UpsertEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error) {
	f.upserts = append(f.upserts, event)
	if googleEventID != "" {
		return googleEventID, nil
	}
	return fmt.Sprintf("google-event-id-%d", len(f.upserts)), nil
}

func (f *fakeCalendarClient) DeleteEvent(ctx context.Context, calendarID, googleEventID string) error {
	f.deletes = append(f.deletes, googleEventID)
	return nil
}

func TestReconcilePropertyCreatesSameDayCleaningEvent(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, arrival, runID); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	stats, err := svc.ReconcileProperty(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventsUpserted != 2 {
		t.Fatalf("EventsUpserted=%d want 2", stats.EventsUpserted)
	}
	events, err := st.ListCleaningCalendarEventsForMonth(ctx, propertyID, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	var checkoutEvent *store.CleaningCalendarEvent
	for i := range events {
		if events[i].CleaningDate == "2026-07-10" {
			checkoutEvent = &events[i]
		}
	}
	if checkoutEvent == nil {
		t.Fatal("checkout cleaning event missing")
	}
	if checkoutEvent.Title != "Upratovanie: Pride Host" {
		t.Fatalf("title=%q", checkoutEvent.Title)
	}
	if checkoutEvent.StartsAt.UTC() != time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC) {
		t.Fatalf("starts_at=%s", checkoutEvent.StartsAt)
	}
	if checkoutEvent.EndsAt.UTC() != time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC) {
		t.Fatalf("ends_at=%s", checkoutEvent.EndsAt)
	}
	if len(client.upserts) != 2 {
		t.Fatalf("upserts=%d want 2", len(client.upserts))
	}
}

func TestReconcilePropertyLateSameDayArrivalUpdatesOnlyTitle(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Title != "Upratovanie: Bez Hosta" {
		t.Fatalf("initial title=%q", before.Title)
	}
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, arrival, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, before.OccupancyID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Title != "Upratovanie: Pride Host" {
		t.Fatalf("updated title=%q", after.Title)
	}
	if !after.StartsAt.Equal(before.StartsAt) || !after.EndsAt.Equal(before.EndsAt) {
		t.Fatalf("time window changed: before %s-%s after %s-%s", before.StartsAt, before.EndsAt, after.StartsAt, after.EndsAt)
	}
}

func TestReconcilePropertyIncludesExternalSale(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "external", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOccupancyExternalSale(ctx, propertyID, 1, 1, 10000, "EUR", "direct", "direct guest"); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if len(client.upserts) != 1 {
		t.Fatalf("upserts=%d want 1", len(client.upserts))
	}
}

func TestReconcilePropertyRemovesStayOutcomeCleaningEvent(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "no-show", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, "no-show")
	if err != nil || row == nil {
		t.Fatalf("get occupancy: %v", err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOccupancyStayOutcome(ctx, propertyID, row.ID, 1, store.StayOutcomeNoShow, "guest did not arrive"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != store.CleaningCalendarStatusRemoved {
		t.Fatalf("status=%q want removed", after.Status)
	}
	if after.ErrorMessage.String != "stay outcome: no_show" {
		t.Fatalf("error_message=%q", after.ErrorMessage.String)
	}
	if len(client.deletes) != 1 {
		t.Fatalf("deletes=%d want 1", len(client.deletes))
	}
}

func TestReconcilePropertyRemovesAndRecreatesManualCleaningExclusion(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "manual-exclusion", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, "manual-exclusion")
	if err != nil || row == nil {
		t.Fatalf("get occupancy: %v", err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOccupancyCleaningCalendarExcluded(ctx, propertyID, row.ID, 1, "owner will clean"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != store.CleaningCalendarStatusRemoved {
		t.Fatalf("status=%q want removed", after.Status)
	}
	if after.ErrorMessage.String != "manual cleaning calendar exclusion" {
		t.Fatalf("error_message=%q", after.ErrorMessage.String)
	}
	if len(client.deletes) != 1 {
		t.Fatalf("deletes=%d want 1", len(client.deletes))
	}
	if err := st.ClearOccupancyCleaningCalendarExcluded(ctx, propertyID, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	recreated, err := st.GetCleaningCalendarEventByOccupancy(ctx, propertyID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recreated.Status != store.CleaningCalendarStatusSynced {
		t.Fatalf("status=%q want synced", recreated.Status)
	}
}

func TestReconcilePropertyManualExcludedArrivalStillCountsAsSameDay(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "checkout-before-excluded", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "excluded-arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, arrival, runID); err != nil {
		t.Fatal(err)
	}
	arrivalRow, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, "excluded-arrival")
	if err != nil || arrivalRow == nil {
		t.Fatalf("get arrival: %v", err)
	}
	if err := st.MarkOccupancyCleaningCalendarExcluded(ctx, propertyID, arrivalRow.ID, 1, "owner will clean"); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListCleaningCalendarEventsForMonth(ctx, propertyID, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	if events[0].OccupancyID == arrivalRow.ID {
		t.Fatal("created cleaning event for manually excluded arrival")
	}
	if events[0].Title != "Upratovanie: Pride Host" {
		t.Fatalf("title=%q want same-day title", events[0].Title)
	}
}

func TestReconcilePropertySameDayArrivalIgnoresStayOutcome(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, arrival, runID); err != nil {
		t.Fatal(err)
	}
	arrivalRow, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, "arrival")
	if err != nil || arrivalRow == nil {
		t.Fatalf("get arrival: %v", err)
	}
	if err := st.MarkOccupancyStayOutcome(ctx, propertyID, arrivalRow.ID, 1, store.StayOutcomeCancelledNonRefundable, "cancelled"); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListCleaningCalendarEventsForMonth(ctx, propertyID, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.OccupancyID == arrivalRow.ID && ev.Status != store.CleaningCalendarStatusRemoved {
			t.Fatalf("outcome arrival created active event: %+v", ev)
		}
		if ev.OccupancyID != arrivalRow.ID && ev.Title != "Upratovanie: Bez Hosta" {
			t.Fatalf("checkout title=%q want no-guest", ev.Title)
		}
	}
}

func setupCleaningCalendarProperty(t *testing.T, ctx context.Context) (*store.Store, int64) {
	t.Helper()
	st := &store.Store{DB: testutil.OpenTestDB(t)}
	user, err := st.CreateUser(ctx, "calendar-owner@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, user.ID, "Calendar property", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, property.ID, map[string]interface{}{"default_check_out_time": "09:00", "default_check_in_time": "14:00"}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	calendarID := "cleaning@example.com"
	if _, err := st.UpdateGoogleCleaningSettings(ctx, property.ID, store.CleaningCalendarSettingsPatch{Enabled: &enabled, CalendarID: &calendarID}); err != nil {
		t.Fatal(err)
	}
	return st, property.ID
}

// PMS_19 §13.11: an unnamed Booking block creates one provisional cleaning
// checkout per blocked night; naming the whole range collapses them to a single
// checkout; a disappeared UID removes all future provisional events.
func TestReconcileProvisionalPerNightAndCollapseOnNaming(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	uid := "prov-block@booking.com"
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	block := store.DesiredBlock{UID: uid, Start: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), Summary: "CLOSED - Not available", ContentHash: "h1"}
	if err := st.ReconcileBookingICSSync(ctx, propertyID, "booking_ics", []store.DesiredBlock{block}, now, &store.SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return now }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	events := activeCleaningDates(t, st, propertyID)
	if len(events) != 3 || !events["2026-07-10"] || !events["2026-07-11"] || !events["2026-07-12"] {
		t.Fatalf("provisional checkouts=%v want 10,11,12", events)
	}

	// Name the whole 3-night range as one stay → collapse to a single checkout.
	if _, err := st.CreateNamedStay(ctx, propertyID, uid, "2026-07-09", "2026-07-12", "Whole Stay", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	events = activeCleaningDates(t, st, propertyID)
	if len(events) != 1 || !events["2026-07-12"] {
		t.Fatalf("after naming, checkouts=%v want only 07-12", events)
	}

	// UID disappears → all future cleaning events removed.
	if err := st.ReconcileBookingICSSync(ctx, propertyID, "booking_ics", []store.DesiredBlock{}, now, &store.SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	events = activeCleaningDates(t, st, propertyID)
	if len(events) != 0 {
		t.Fatalf("after disappearance, checkouts=%v want none", events)
	}
}

// PMS_19 §13.12: marking an unnamed blocked night closed / no guest removes the
// provisional cleaning event and drops the night out of guest occupancy while
// still counting it as availability-blocked... until it is closed.
func TestReconcile_NoGuestClosureRemovesCleaningAndGuestOccupancy(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	uid := "closure-block@booking.com"
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	blk := store.DesiredBlock{UID: uid, Start: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), Summary: "CLOSED - Not available", ContentHash: "h"}
	if err := st.ReconcileBookingICSSync(ctx, propertyID, "booking_ics", []store.DesiredBlock{blk}, now, &store.SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return now }}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if got := activeCleaningDates(t, st, propertyID); len(got) != 1 || !got["2026-07-16"] {
		t.Fatalf("provisional cleaning=%v want 07-16", got)
	}
	avail, guest, err := st.OccupancyMetricNights(ctx, propertyID, "2026-07-15", "2026-07-16")
	if err != nil {
		t.Fatal(err)
	}
	if avail != 1 || guest != 0 {
		t.Fatalf("before closure avail=%d guest=%d want 1/0", avail, guest)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, uid)
	if err != nil || row == nil {
		t.Fatalf("get occupancy: %v", err)
	}
	if err := st.CloseOccupancy(ctx, propertyID, row.ID, 1, "owner use", "owner_stay"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if got := activeCleaningDates(t, st, propertyID); len(got) != 0 {
		t.Fatalf("after closure cleaning=%v want none", got)
	}
	avail, guest, err = st.OccupancyMetricNights(ctx, propertyID, "2026-07-15", "2026-07-16")
	if err != nil {
		t.Fatal(err)
	}
	if avail != 0 || guest != 0 {
		t.Fatalf("after closure avail=%d guest=%d want 0/0 (closed excluded)", avail, guest)
	}

	// PMS_19 §13.12 (second paragraph): reopening restores unnamed-block
	// coverage and recreates the provisional cleaning event.
	if err := st.ReopenOccupancy(ctx, propertyID, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
		t.Fatal(err)
	}
	if got := activeCleaningDates(t, st, propertyID); len(got) != 1 || !got["2026-07-16"] {
		t.Fatalf("after reopen cleaning=%v want 07-16 restored", got)
	}
	avail, guest, err = st.OccupancyMetricNights(ctx, propertyID, "2026-07-15", "2026-07-16")
	if err != nil {
		t.Fatal(err)
	}
	if avail != 1 || guest != 0 {
		t.Fatalf("after reopen avail=%d guest=%d want 1/0", avail, guest)
	}
}

// PMS_19 §13.14: syncing the same feed twice must not create duplicate cleaning
// events; each identity key maps to exactly one event.
func TestReconcile_CleaningIdempotency(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	uid := "idem-block@booking.com"
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	blk := store.DesiredBlock{UID: uid, Start: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), Summary: "CLOSED - Not available", ContentHash: "h"}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		if err := st.ReconcileBookingICSSync(ctx, propertyID, "booking_ics", []store.DesiredBlock{blk}, now, &store.SyncCounters{}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ReconcileProperty(ctx, propertyID, "test"); err != nil {
			t.Fatal(err)
		}
	}
	all, err := st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("cleaning events=%d want 3 (no duplicates)", len(all))
	}
}

func TestReconcileDateRangeSkipsUnchangedGoogleEvent(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	runID, err := st.StartOccupancySyncRun(ctx, propertyID, "test")
	if err != nil {
		t.Fatal(err)
	}
	checkout := occupancy(propertyID, "hash-noop", time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	if err := st.UpsertOccupancy(ctx, checkout, runID); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].DesiredHash.Valid || !events[0].GoogleEventID.Valid {
		t.Fatalf("event after first reconcile = %+v", events)
	}
	client.upserts = nil
	client.events = []GoogleCalendarEvent{{
		ID:      events[0].GoogleEventID.String,
		Summary: events[0].Title,
		Status:  "confirmed",
		Start:   events[0].StartsAt,
		End:     events[0].EndsAt,
		PrivateProperties: map[string]string{
			"pms_property_id":       fmt.Sprintf("%d", propertyID),
			"pms_cleaning_event_id": fmt.Sprintf("%d", events[0].ID),
		},
	}}
	stats, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test")
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventsUpserted != 0 || len(client.upserts) != 0 {
		t.Fatalf("second reconcile patched unchanged event: stats=%+v upserts=%d", stats, len(client.upserts))
	}
}

func TestReconcileDateRangeUsesPMS21CleaningOwnership(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	uid := "stage6-raw@booking.com"
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	block := store.DesiredBlock{UID: uid, Start: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), Summary: "CLOSED", ContentHash: "h"}
	if err := st.ReconcileBookingICSSync(ctx, propertyID, store.UpstreamSourceBookingICS, []store.DesiredBlock{block}, now, &store.SyncCounters{RawBlocksDualWrite: true}); err != nil {
		t.Fatal(err)
	}
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client, Now: func() time.Time { return now }}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-12", "test"); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("raw provisional events=%d want 3", len(events))
	}
	for _, ev := range events {
		if ev.Title != "Upratovanie" || ev.CleaningKind != store.CleaningKindProvisionalBlock || !ev.RawBookingBlockID.Valid || !ev.CleaningIdentity.Valid {
			t.Fatalf("raw event missing PMS21 ownership: %+v", ev)
		}
	}
	var blockID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ?`, propertyID, uid).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, propertyID, blockID, store.NamedStayCreateInput{DisplayName: "Named", StayType: store.StayTypeBookingCom, CheckInDate: "2026-07-09", CheckOutDate: "2026-07-12", CreatedByUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-12", "test"); err != nil {
		t.Fatal(err)
	}
	events, err = st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("after promotion active events=%d want 1: %+v", len(events), events)
	}
	if events[0].CleaningKind != store.CleaningKindNamedStay || !events[0].NamedStayID.Valid || events[0].NamedStayID.Int64 != stay.ID || !events[0].CleaningIdentity.Valid {
		t.Fatalf("named stay event missing PMS21 ownership: %+v", events[0])
	}
}

func activeCleaningDates(t *testing.T, st *store.Store, propertyID int64) map[string]bool {
	t.Helper()
	all, err := st.ListActiveCleaningCalendarEvents(context.Background(), propertyID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range all {
		out[e.CleaningDate] = true
	}
	return out
}

func occupancy(propertyID int64, uid string, start, end time.Time, status string) *store.Occupancy {
	return &store.Occupancy{
		PropertyID:     propertyID,
		SourceType:     "booking_ics",
		SourceEventUID: uid,
		StartAt:        start,
		EndAt:          end,
		Status:         status,
		RawSummary:     sql.NullString{String: uid, Valid: true},
		ContentHash:    uid + "-hash",
	}
}
