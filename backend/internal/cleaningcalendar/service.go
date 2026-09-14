package cleaningcalendar

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"pms/backend/internal/store"
)

const (
	reconcileFutureWindowDays = 365
	minimalConflictDuration   = 30 * time.Minute
)

type CalendarClient interface {
	Configured() bool
	ListEvents(ctx context.Context, calendarID string, timeMin, timeMax time.Time) ([]GoogleCalendarEvent, error)
	UpsertEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error)
	DeleteEvent(ctx context.Context, calendarID, googleEventID string) error
}

type GoogleCalendarEvent struct {
	ID                string
	Summary           string
	Description       string
	Status            string
	Start             time.Time
	End               time.Time
	PrivateProperties map[string]string
}

type CalendarEventPayload struct {
	CalendarID   string
	Summary      string
	Description  string
	Start        time.Time
	End          time.Time
	TimeZone     string
	PropertyID   int64
	NamedStayID  int64
	Identity     string
	LocalEventID int64
}

type Service struct {
	Store  *store.Store
	Client CalendarClient
	Now    func() time.Time
	mu     sync.Mutex
}

type ReconcileStats struct {
	EventsSeen     int `json:"events_seen"`
	EventsUpserted int `json:"events_upserted"`
	EventsRemoved  int `json:"events_removed"`
}

func (s *Service) ReconcileProperty(ctx context.Context, propertyID int64, trigger string) (*ReconcileStats, error) {
	if s.Store == nil {
		return nil, errors.New("store not configured")
	}
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	localNow := now.In(loc)
	windowEnd := localNow.AddDate(0, 0, reconcileFutureWindowDays)
	fromDate := localNow.Format("2006-01-02")
	toDate := windowEnd.In(loc).Format("2006-01-02")
	return s.ReconcilePropertyDateRange(ctx, propertyID, fromDate, toDate, trigger)
}

func (s *Service) ReconcilePropertyDateRange(ctx context.Context, propertyID int64, fromDate, toDate, trigger string) (*ReconcileStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Store == nil {
		return nil, errors.New("store not configured")
	}
	owner := fmt.Sprintf("cleaning-service-%p", s)
	acquired, err := s.Store.TryAcquireJobLease(ctx, fmt.Sprintf("cleaning_calendar_property_%d", propertyID), owner, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("cleaning calendar reconciliation already in progress")
	}
	defer s.Store.ReleaseJobLease(context.Background(), fmt.Sprintf("cleaning_calendar_property_%d", propertyID), owner)
	if _, err := time.Parse("2006-01-02", fromDate); err != nil {
		return nil, err
	}
	toParsed, err := time.Parse("2006-01-02", toDate)
	if err != nil {
		return nil, err
	}
	if toDate < fromDate {
		return nil, errors.New("invalid cleaning reconcile date range")
	}
	settings, err := s.Store.GetGoogleCleaningSettings(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !settings.CalendarID.Valid || strings.TrimSpace(settings.CalendarID.String) == "" {
		return &ReconcileStats{}, nil
	}
	runID, err := s.Store.StartCleaningCalendarSyncRun(ctx, propertyID, trigger)
	if err != nil {
		return nil, err
	}
	stats := &ReconcileStats{}
	status := "success"
	var runErr *string
	finish := func(err error) (*ReconcileStats, error) {
		if err != nil {
			status = "failure"
			msg := err.Error()
			runErr = &msg
		} else if runErr != nil {
			status = "partial"
			err = errors.New(*runErr)
		}
		_ = s.Store.FinishCleaningCalendarSyncRun(ctx, runID, status, runErr, stats.EventsSeen, stats.EventsUpserted, stats.EventsRemoved)
		return stats, err
	}
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return finish(err)
	}
	profile, err := s.Store.GetPropertyProfile(ctx, propertyID)
	if err != nil {
		return finish(err)
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	dayStart, _ := time.ParseInLocation("2006-01-02", fromDate, loc)
	dayAfterEnd := time.Date(toParsed.Year(), toParsed.Month(), toParsed.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	today := time.Now().UTC()
	if s.Now != nil {
		today = s.Now().UTC()
	}
	horizonFrom := today.In(loc).Format("2006-01-02")
	horizonTo := today.In(loc).AddDate(0, 0, reconcileFutureWindowDays).Format("2006-01-02")
	desiredFrom, desiredTo := fromDate, toDate
	if desiredFrom < horizonFrom {
		desiredFrom = horizonFrom
	}
	if desiredTo > horizonTo {
		desiredTo = horizonTo
	}
	googleIndex := googleEventIndex{}
	if s.Client != nil && s.Client.Configured() {
		googleEvents, err := s.Client.ListEvents(ctx, settings.CalendarID.String, dayStart, dayAfterEnd)
		if err != nil {
			return finish(err)
		}
		googleIndex = newGoogleEventIndex(googleEvents, propertyID, loc)
	}
	desiredEvents := []*store.CleaningCalendarEvent{}
	if desiredFrom <= desiredTo {
		desiredEvents, err = s.buildDesiredEvents(ctx, propertyID, settings, profile, prop.Timezone, loc, desiredFrom, desiredTo)
	}
	if err != nil {
		return finish(err)
	}
	desired := make(map[string]struct{})
	reusedGoogleIDs := make(map[string]struct{})
	for i := range desiredEvents {
		event := desiredEvents[i]
		stats.EventsSeen++
		key := cleaningEventKey(event)
		desired[key] = struct{}{}
		existing := s.lookupExistingDesiredEvent(ctx, event)
		if existing != nil {
			if !existing.ScheduleHash.Valid || existing.ScheduleHash.String == event.ScheduleHash.String {
				event.EndsAt = existing.EndsAt
				event.WarningMessage = existing.WarningMessage
			}
			event.DesiredHash = sql.NullString{String: desiredHash(event, prop.Timezone), Valid: true}
			if matchedID := googleIndex.match(event, existing); matchedID != "" && !event.GoogleEventID.Valid {
				event.GoogleEventID = sql.NullString{String: matchedID, Valid: true}
			}
			if s.canSkipGoogleUpsert(existing, event, googleIndex.listed, googleIndex.seen(existing)) {
				continue
			}
		} else if matchedID := googleIndex.match(event, nil); matchedID != "" {
			event.GoogleEventID = sql.NullString{String: matchedID, Valid: true}
		}
		if event.GoogleEventID.Valid && strings.TrimSpace(event.GoogleEventID.String) != "" {
			reusedGoogleIDs[strings.TrimSpace(event.GoogleEventID.String)] = struct{}{}
		}
		saved, err := s.Store.UpsertCleaningCalendarEventByIdentity(ctx, event)
		if err != nil {
			return finish(err)
		}
		stats.EventsUpserted++
		if err := s.syncUpsert(ctx, saved, prop.Timezone, runID); err != nil {
			msg := err.Error()
			runErr = &msg
		}
	}
	active, err := s.Store.ListActiveCleaningCalendarEvents(ctx, propertyID)
	if err != nil {
		return finish(err)
	}
	for _, ev := range active {
		if _, ok := desired[cleaningEventKey(&ev)]; ok {
			continue
		}
		if ev.CleaningDate < horizonFrom && ev.PendingAction != store.CleaningPendingActionDelete {
			if ev.PendingAction == store.CleaningPendingActionUpsert {
				_ = s.Store.SetCleaningCalendarEventPendingAction(ctx, ev.PropertyID, ev.ID, store.CleaningPendingActionNone)
			}
			continue
		}
		if ev.GoogleEventID.Valid {
			if _, reused := reusedGoogleIDs[strings.TrimSpace(ev.GoogleEventID.String)]; reused {
				if err := s.Store.MarkCleaningCalendarEventRemoved(ctx, ev.PropertyID, ev.ID, nil); err != nil {
					return finish(err)
				}
				action := "delete"
				insertLog(s.Store, ctx, ev.PropertyID, ev.ID, runID, action, "removed; Google event reused")
				stats.EventsRemoved++
				continue
			}
		}
		if matchedID := googleIndex.match(&ev, &ev); matchedID != "" && !ev.GoogleEventID.Valid {
			ev.GoogleEventID = sql.NullString{String: matchedID, Valid: true}
			_ = s.Store.MarkCleaningCalendarEventGoogleSeen(ctx, ev.PropertyID, ev.ID, matchedID)
		}
		if err := s.syncDelete(ctx, &ev, runID); err != nil {
			msg := err.Error()
			runErr = &msg
			continue
		}
		stats.EventsRemoved++
	}
	return finish(nil)
}

func cleaningEventKey(event *store.CleaningCalendarEvent) string {
	if event == nil {
		return ""
	}
	if event.CleaningIdentity.Valid && strings.TrimSpace(event.CleaningIdentity.String) != "" {
		return "identity:" + strings.TrimSpace(event.CleaningIdentity.String)
	}
	return fmt.Sprintf("event:%d", event.ID)
}

func (s *Service) buildDesiredEvents(ctx context.Context, propertyID int64, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, fromDate, toDate string) ([]*store.CleaningCalendarEvent, error) {
	out := []*store.CleaningCalendarEvent{}
	namedTargets, err := s.Store.ListCleaningNamedStayTargets(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	for _, target := range namedTargets {
		event, err := s.buildNamedStayEvent(ctx, settings, profile, timezone, loc, target)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, nil
}

func (s *Service) buildNamedStayEvent(ctx context.Context, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, target store.CleaningNamedStayTarget) (*store.CleaningCalendarEvent, error) {
	cd, err := time.ParseInLocation("2006-01-02", target.CheckOutDate, loc)
	if err != nil {
		return nil, err
	}
	outH, outM := parseHM(profile.DefaultCheckOutTime, 10, 0)
	inH, inM := parseHM(profile.DefaultCheckInTime, 14, 0)
	starts := time.Date(cd.Year(), cd.Month(), cd.Day(), outH, outM, 0, 0, loc)
	ends := starts.Add(time.Duration(settings.DefaultDurationMinutes) * time.Minute)
	next, err := s.Store.FindCleaningCalendarSameDayNamedStayArrival(ctx, target.PropertyID, target.NamedStayID, target.CheckOutDate)
	if err != nil {
		return nil, err
	}
	var warning sql.NullString
	sameDay := next != nil
	if sameDay {
		ends = time.Date(cd.Year(), cd.Month(), cd.Day(), inH, inM, 0, 0, loc).Add(-time.Hour)
		if !ends.After(starts) {
			ends = starts.Add(minimalConflictDuration)
			warning = sql.NullString{String: "same-day check-in leaves less than one hour after checkout", Valid: true}
		}
	}
	event := &store.CleaningCalendarEvent{
		PropertyID:       target.PropertyID,
		NamedStayID:      sql.NullInt64{Int64: target.NamedStayID, Valid: true},
		CheckoutDate:     sql.NullString{String: target.CheckOutDate, Valid: true},
		CleaningKind:     store.CleaningKindNamedStay,
		CleaningIdentity: sql.NullString{String: store.NamedStayCleaningIdentity(target.PropertyID, target.NamedStayID, target.CheckOutDate), Valid: true},
		GoogleCalendarID: strings.TrimSpace(settings.CalendarID.String),
		CleaningDate:     target.CheckOutDate,
		StartsAt:         starts.UTC(),
		EndsAt:           ends.UTC(),
		SameDayArrival:   sameDay,
		Title:            renderTitle(settings, sameDay),
		Status:           store.CleaningCalendarStatusPending,
		WarningMessage:   warning,
		ErrorMessage:     sql.NullString{},
		PendingAction:    store.CleaningPendingActionUpsert,
	}
	event.ScheduleHash = sql.NullString{String: scheduleHash(timezone, target.CheckOutDate, profile.DefaultCheckOutTime, profile.DefaultCheckInTime, settings.DefaultDurationMinutes), Valid: true}
	event.DesiredHash = sql.NullString{String: desiredHash(event, timezone), Valid: true}
	return event, nil
}

func (s *Service) lookupExistingDesiredEvent(ctx context.Context, event *store.CleaningCalendarEvent) *store.CleaningCalendarEvent {
	if event.CleaningIdentity.Valid && strings.TrimSpace(event.CleaningIdentity.String) != "" {
		if existing, err := s.Store.GetCleaningCalendarEventByCleaningIdentity(ctx, event.CleaningIdentity.String); err == nil {
			return existing
		}
	}
	return nil
}

func (s *Service) canSkipGoogleUpsert(existing, desired *store.CleaningCalendarEvent, googleListed bool, googleSeen bool) bool {
	if existing == nil || existing.Status != store.CleaningCalendarStatusSynced || !existing.GoogleEventID.Valid || strings.TrimSpace(existing.GoogleEventID.String) == "" {
		return false
	}
	if desired == nil || !existing.DesiredHash.Valid || !desired.DesiredHash.Valid || strings.TrimSpace(existing.DesiredHash.String) != strings.TrimSpace(desired.DesiredHash.String) {
		return false
	}
	if googleListed && !googleSeen {
		return false
	}
	return true
}

func desiredHash(event *store.CleaningCalendarEvent, timezone string) string {
	parts := []string{
		event.GoogleCalendarID,
		event.Title,
		event.StartsAt.UTC().Format(time.RFC3339),
		event.EndsAt.UTC().Format(time.RFC3339),
		timezone,
		event.CleaningKind,
		nullStringValue(event.CleaningIdentity),
		fmt.Sprintf("stay:%d", nullInt64Value(event.NamedStayID)),
		fmt.Sprintf("same:%t", event.SameDayArrival),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func scheduleHash(timezone, checkout, checkoutTime, checkinTime string, duration int) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{timezone, checkout, checkoutTime, checkinTime, fmt.Sprintf("%d", duration)}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func nullStringValue(v sql.NullString) string {
	if v.Valid {
		return strings.TrimSpace(v.String)
	}
	return ""
}

func nullInt64Value(v sql.NullInt64) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

type googleEventIndex struct {
	listed         bool
	byID           map[string]GoogleCalendarEvent
	byCleaningID   map[string]GoogleCalendarEvent
	byIdentity     map[string]GoogleCalendarEvent
	byNamedStayDay map[string]GoogleCalendarEvent
}

func newGoogleEventIndex(events []GoogleCalendarEvent, propertyID int64, loc *time.Location) googleEventIndex {
	idx := googleEventIndex{
		listed:         true,
		byID:           map[string]GoogleCalendarEvent{},
		byCleaningID:   map[string]GoogleCalendarEvent{},
		byIdentity:     map[string]GoogleCalendarEvent{},
		byNamedStayDay: map[string]GoogleCalendarEvent{},
	}
	propID := fmt.Sprintf("%d", propertyID)
	for _, ev := range events {
		if strings.TrimSpace(ev.ID) == "" || ev.Status == "cancelled" {
			continue
		}
		idx.byID[ev.ID] = ev
		day := ev.Start.In(loc).Format("2006-01-02")
		priv := ev.PrivateProperties
		if priv == nil || strings.TrimSpace(priv["pms_property_id"]) != propID {
			continue
		}
		if v := strings.TrimSpace(priv["pms_cleaning_event_id"]); v != "" {
			indexGoogleEvent(idx.byCleaningID, v, ev)
		}
		if v := strings.TrimSpace(priv["pms_cleaning_identity"]); v != "" {
			indexGoogleEvent(idx.byIdentity, v, ev)
		}
		if v := strings.TrimSpace(priv["pms_named_stay_id"]); v != "" {
			indexGoogleEvent(idx.byNamedStayDay, v+"\x00"+day, ev)
		}
	}
	return idx
}

func indexGoogleEvent(index map[string]GoogleCalendarEvent, key string, event GoogleCalendarEvent) {
	current, exists := index[key]
	if !exists || event.ID < current.ID {
		index[key] = event
	}
}

func (idx googleEventIndex) match(desired, existing *store.CleaningCalendarEvent) string {
	if !idx.listed {
		return ""
	}
	if existing != nil && existing.GoogleEventID.Valid {
		if ev, ok := idx.byID[strings.TrimSpace(existing.GoogleEventID.String)]; ok {
			return ev.ID
		}
	}
	if desired != nil && desired.GoogleEventID.Valid {
		if ev, ok := idx.byID[strings.TrimSpace(desired.GoogleEventID.String)]; ok {
			return ev.ID
		}
	}
	if existing != nil && existing.ID > 0 {
		if ev, ok := idx.byCleaningID[fmt.Sprintf("%d", existing.ID)]; ok {
			return ev.ID
		}
	}
	if desired != nil && desired.CleaningIdentity.Valid {
		if ev, ok := idx.byIdentity[strings.TrimSpace(desired.CleaningIdentity.String)]; ok {
			return ev.ID
		}
	}
	if existing != nil && existing.CleaningIdentity.Valid {
		if ev, ok := idx.byIdentity[strings.TrimSpace(existing.CleaningIdentity.String)]; ok {
			return ev.ID
		}
	}
	if desired != nil && desired.NamedStayID.Valid {
		if ev, ok := idx.byNamedStayDay[fmt.Sprintf("%d\x00%s", desired.NamedStayID.Int64, desired.CleaningDate)]; ok {
			return ev.ID
		}
	}
	return ""
}

func (idx googleEventIndex) seen(event *store.CleaningCalendarEvent) bool {
	if !idx.listed {
		return false
	}
	if event == nil || !event.GoogleEventID.Valid {
		return false
	}
	_, ok := idx.byID[strings.TrimSpace(event.GoogleEventID.String)]
	return ok
}

func (s *Service) RetryEvent(ctx context.Context, propertyID, eventID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Store == nil {
		return errors.New("store not configured")
	}
	owner := fmt.Sprintf("cleaning-service-%p", s)
	acquired, err := s.Store.TryAcquireJobLease(ctx, fmt.Sprintf("cleaning_calendar_property_%d", propertyID), owner, 30*time.Minute)
	if err != nil {
		return err
	}
	if !acquired {
		return errors.New("cleaning calendar reconciliation already in progress")
	}
	defer s.Store.ReleaseJobLease(context.Background(), fmt.Sprintf("cleaning_calendar_property_%d", propertyID), owner)
	event, err := s.Store.GetCleaningCalendarEvent(ctx, propertyID, eventID)
	if err != nil {
		return err
	}
	if event.PendingAction == store.CleaningPendingActionDelete || event.Status == store.CleaningCalendarStatusRemoved {
		return s.syncDelete(ctx, event, 0)
	}
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return err
	}
	return s.syncUpsert(ctx, event, prop.Timezone, 0)
}

func (s *Service) syncUpsert(ctx context.Context, event *store.CleaningCalendarEvent, timezone string, runID int64) error {
	if err := s.Store.SetCleaningCalendarEventPendingAction(ctx, event.PropertyID, event.ID, store.CleaningPendingActionUpsert); err != nil {
		return err
	}
	if s.Client == nil || !s.Client.Configured() {
		msg := "google calendar client not configured"
		_ = s.Store.UpdateCleaningCalendarEventGoogleResult(ctx, event.PropertyID, event.ID, "", store.CleaningCalendarStatusError, &msg)
		insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "upsert_error", msg)
		return errors.New(msg)
	}
	googleID, err := s.Client.UpsertEvent(ctx, CalendarEventPayload{
		CalendarID:   event.GoogleCalendarID,
		Summary:      event.Title,
		Description:  "",
		Start:        event.StartsAt,
		End:          event.EndsAt,
		TimeZone:     timezone,
		PropertyID:   event.PropertyID,
		NamedStayID:  nullInt64Value(event.NamedStayID),
		Identity:     nullStringValue(event.CleaningIdentity),
		LocalEventID: event.ID,
	}, strings.TrimSpace(event.GoogleEventID.String))
	if err != nil {
		msg := err.Error()
		_ = s.Store.UpdateCleaningCalendarEventGoogleResult(ctx, event.PropertyID, event.ID, "", store.CleaningCalendarStatusError, &msg)
		insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "upsert_error", msg)
		return err
	}
	if err := s.Store.UpdateCleaningCalendarEventGoogleResult(ctx, event.PropertyID, event.ID, googleID, store.CleaningCalendarStatusSynced, nil); err != nil {
		return err
	}
	action := "upsert"
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, action, "synced")
	return nil
}

func (s *Service) syncDelete(ctx context.Context, event *store.CleaningCalendarEvent, runID int64) error {
	if err := s.Store.SetCleaningCalendarEventPendingAction(ctx, event.PropertyID, event.ID, store.CleaningPendingActionDelete); err != nil {
		return err
	}
	if event.GoogleEventID.Valid && strings.TrimSpace(event.GoogleEventID.String) != "" && (s.Client == nil || !s.Client.Configured()) {
		msg := "google calendar client not configured"
		_ = s.Store.UpdateCleaningCalendarEventGoogleResult(ctx, event.PropertyID, event.ID, "", store.CleaningCalendarStatusError, &msg)
		insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "delete_error", msg)
		return errors.New(msg)
	}
	if s.Client != nil && s.Client.Configured() && event.GoogleEventID.Valid && strings.TrimSpace(event.GoogleEventID.String) != "" {
		if err := s.Client.DeleteEvent(ctx, event.GoogleCalendarID, strings.TrimSpace(event.GoogleEventID.String)); err != nil {
			msg := err.Error()
			_ = s.Store.UpdateCleaningCalendarEventGoogleResult(ctx, event.PropertyID, event.ID, "", store.CleaningCalendarStatusError, &msg)
			insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "delete_error", msg)
			return err
		}
	}
	if err := s.Store.MarkCleaningCalendarEventRemoved(ctx, event.PropertyID, event.ID, nil); err != nil {
		return err
	}
	message := "removed"
	action := "delete"
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, action, message)
	return nil
}

func renderTitle(settings *store.GoogleCleaningSettings, sameDay bool) string {
	label := settings.NoGuestLabel
	if sameDay {
		label = settings.SameDayLabel
	}
	return strings.Join(strings.Fields(strings.TrimSpace(settings.TitlePrefix)+" "+strings.TrimSpace(label)), " ")
}

func parseHM(v string, defH, defM int) (int, int) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return defH, defM
	}
	var h, m int
	if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return defH, defM
	}
	return h, m
}

func insertLog(st *store.Store, ctx context.Context, propertyID, eventID, runID int64, action, message string) {
	var eid *int64
	if eventID > 0 {
		eid = &eventID
	}
	var rid *int64
	if runID > 0 {
		rid = &runID
	}
	_ = st.InsertCleaningCalendarEventLog(ctx, propertyID, eid, rid, action, message)
}
