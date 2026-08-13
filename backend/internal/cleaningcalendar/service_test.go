package cleaningcalendar

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

type fakeCalendarClient struct {
	configured bool
	upserts    []CalendarEventPayload
	upsertIDs  []string
	deletes    []string
	events     []GoogleCalendarEvent
}

func (f *fakeCalendarClient) Configured() bool { return f.configured }

func (f *fakeCalendarClient) ListEvents(context.Context, string, time.Time, time.Time) ([]GoogleCalendarEvent, error) {
	return f.events, nil
}

func (f *fakeCalendarClient) UpsertEvent(_ context.Context, event CalendarEventPayload, googleEventID string) (string, error) {
	f.upserts = append(f.upserts, event)
	f.upsertIDs = append(f.upsertIDs, googleEventID)
	if googleEventID != "" {
		return googleEventID, nil
	}
	return fmt.Sprintf("google-event-id-%d", len(f.upserts)), nil
}

func (f *fakeCalendarClient) DeleteEvent(_ context.Context, _ string, googleEventID string) error {
	f.deletes = append(f.deletes, googleEventID)
	return nil
}

func TestReconcileUsesNamedStayNightsForSameDayArrival(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	checkout := createCleaningStay(t, st, propertyID, "Checkout Guest", "2026-07-09", "2026-07-10")
	arrival := createCleaningStay(t, st, propertyID, "Arrival Guest", "2026-07-10", "2026-07-12")

	// The night ledger is authoritative for arrival detection.
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET check_in_date = '2026-07-11' WHERE id = ?`, arrival.ID); err != nil {
		t.Fatal(err)
	}

	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client}
	stats, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-12", "test")
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventsUpserted != 2 {
		t.Fatalf("EventsUpserted=%d want 2", stats.EventsUpserted)
	}

	event, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, store.NamedStayCleaningIdentity(propertyID, checkout.ID, "2026-07-10"))
	if err != nil {
		t.Fatal(err)
	}
	if !event.NamedStayID.Valid || event.NamedStayID.Int64 != checkout.ID || event.RawBookingBlockID.Valid {
		t.Fatalf("bad cleaning ownership: %+v", event)
	}
	if !event.SameDayArrival || event.Title != "Upratovanie: Pride Host" {
		t.Fatalf("same-day event=%+v", event)
	}
	if !event.StartsAt.Equal(time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)) || !event.EndsAt.Equal(time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("cleaning window=%s-%s", event.StartsAt, event.EndsAt)
	}
	if len(client.upserts) != 2 || client.upserts[0].NamedStayID != checkout.ID || client.upserts[0].Identity != event.CleaningIdentity.String {
		t.Fatalf("google payloads=%+v", client.upserts)
	}
}

func TestReconcileRawOwnershipIsDeterministicAndCollapsesToNamedStay(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	firstID := insertRawCleaningBlock(t, st, propertyID, "raw-first", "2026-07-09", "2026-07-12")
	insertRawCleaningBlock(t, st, propertyID, "raw-overlap", "2026-07-09", "2026-07-12")

	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-12", "test"); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("raw events=%d want 3", len(events))
	}
	for _, event := range events {
		if !event.RawBookingBlockID.Valid || event.RawBookingBlockID.Int64 != firstID || event.NamedStayID.Valid {
			t.Fatalf("non-deterministic raw owner: %+v", event)
		}
	}

	stay := createCleaningStay(t, st, propertyID, "Named Guest", "2026-07-09", "2026-07-12")
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-12", "test"); err != nil {
		t.Fatal(err)
	}
	events, err = st.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].NamedStayID.Valid || events[0].NamedStayID.Int64 != stay.ID {
		t.Fatalf("events after naming=%+v", events)
	}
}

func TestReconcileRemovalIsDateScoped(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	first := createCleaningStay(t, st, propertyID, "First", "2026-07-09", "2026-07-10")
	second := createCleaningStay(t, st, propertyID, "Second", "2026-07-19", "2026-07-20")
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-20", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET status = 'archived' WHERE id IN (?, ?)`, first.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	firstEvent, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, store.NamedStayCleaningIdentity(propertyID, first.ID, "2026-07-10"))
	if err != nil {
		t.Fatal(err)
	}
	secondEvent, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, store.NamedStayCleaningIdentity(propertyID, second.ID, "2026-07-20"))
	if err != nil {
		t.Fatal(err)
	}
	if firstEvent.Status != store.CleaningCalendarStatusRemoved || secondEvent.Status != store.CleaningCalendarStatusSynced {
		t.Fatalf("date-scoped statuses=%q/%q", firstEvent.Status, secondEvent.Status)
	}
	if len(client.deletes) != 1 {
		t.Fatalf("deletes=%v want one", client.deletes)
	}
}

func TestReconcilePreservesStoredGoogleIDAndEventHistory(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	stay := createCleaningStay(t, st, propertyID, "History Guest", "2026-07-09", "2026-07-10")
	identity := store.NamedStayCleaningIdentity(propertyID, stay.ID, "2026-07-10")
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client}

	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	original, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET status = 'archived' WHERE id = ?`, stay.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET status = 'active' WHERE id = ?`, stay.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	restored, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != original.ID || restored.GoogleEventID.String != original.GoogleEventID.String || restored.Status != store.CleaningCalendarStatusSynced {
		t.Fatalf("history was replaced: before=%+v after=%+v", original, restored)
	}
	if got := client.upsertIDs[len(client.upsertIDs)-1]; got != original.GoogleEventID.String {
		t.Fatalf("stored Google ID=%q want %q", got, original.GoogleEventID.String)
	}
	var logCount int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM cleaning_calendar_event_logs WHERE cleaning_calendar_event_id = ?`, original.ID).Scan(&logCount); err != nil {
		t.Fatal(err)
	}
	if logCount != 3 {
		t.Fatalf("event logs=%d want 3", logCount)
	}
}

func TestReconcileSkipsUnchangedListedGoogleEvent(t *testing.T) {
	ctx := context.Background()
	st, propertyID := setupCleaningCalendarProperty(t, ctx)
	stay := createCleaningStay(t, st, propertyID, "No-op Guest", "2026-07-09", "2026-07-10")
	client := &fakeCalendarClient{configured: true}
	svc := &Service{Store: st, Client: client}
	if _, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test"); err != nil {
		t.Fatal(err)
	}
	event, err := st.GetCleaningCalendarEventByCleaningIdentity(ctx, store.NamedStayCleaningIdentity(propertyID, stay.ID, "2026-07-10"))
	if err != nil {
		t.Fatal(err)
	}
	client.upserts = nil
	client.events = []GoogleCalendarEvent{{
		ID: event.GoogleEventID.String, Summary: event.Title, Status: "confirmed", Start: event.StartsAt, End: event.EndsAt,
		PrivateProperties: map[string]string{
			"pms_property_id":       fmt.Sprintf("%d", propertyID),
			"pms_cleaning_event_id": fmt.Sprintf("%d", event.ID),
			"pms_cleaning_identity": event.CleaningIdentity.String,
		},
	}}
	stats, err := svc.ReconcilePropertyDateRange(ctx, propertyID, "2026-07-10", "2026-07-10", "test")
	if err != nil {
		t.Fatal(err)
	}
	if stats.EventsUpserted != 0 || len(client.upserts) != 0 {
		t.Fatalf("unchanged event patched: stats=%+v upserts=%d", stats, len(client.upserts))
	}
}

func TestGoogleMatchingHasNoLegacyOrSummaryFallback(t *testing.T) {
	loc := time.UTC
	event := &store.CleaningCalendarEvent{
		PropertyID:       1,
		NamedStayID:      sql.NullInt64{Int64: 7, Valid: true},
		CleaningIdentity: sql.NullString{String: "stay:1:7:2026-07-10", Valid: true},
		CleaningDate:     "2026-07-10",
		Title:            "Upratovanie: Bez Hosta",
	}
	idx := newGoogleEventIndex([]GoogleCalendarEvent{{
		ID: "legacy", Summary: event.Title, Start: time.Date(2026, 7, 10, 9, 0, 0, 0, loc),
		PrivateProperties: map[string]string{"pms_property_id": "1", "pms_occupancy_id": "99"},
	}}, 1, loc)
	if got := idx.match(event, nil); got != "" {
		t.Fatalf("legacy event matched as %q", got)
	}
}

func TestGoogleWriteOmitsLegacyOccupancyProperty(t *testing.T) {
	var requestBody map[string]interface{}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"id":"google-id"}`)), Header: make(http.Header)}, nil
	})}
	client := &ServiceAccountClient{HTTP: httpClient, accessToken: "token", expiresAt: time.Now().Add(time.Hour)}
	if _, err := client.writeEvent(context.Background(), http.MethodPost, "https://calendar.test/events", CalendarEventPayload{
		PropertyID: 1, NamedStayID: 7, Identity: "stay:1:7:2026-07-10", LocalEventID: 8,
		Start: time.Now(), End: time.Now().Add(time.Hour), TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	extended := requestBody["extendedProperties"].(map[string]interface{})
	private := extended["private"].(map[string]interface{})
	if _, ok := private["pms_occupancy_id"]; ok {
		t.Fatalf("legacy property written: %+v", private)
	}
	if private["pms_named_stay_id"] != "7" || private["pms_cleaning_identity"] != "stay:1:7:2026-07-10" {
		t.Fatalf("new ownership missing: %+v", private)
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

func createCleaningStay(t *testing.T, st *store.Store, propertyID int64, name, checkIn, checkOut string) *store.NamedStay {
	t.Helper()
	stay, err := st.CreateNamedStayRecord(context.Background(), store.NamedStayCreateInput{
		PropertyID: propertyID, DisplayName: name, StayType: store.StayTypeBookingCom,
		CheckInDate: checkIn, CheckOutDate: checkOut,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stay
}

func insertRawCleaningBlock(t *testing.T, st *store.Store, propertyID int64, uid, checkIn, checkOut string) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO raw_booking_blocks (
			property_id, source_type, source_event_uid, check_in_date, check_out_date, status,
			content_hash, imported_at, last_synced_at, created_at, updated_at
		) VALUES (?, 'booking_ics', ?, ?, ?, 'active', ?, ?, ?, ?, ?)`, propertyID, uid, checkIn, checkOut, uid+"-hash", now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	blockID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	start, _ := time.Parse("2006-01-02", checkIn)
	end, _ := time.Parse("2006-01-02", checkOut)
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		if _, err := st.DB.ExecContext(ctx, `
			INSERT INTO raw_booking_block_nights (property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?)`, propertyID, blockID, day.Format("2006-01-02"), now, now); err != nil {
			t.Fatal(err)
		}
	}
	return blockID
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
