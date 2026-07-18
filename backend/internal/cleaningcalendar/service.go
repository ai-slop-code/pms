package cleaningcalendar

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"pms/backend/internal/store"
)

const (
	reconcilePastWindowDays   = 30
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
	OccupancyID  int64
	NamedStayID  int64
	RawBlockID   int64
	Identity     string
	LocalEventID int64
}

type Service struct {
	Store  *store.Store
	Client CalendarClient
	Now    func() time.Time
}

type ReconcileStats struct {
	EventsSeen     int `json:"events_seen"`
	EventsUpserted int `json:"events_upserted"`
	EventsRemoved  int `json:"events_removed"`
	// PMS_19 §12 observability: provisional (unnamed-block) checkout counts.
	ProvisionalCreated int `json:"provisional_cleaning_events_created"`
	ProvisionalRemoved int `json:"provisional_cleaning_events_removed"`
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
	windowStart := now.AddDate(0, 0, -reconcilePastWindowDays)
	windowEnd := now.AddDate(0, 0, reconcileFutureWindowDays)
	fromDate := windowStart.In(loc).Format("2006-01-02")
	toDate := windowEnd.In(loc).Format("2006-01-02")
	return s.ReconcilePropertyDateRange(ctx, propertyID, fromDate, toDate, trigger)
}

func (s *Service) ReconcilePropertyDateRange(ctx context.Context, propertyID int64, fromDate, toDate, trigger string) (*ReconcileStats, error) {
	if s.Store == nil {
		return nil, errors.New("store not configured")
	}
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
	googleIndex := googleEventIndex{}
	if s.Client != nil && s.Client.Configured() {
		googleEvents, err := s.Client.ListEvents(ctx, settings.CalendarID.String, dayStart, dayAfterEnd)
		if err != nil {
			return finish(err)
		}
		googleIndex = newGoogleEventIndex(googleEvents, propertyID, loc)
	}
	desiredEvents, err := s.buildDesiredEvents(ctx, propertyID, settings, profile, prop.Timezone, loc, fromDate, toDate)
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
			preserveExistingWindowForSameDayOnlyChange(existing, event)
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
		if event.CleaningKind == store.CleaningKindProvisionalBlock {
			stats.ProvisionalCreated++
		}
		if err := s.syncUpsert(ctx, saved, prop.Timezone, runID); err != nil {
			msg := err.Error()
			runErr = &msg
		}
	}
	active, err := s.Store.ListActiveCleaningCalendarEventsForDateRange(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return finish(err)
	}
	for _, ev := range active {
		if _, ok := desired[cleaningEventKey(&ev)]; ok {
			continue
		}
		if ev.GoogleEventID.Valid {
			if _, reused := reusedGoogleIDs[strings.TrimSpace(ev.GoogleEventID.String)]; reused {
				if err := s.Store.MarkCleaningCalendarEventRemoved(ctx, ev.PropertyID, ev.ID, nil); err != nil {
					return finish(err)
				}
				stats.EventsRemoved++
				continue
			}
		}
		if matchedID := googleIndex.match(&ev, &ev); matchedID != "" && !ev.GoogleEventID.Valid {
			ev.GoogleEventID = sql.NullString{String: matchedID, Valid: true}
			_ = s.Store.MarkCleaningCalendarEventGoogleSeen(ctx, ev.PropertyID, ev.ID, matchedID)
		}
		removalReason := s.removalReason(ctx, propertyID, ev.OccupancyID)
		if err := s.syncDelete(ctx, &ev, runID, removalReason); err != nil {
			msg := err.Error()
			runErr = &msg
			continue
		}
		stats.EventsRemoved++
		if ev.CleaningKind == store.CleaningKindProvisionalBlock {
			stats.ProvisionalRemoved++
		}
	}
	return finish(nil)
}

func cleaningIdentityKey(upstreamUID, checkoutDate, kind string) string {
	return upstreamUID + "\x00" + checkoutDate + "\x00" + kind
}

func cleaningEventKey(event *store.CleaningCalendarEvent) string {
	if event == nil {
		return ""
	}
	if event.CleaningIdentity.Valid && strings.TrimSpace(event.CleaningIdentity.String) != "" {
		return "identity:" + strings.TrimSpace(event.CleaningIdentity.String)
	}
	if event.UpstreamEventUID.Valid && event.CheckoutDate.Valid {
		return "legacy:" + cleaningIdentityKey(event.UpstreamEventUID.String, event.CheckoutDate.String, event.CleaningKind)
	}
	return fmt.Sprintf("event:%d", event.ID)
}

func (s *Service) buildDesiredEvents(ctx context.Context, propertyID int64, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, fromDate, toDate string) ([]*store.CleaningCalendarEvent, error) {
	hasNewModel, err := s.Store.PropertyHasPMS21CleaningSources(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	if !hasNewModel {
		return s.buildLegacyDesiredEvents(ctx, propertyID, settings, profile, timezone, loc, fromDate, toDate)
	}
	out := []*store.CleaningCalendarEvent{}
	rawTargets, err := s.Store.ListCleaningRawProvisionalTargets(ctx, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	for _, target := range rawTargets {
		event, err := s.buildRawProvisionalEvent(settings, profile, timezone, loc, propertyID, target)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
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

func (s *Service) buildLegacyDesiredEvents(ctx context.Context, propertyID int64, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, fromDate, toDate string) ([]*store.CleaningCalendarEvent, error) {
	from, err := time.ParseInLocation("2006-01-02", fromDate, loc)
	if err != nil {
		return nil, err
	}
	to, err := time.ParseInLocation("2006-01-02", toDate, loc)
	if err != nil {
		return nil, err
	}
	candidates, err := s.Store.ListCleaningEligibleOccupancies(ctx, propertyID, from.AddDate(0, 0, -1).UTC(), to.AddDate(0, 0, 1).UTC())
	if err != nil {
		return nil, err
	}
	hasCoverage, err := s.Store.PropertyHasOccupancyNights(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	out := []*store.CleaningCalendarEvent{}
	for _, occ := range candidates {
		if !occ.UpstreamEventUID.Valid || strings.TrimSpace(occ.UpstreamEventUID.String) == "" {
			continue
		}
		dates, kind := s.Store.CleaningCheckoutDatesForOccupancy(ctx, &occ, hasCoverage)
		for _, d := range dates {
			if d < fromDate || d > toDate {
				continue
			}
			event, err := s.buildEvent(ctx, settings, profile, timezone, loc, occ, d, kind)
			if err != nil {
				return nil, err
			}
			event.DesiredHash = sql.NullString{String: desiredHash(event, timezone), Valid: true}
			out = append(out, event)
		}
	}
	return out, nil
}

func (s *Service) buildRawProvisionalEvent(settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, propertyID int64, target store.CleaningRawProvisionalTarget) (*store.CleaningCalendarEvent, error) {
	cd, err := time.ParseInLocation("2006-01-02", target.CheckoutDate, loc)
	if err != nil {
		return nil, err
	}
	outH, outM := parseHM(profile.DefaultCheckOutTime, 10, 0)
	starts := time.Date(cd.Year(), cd.Month(), cd.Day(), outH, outM, 0, 0, loc)
	ends := starts.Add(time.Duration(settings.DefaultDurationMinutes) * time.Minute)
	event := &store.CleaningCalendarEvent{
		PropertyID:        propertyID,
		OccupancyID:       nullInt64Value(target.LegacyOccupancyID),
		RawBookingBlockID: target.RawBookingBlockID,
		UpstreamEventUID:  target.UpstreamEventUID,
		CheckoutDate:      sql.NullString{String: target.CheckoutDate, Valid: true},
		CleaningKind:      store.CleaningKindProvisionalBlock,
		CleaningIdentity:  sql.NullString{String: store.RawProvisionalCleaningIdentity(propertyID, target.CheckoutDate), Valid: true},
		GoogleCalendarID:  strings.TrimSpace(settings.CalendarID.String),
		CleaningDate:      target.CheckoutDate,
		StartsAt:          starts.UTC(),
		EndsAt:            ends.UTC(),
		Title:             "Upratovanie",
		Status:            store.CleaningCalendarStatusPending,
		WarningMessage:    sql.NullString{},
		ErrorMessage:      sql.NullString{},
	}
	event.DesiredHash = sql.NullString{String: desiredHash(event, timezone), Valid: true}
	return event, nil
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
	var nextID sql.NullInt64
	var warning sql.NullString
	sameDay := next != nil
	if sameDay {
		nextID = next.LegacyOccupancyID
		ends = time.Date(cd.Year(), cd.Month(), cd.Day(), inH, inM, 0, 0, loc).Add(-time.Hour)
		if !ends.After(starts) {
			ends = starts.Add(minimalConflictDuration)
			warning = sql.NullString{String: "same-day check-in leaves less than one hour after checkout", Valid: true}
		}
	}
	event := &store.CleaningCalendarEvent{
		PropertyID:       target.PropertyID,
		OccupancyID:      nullInt64Value(target.LegacyOccupancyID),
		NamedStayID:      sql.NullInt64{Int64: target.NamedStayID, Valid: true},
		CheckoutDate:     sql.NullString{String: target.CheckOutDate, Valid: true},
		CleaningKind:     store.CleaningKindNamedStay,
		CleaningIdentity: sql.NullString{String: store.NamedStayCleaningIdentity(target.PropertyID, target.NamedStayID, target.CheckOutDate), Valid: true},
		GoogleCalendarID: strings.TrimSpace(settings.CalendarID.String),
		CleaningDate:     target.CheckOutDate,
		StartsAt:         starts.UTC(),
		EndsAt:           ends.UTC(),
		SameDayArrival:   sameDay,
		NextOccupancyID:  nextID,
		Title:            renderTitle(settings, sameDay),
		Status:           store.CleaningCalendarStatusPending,
		WarningMessage:   warning,
		ErrorMessage:     sql.NullString{},
	}
	event.DesiredHash = sql.NullString{String: desiredHash(event, timezone), Valid: true}
	return event, nil
}

func (s *Service) lookupExistingDesiredEvent(ctx context.Context, event *store.CleaningCalendarEvent) *store.CleaningCalendarEvent {
	if event.CleaningIdentity.Valid && strings.TrimSpace(event.CleaningIdentity.String) != "" {
		if existing, err := s.Store.GetCleaningCalendarEventByCleaningIdentity(ctx, event.CleaningIdentity.String); err == nil {
			return existing
		}
	}
	if event.UpstreamEventUID.Valid && event.CheckoutDate.Valid {
		if existing, err := s.Store.GetCleaningCalendarEventByIdentity(ctx, event.PropertyID, event.UpstreamEventUID.String, event.CheckoutDate.String, event.CleaningKind); err == nil {
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
		fmt.Sprintf("occ:%d", event.OccupancyID),
		fmt.Sprintf("stay:%d", nullInt64Value(event.NamedStayID)),
		fmt.Sprintf("raw:%d", nullInt64Value(event.RawBookingBlockID)),
		fmt.Sprintf("same:%t", event.SameDayArrival),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
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
	byRawBlockDay  map[string]GoogleCalendarEvent
	bySummaryDay   map[string]GoogleCalendarEvent
}

func newGoogleEventIndex(events []GoogleCalendarEvent, propertyID int64, loc *time.Location) googleEventIndex {
	idx := googleEventIndex{
		listed:         true,
		byID:           map[string]GoogleCalendarEvent{},
		byCleaningID:   map[string]GoogleCalendarEvent{},
		byIdentity:     map[string]GoogleCalendarEvent{},
		byNamedStayDay: map[string]GoogleCalendarEvent{},
		byRawBlockDay:  map[string]GoogleCalendarEvent{},
		bySummaryDay:   map[string]GoogleCalendarEvent{},
	}
	propID := fmt.Sprintf("%d", propertyID)
	for _, ev := range events {
		if strings.TrimSpace(ev.ID) == "" || ev.Status == "cancelled" {
			continue
		}
		idx.byID[ev.ID] = ev
		day := ev.Start.In(loc).Format("2006-01-02")
		if strings.TrimSpace(ev.Summary) != "" {
			idx.bySummaryDay[day+"\x00"+strings.TrimSpace(ev.Summary)] = ev
		}
		priv := ev.PrivateProperties
		if priv == nil || strings.TrimSpace(priv["pms_property_id"]) != propID {
			continue
		}
		if v := strings.TrimSpace(priv["pms_cleaning_event_id"]); v != "" {
			idx.byCleaningID[v] = ev
		}
		if v := strings.TrimSpace(priv["pms_cleaning_identity"]); v != "" {
			idx.byIdentity[v] = ev
		}
		if v := strings.TrimSpace(priv["pms_named_stay_id"]); v != "" {
			idx.byNamedStayDay[v+"\x00"+day] = ev
		}
		if v := strings.TrimSpace(priv["pms_raw_booking_block_id"]); v != "" {
			idx.byRawBlockDay[v+"\x00"+day] = ev
		}
	}
	return idx
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
	if desired != nil && desired.RawBookingBlockID.Valid {
		if ev, ok := idx.byRawBlockDay[fmt.Sprintf("%d\x00%s", desired.RawBookingBlockID.Int64, desired.CleaningDate)]; ok {
			return ev.ID
		}
	}
	if desired != nil {
		if ev, ok := idx.bySummaryDay[desired.CleaningDate+"\x00"+strings.TrimSpace(desired.Title)]; ok {
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

func preserveExistingWindowForSameDayOnlyChange(existing *store.CleaningCalendarEvent, next *store.CleaningCalendarEvent) {
	if existing == nil || next == nil {
		return
	}
	if existing.CleaningDate != next.CleaningDate || !existing.StartsAt.Equal(next.StartsAt) {
		return
	}
	if existing.SameDayArrival == next.SameDayArrival {
		return
	}
	next.EndsAt = existing.EndsAt
	next.WarningMessage = existing.WarningMessage
}

func (s *Service) RetryEvent(ctx context.Context, propertyID, eventID int64) error {
	event, err := s.Store.GetCleaningCalendarEvent(ctx, propertyID, eventID)
	if err != nil {
		return err
	}
	if event.Status == store.CleaningCalendarStatusRemoved {
		return s.syncDelete(ctx, event, 0, nil)
	}
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return err
	}
	return s.syncUpsert(ctx, event, prop.Timezone, 0)
}

func (s *Service) buildEvent(ctx context.Context, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, occ store.Occupancy, checkoutDate, kind string) (*store.CleaningCalendarEvent, error) {
	cd, err := time.ParseInLocation("2006-01-02", checkoutDate, loc)
	if err != nil {
		return nil, err
	}
	dayStart := time.Date(cd.Year(), cd.Month(), cd.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	next, err := s.Store.FindCleaningCalendarSameDayArrival(ctx, occ.PropertyID, occ.ID, dayStart.UTC(), dayEnd.UTC())
	if err != nil {
		return nil, err
	}
	outH, outM := parseHM(profile.DefaultCheckOutTime, 10, 0)
	inH, inM := parseHM(profile.DefaultCheckInTime, 14, 0)
	starts := time.Date(cd.Year(), cd.Month(), cd.Day(), outH, outM, 0, 0, loc)
	ends := starts.Add(time.Duration(settings.DefaultDurationMinutes) * time.Minute)
	sameDay := next != nil
	var nextID sql.NullInt64
	var warning sql.NullString
	if sameDay {
		nextID = sql.NullInt64{Int64: next.ID, Valid: true}
		ends = time.Date(cd.Year(), cd.Month(), cd.Day(), inH, inM, 0, 0, loc).Add(-time.Hour)
		if !ends.After(starts) {
			ends = starts.Add(minimalConflictDuration)
			warning = sql.NullString{String: "same-day check-in leaves less than one hour after checkout", Valid: true}
		}
	}
	title := renderTitle(settings, sameDay)
	return &store.CleaningCalendarEvent{
		PropertyID:       occ.PropertyID,
		OccupancyID:      occ.ID,
		UpstreamEventUID: occ.UpstreamEventUID,
		CheckoutDate:     sql.NullString{String: checkoutDate, Valid: true},
		CleaningKind:     kind,
		GoogleCalendarID: strings.TrimSpace(settings.CalendarID.String),
		CleaningDate:     dayStart.Format("2006-01-02"),
		StartsAt:         starts.UTC(),
		EndsAt:           ends.UTC(),
		SameDayArrival:   sameDay,
		NextOccupancyID:  nextID,
		Title:            title,
		Status:           store.CleaningCalendarStatusPending,
		WarningMessage:   warning,
		ErrorMessage:     sql.NullString{},
	}, nil
}

func (s *Service) syncUpsert(ctx context.Context, event *store.CleaningCalendarEvent, timezone string, runID int64) error {
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
		OccupancyID:  event.OccupancyID,
		NamedStayID:  nullInt64Value(event.NamedStayID),
		RawBlockID:   nullInt64Value(event.RawBookingBlockID),
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
	if event.CleaningKind == store.CleaningKindProvisionalBlock {
		action = "cleaning_provisional_created" // PMS_19 §11B
		_ = s.Store.InsertAuditLog(ctx, nil, action, "cleaning_calendar_event", fmt.Sprintf("%d", event.ID), "synced", "system", "cleaning_sync")
	}
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, action, "synced")
	return nil
}

func (s *Service) syncDelete(ctx context.Context, event *store.CleaningCalendarEvent, runID int64, removalReason *string) error {
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
	if err := s.Store.MarkCleaningCalendarEventRemoved(ctx, event.PropertyID, event.ID, removalReason); err != nil {
		return err
	}
	message := "removed"
	if removalReason != nil && strings.TrimSpace(*removalReason) != "" {
		message = strings.TrimSpace(*removalReason)
	}
	action := "delete"
	if event.CleaningKind == store.CleaningKindProvisionalBlock {
		action = "cleaning_provisional_removed" // PMS_19 §11B
		_ = s.Store.InsertAuditLog(ctx, nil, action, "cleaning_calendar_event", fmt.Sprintf("%d", event.ID), message, "system", "cleaning_sync")
	}
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, action, message)
	return nil
}

func (s *Service) removalReason(ctx context.Context, propertyID, occupancyID int64) *string {
	occ, err := s.Store.GetOccupancyByID(ctx, propertyID, occupancyID)
	if err != nil {
		return nil
	}
	if occ.StayOutcome.Valid {
		msg := "stay outcome: " + occ.StayOutcome.String
		return &msg
	}
	if occ.CleaningCalendarExcluded {
		msg := "manual cleaning calendar exclusion"
		return &msg
	}
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
