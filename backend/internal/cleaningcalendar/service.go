package cleaningcalendar

import (
	"context"
	"database/sql"
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
	UpsertEvent(ctx context.Context, event CalendarEventPayload, googleEventID string) (string, error)
	DeleteEvent(ctx context.Context, calendarID, googleEventID string) error
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
}

func (s *Service) ReconcileProperty(ctx context.Context, propertyID int64, trigger string) (*ReconcileStats, error) {
	if s.Store == nil {
		return nil, errors.New("store not configured")
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
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	windowStart := now.AddDate(0, 0, -reconcilePastWindowDays)
	windowEnd := now.AddDate(0, 0, reconcileFutureWindowDays)
	candidates, err := s.Store.ListCleaningCalendarCheckoutCandidates(ctx, propertyID, windowStart, windowEnd)
	if err != nil {
		return finish(err)
	}
	eligible := make(map[int64]struct{}, len(candidates))
	for _, occ := range candidates {
		stats.EventsSeen++
		eligible[occ.ID] = struct{}{}
		event, err := s.buildEvent(ctx, settings, profile, prop.Timezone, loc, occ)
		if err != nil {
			msg := err.Error()
			runErr = &msg
			_ = s.Store.InsertCleaningCalendarEventLog(ctx, propertyID, nil, &runID, "build_error", msg)
			continue
		}
		if existing, err := s.Store.GetCleaningCalendarEventByOccupancy(ctx, propertyID, occ.ID); err == nil {
			preserveExistingWindowForSameDayOnlyChange(existing, event)
		}
		saved, err := s.Store.UpsertCleaningCalendarEvent(ctx, event)
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
		if _, ok := eligible[ev.OccupancyID]; ok {
			continue
		}
		removalReason := s.removalReason(ctx, propertyID, ev.OccupancyID)
		if err := s.syncDelete(ctx, &ev, runID, removalReason); err != nil {
			msg := err.Error()
			runErr = &msg
			continue
		}
		stats.EventsRemoved++
	}
	return finish(nil)
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

func (s *Service) buildEvent(ctx context.Context, settings *store.GoogleCleaningSettings, profile *store.PropertyProfile, timezone string, loc *time.Location, occ store.Occupancy) (*store.CleaningCalendarEvent, error) {
	checkoutDay := occ.EndAt.In(loc)
	dayStart := time.Date(checkoutDay.Year(), checkoutDay.Month(), checkoutDay.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	next, err := s.Store.FindCleaningCalendarSameDayArrival(ctx, occ.PropertyID, occ.ID, dayStart.UTC(), dayEnd.UTC())
	if err != nil {
		return nil, err
	}
	outH, outM := parseHM(profile.DefaultCheckOutTime, 10, 0)
	inH, inM := parseHM(profile.DefaultCheckInTime, 14, 0)
	starts := time.Date(checkoutDay.Year(), checkoutDay.Month(), checkoutDay.Day(), outH, outM, 0, 0, loc)
	ends := starts.Add(time.Duration(settings.DefaultDurationMinutes) * time.Minute)
	sameDay := next != nil
	var nextID sql.NullInt64
	var warning sql.NullString
	if sameDay {
		nextID = sql.NullInt64{Int64: next.ID, Valid: true}
		ends = time.Date(checkoutDay.Year(), checkoutDay.Month(), checkoutDay.Day(), inH, inM, 0, 0, loc).Add(-time.Hour)
		if !ends.After(starts) {
			ends = starts.Add(minimalConflictDuration)
			warning = sql.NullString{String: "same-day check-in leaves less than one hour after checkout", Valid: true}
		}
	}
	title := renderTitle(settings, sameDay)
	return &store.CleaningCalendarEvent{
		PropertyID:       occ.PropertyID,
		OccupancyID:      occ.ID,
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
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "upsert", "synced")
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
	insertLog(s.Store, ctx, event.PropertyID, event.ID, runID, "delete", message)
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
