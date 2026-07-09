package cleaningcalendar

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

type fakeCalendarClient struct {
	configured bool
	upserts    []CalendarEventPayload
	deletes    []string
}

func (f *fakeCalendarClient) Configured() bool { return f.configured }

func (f *fakeCalendarClient) UpsertEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error) {
	f.upserts = append(f.upserts, event)
	if googleEventID != "" {
		return googleEventID, nil
	}
	return "google-event-id", nil
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
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
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
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "external", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "no-show", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "manual-exclusion", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "checkout-before-excluded", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "excluded-arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), "active")
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
	checkout := occupancy(propertyID, "checkout", time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), "active")
	arrival := occupancy(propertyID, "arrival", time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), "active")
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
