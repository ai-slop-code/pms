package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	CleaningCalendarStatusPending = "pending"
	CleaningCalendarStatusSynced  = "synced"
	CleaningCalendarStatusError   = "error"
	CleaningCalendarStatusRemoved = "removed"
	CleaningPendingActionNone     = "none"
	CleaningPendingActionUpsert   = "upsert"
	CleaningPendingActionDelete   = "delete"

	defaultCleaningCalendarDuration = 180
	defaultCleaningCalendarPrefix   = "Upratovanie:"
	defaultCleaningCalendarSameDay  = "Pride Host"
	defaultCleaningCalendarNoGuest  = "Bez Hosta"
)

type GoogleCleaningSettings struct {
	PropertyID             int64
	Enabled                bool
	CalendarID             sql.NullString
	DefaultDurationMinutes int
	TitlePrefix            string
	SameDayLabel           string
	NoGuestLabel           string
	ConnectedAccountID     sql.NullString
	UpdatedAt              time.Time
}

type CleaningCalendarEvent struct {
	ID               int64
	PropertyID       int64
	NamedStayID      sql.NullInt64
	CheckoutDate     sql.NullString
	CleaningKind     string
	CleaningIdentity sql.NullString
	DesiredHash      sql.NullString
	GoogleCalendarID string
	GoogleEventID    sql.NullString
	CleaningDate     string
	StartsAt         time.Time
	EndsAt           time.Time
	SameDayArrival   bool
	Title            string
	Status           string
	WarningMessage   sql.NullString
	ErrorMessage     sql.NullString
	PendingAction    string
	ScheduleHash     sql.NullString
	LastSyncedAt     sql.NullTime
	LastGoogleSeenAt sql.NullTime
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const (
	CleaningKindNamedStay = "named_stay"
)

const cleaningCalendarColumns = `id, property_id, named_stay_id, checkout_date, cleaning_kind, cleaning_identity, desired_hash,
	google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
	same_day_arrival, title, status, warning_message, error_message, pending_action, schedule_hash, last_synced_at, last_google_seen_at, created_at, updated_at`

type CleaningCalendarSyncRun struct {
	ID             int64
	PropertyID     int64
	StartedAt      time.Time
	FinishedAt     sql.NullTime
	Status         string
	ErrorMessage   sql.NullString
	EventsSeen     int
	EventsUpserted int
	EventsRemoved  int
	Trigger        string
	CreatedAt      time.Time
}

type CleaningCalendarSettingsPatch struct {
	Enabled                *bool
	CalendarID             *string
	DefaultDurationMinutes *int
	TitlePrefix            *string
	SameDayLabel           *string
	NoGuestLabel           *string
	ConnectedAccountID     *string
}

func defaultGoogleCleaningSettings(propertyID int64) *GoogleCleaningSettings {
	return &GoogleCleaningSettings{
		PropertyID:             propertyID,
		DefaultDurationMinutes: defaultCleaningCalendarDuration,
		TitlePrefix:            defaultCleaningCalendarPrefix,
		SameDayLabel:           defaultCleaningCalendarSameDay,
		NoGuestLabel:           defaultCleaningCalendarNoGuest,
	}
}

func (s *Store) GetGoogleCleaningSettings(ctx context.Context, propertyID int64) (*GoogleCleaningSettings, error) {
	var row GoogleCleaningSettings
	var enabled int
	var updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT property_id, enabled, calendar_id, default_duration_minutes, title_prefix, same_day_label, no_guest_label, connected_account_id, updated_at
		FROM property_google_cleaning_settings
		WHERE property_id = ?`, propertyID).
		Scan(&row.PropertyID, &enabled, &row.CalendarID, &row.DefaultDurationMinutes, &row.TitlePrefix, &row.SameDayLabel, &row.NoGuestLabel, &row.ConnectedAccountID, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultGoogleCleaningSettings(propertyID), nil
	}
	if err != nil {
		return nil, err
	}
	row.Enabled = enabled == 1
	if row.DefaultDurationMinutes <= 0 {
		row.DefaultDurationMinutes = defaultCleaningCalendarDuration
	}
	if strings.TrimSpace(row.TitlePrefix) == "" {
		row.TitlePrefix = defaultCleaningCalendarPrefix
	}
	if strings.TrimSpace(row.SameDayLabel) == "" {
		row.SameDayLabel = defaultCleaningCalendarSameDay
	}
	if strings.TrimSpace(row.NoGuestLabel) == "" {
		row.NoGuestLabel = defaultCleaningCalendarNoGuest
	}
	row.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &row, nil
}

func (s *Store) UpdateGoogleCleaningSettings(ctx context.Context, propertyID int64, patch CleaningCalendarSettingsPatch) (*GoogleCleaningSettings, error) {
	settings, err := s.GetGoogleCleaningSettings(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	if patch.Enabled != nil {
		settings.Enabled = *patch.Enabled
	}
	if patch.CalendarID != nil {
		settings.CalendarID = nullStringFromTrimmed(*patch.CalendarID)
	}
	if patch.DefaultDurationMinutes != nil && *patch.DefaultDurationMinutes > 0 {
		settings.DefaultDurationMinutes = *patch.DefaultDurationMinutes
	}
	if patch.TitlePrefix != nil {
		settings.TitlePrefix = defaultIfBlank(*patch.TitlePrefix, defaultCleaningCalendarPrefix)
	}
	if patch.SameDayLabel != nil {
		settings.SameDayLabel = defaultIfBlank(*patch.SameDayLabel, defaultCleaningCalendarSameDay)
	}
	if patch.NoGuestLabel != nil {
		settings.NoGuestLabel = defaultIfBlank(*patch.NoGuestLabel, defaultCleaningCalendarNoGuest)
	}
	if patch.ConnectedAccountID != nil {
		settings.ConnectedAccountID = nullStringFromTrimmed(*patch.ConnectedAccountID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if settings.Enabled {
		enabled = 1
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO property_google_cleaning_settings (
			property_id, enabled, calendar_id, default_duration_minutes, title_prefix, same_day_label, no_guest_label, connected_account_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id) DO UPDATE SET
			enabled = excluded.enabled,
			calendar_id = excluded.calendar_id,
			default_duration_minutes = excluded.default_duration_minutes,
			title_prefix = excluded.title_prefix,
			same_day_label = excluded.same_day_label,
			no_guest_label = excluded.no_guest_label,
			connected_account_id = excluded.connected_account_id,
			updated_at = excluded.updated_at`,
		propertyID, enabled, nullStr(settings.CalendarID), settings.DefaultDurationMinutes, settings.TitlePrefix, settings.SameDayLabel, settings.NoGuestLabel, nullStr(settings.ConnectedAccountID), now)
	if err != nil {
		return nil, err
	}
	return s.GetGoogleCleaningSettings(ctx, propertyID)
}

func (s *Store) ListPropertyIDsWithGoogleCleaningSync(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id
		FROM properties p
		INNER JOIN property_google_cleaning_settings gcs ON gcs.property_id = p.id
		WHERE p.active = 1
		  AND gcs.enabled = 1
		  AND gcs.calendar_id IS NOT NULL
		  AND TRIM(gcs.calendar_id) != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) GetCleaningCalendarEventByCleaningIdentity(ctx context.Context, cleaningIdentity string) (*CleaningCalendarEvent, error) {
	rows, err := s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE cleaning_identity = ?`, cleaningIdentity)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

func (s *Store) UpsertCleaningCalendarEventByIdentity(ctx context.Context, event *CleaningCalendarEvent) (*CleaningCalendarEvent, error) {
	if !event.CleaningIdentity.Valid || strings.TrimSpace(event.CleaningIdentity.String) == "" {
		return nil, errors.New("cleaning identity required")
	}
	if !event.NamedStayID.Valid || event.NamedStayID.Int64 <= 0 {
		return nil, errors.New("named stay owner required")
	}
	return s.upsertCleaningCalendarEventByCleaningIdentity(ctx, event)
}

func (s *Store) upsertCleaningCalendarEventByCleaningIdentity(ctx context.Context, event *CleaningCalendarEvent) (*CleaningCalendarEvent, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	sameDay := 0
	if event.SameDayArrival {
		sameDay = 1
	}
	var googleEventID interface{}
	if event.GoogleEventID.Valid && strings.TrimSpace(event.GoogleEventID.String) != "" {
		googleEventID = strings.TrimSpace(event.GoogleEventID.String)
	}
	var warning interface{}
	if event.WarningMessage.Valid && strings.TrimSpace(event.WarningMessage.String) != "" {
		warning = strings.TrimSpace(event.WarningMessage.String)
	}
	var errMsg interface{}
	if event.ErrorMessage.Valid && strings.TrimSpace(event.ErrorMessage.String) != "" {
		errMsg = strings.TrimSpace(event.ErrorMessage.String)
	}
	var lastSynced interface{}
	if event.LastSyncedAt.Valid {
		lastSynced = event.LastSyncedAt.Time.UTC().Format(time.RFC3339)
	}
	var lastSeen interface{}
	if event.LastGoogleSeenAt.Valid {
		lastSeen = event.LastGoogleSeenAt.Time.UTC().Format(time.RFC3339)
	}
	pendingAction := event.PendingAction
	if pendingAction == "" {
		pendingAction = CleaningPendingActionUpsert
	}
	kind := event.CleaningKind
	if kind == "" {
		kind = CleaningKindNamedStay
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_calendar_events (
			property_id, named_stay_id, checkout_date, cleaning_kind, cleaning_identity, desired_hash,
			google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
			same_day_arrival, title, status, warning_message, error_message, pending_action, schedule_hash, last_synced_at, last_google_seen_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cleaning_identity) WHERE cleaning_identity IS NOT NULL DO UPDATE SET
			named_stay_id = excluded.named_stay_id,
			checkout_date = excluded.checkout_date,
			cleaning_kind = excluded.cleaning_kind,
			desired_hash = excluded.desired_hash,
			google_calendar_id = excluded.google_calendar_id,
			google_event_id = COALESCE(excluded.google_event_id, cleaning_calendar_events.google_event_id),
			cleaning_date = excluded.cleaning_date,
			starts_at = excluded.starts_at,
			ends_at = excluded.ends_at,
			same_day_arrival = excluded.same_day_arrival,
			title = excluded.title,
			status = excluded.status,
			warning_message = excluded.warning_message,
			error_message = excluded.error_message,
			pending_action = excluded.pending_action,
			schedule_hash = excluded.schedule_hash,
			last_synced_at = COALESCE(excluded.last_synced_at, cleaning_calendar_events.last_synced_at),
			last_google_seen_at = COALESCE(excluded.last_google_seen_at, cleaning_calendar_events.last_google_seen_at),
			updated_at = excluded.updated_at`,
		event.PropertyID, nullInt(event.NamedStayID), nullStr(event.CheckoutDate), kind,
		nullStr(event.CleaningIdentity), nullStr(event.DesiredHash), event.GoogleCalendarID, googleEventID, event.CleaningDate,
		event.StartsAt.UTC().Format(time.RFC3339), event.EndsAt.UTC().Format(time.RFC3339), sameDay, event.Title,
		event.Status, warning, errMsg, pendingAction, nullStr(event.ScheduleHash), lastSynced, lastSeen, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetCleaningCalendarEventByCleaningIdentity(ctx, event.CleaningIdentity.String)
}

func (s *Store) GetCleaningCalendarEvent(ctx context.Context, propertyID, eventID int64) (*CleaningCalendarEvent, error) {
	rows, err := s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND id = ?`, propertyID, eventID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

func (s *Store) ListCleaningCalendarEventsForMonth(ctx context.Context, propertyID int64, month string) ([]CleaningCalendarEvent, error) {
	return s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND substr(cleaning_date, 1, 7) = ?
		ORDER BY cleaning_date ASC, starts_at ASC`, propertyID, month)
}

func (s *Store) ListActiveCleaningCalendarEvents(ctx context.Context, propertyID int64) ([]CleaningCalendarEvent, error) {
	return s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND status <> 'removed'
		ORDER BY cleaning_date ASC, starts_at ASC`, propertyID)
}

func (s *Store) ListActiveCleaningCalendarEventsForDateRange(ctx context.Context, propertyID int64, fromDate, toDate string) ([]CleaningCalendarEvent, error) {
	return s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND status <> 'removed'
		  AND cleaning_date >= ? AND cleaning_date <= ?
		ORDER BY cleaning_date ASC, starts_at ASC`, propertyID, fromDate, toDate)
}

func (s *Store) MarkCleaningCalendarEventRemoved(ctx context.Context, propertyID, eventID int64, errMsg *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_events
		SET status = 'removed', pending_action = 'none', error_message = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, nullableString(ptrStringValue(errMsg)), now, propertyID, eventID)
	return err
}

func (s *Store) UpdateCleaningCalendarEventGoogleResult(ctx context.Context, propertyID, eventID int64, googleEventID string, status string, errMsg *string) error {
	now := time.Now().UTC()
	var gid interface{}
	if strings.TrimSpace(googleEventID) != "" {
		gid = strings.TrimSpace(googleEventID)
	}
	var synced interface{}
	if status == CleaningCalendarStatusSynced || status == CleaningCalendarStatusRemoved {
		synced = now.Format(time.RFC3339)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_events
		SET google_event_id = COALESCE(?, google_event_id), status = ?, pending_action = CASE WHEN ? = 'error' THEN pending_action ELSE 'none' END, error_message = ?, last_synced_at = COALESCE(?, last_synced_at), updated_at = ?
		WHERE property_id = ? AND id = ?`, gid, status, status, nullableString(ptrStringValue(errMsg)), synced, now.Format(time.RFC3339), propertyID, eventID)
	return err
}

func (s *Store) SetCleaningCalendarEventPendingAction(ctx context.Context, propertyID, eventID int64, action string) error {
	if action != CleaningPendingActionNone && action != CleaningPendingActionUpsert && action != CleaningPendingActionDelete {
		return errors.New("invalid cleaning pending action")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE cleaning_calendar_events SET pending_action = ?, updated_at = ? WHERE property_id = ? AND id = ?`, action, time.Now().UTC().Format(time.RFC3339), propertyID, eventID)
	return err
}

func (s *Store) MarkCleaningCalendarEventGoogleSeen(ctx context.Context, propertyID, eventID int64, googleEventID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_events
		SET google_event_id = COALESCE(?, google_event_id), last_google_seen_at = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, nullableString(strings.TrimSpace(googleEventID)), now, now, propertyID, eventID)
	return err
}

type CleaningNamedStayTarget struct {
	NamedStayID  int64
	PropertyID   int64
	DisplayName  string
	StayType     string
	CheckInDate  string
	CheckOutDate string
}

func (s *Store) ListCleaningNamedStayTargets(ctx context.Context, propertyID int64, fromDate, toDate string) ([]CleaningNamedStayTarget, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date
		FROM named_stays ns
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND ns.cleaning_required = 1
		  AND COALESCE(ns.review_resolution, ns.review_status, 'confirmed') = 'confirmed'
		  AND COALESCE(ns.stay_outcome, '') NOT IN ('no_show', 'cancelled_non_refundable')
		  AND ns.check_out_date >= ? AND ns.check_out_date <= ?
		ORDER BY ns.check_out_date ASC, ns.id ASC`, propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CleaningNamedStayTarget{}
	for rows.Next() {
		var row CleaningNamedStayTarget
		if err := rows.Scan(&row.NamedStayID, &row.PropertyID, &row.DisplayName, &row.StayType, &row.CheckInDate, &row.CheckOutDate); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) FindCleaningCalendarSameDayNamedStayArrival(ctx context.Context, propertyID, checkoutNamedStayID int64, checkoutDate string) (*CleaningNamedStayTarget, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date
		FROM named_stays ns
		JOIN named_stay_nights nsn ON nsn.named_stay_id = ns.id
		WHERE ns.property_id = ?
		  AND ns.id <> ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_resolution, ns.review_status, 'confirmed') = 'confirmed'
		  AND COALESCE(ns.stay_outcome, '') NOT IN ('no_show', 'cancelled_non_refundable')
		  AND nsn.property_id = ?
		  AND nsn.active = 1
		  AND nsn.local_night_date = ?
		ORDER BY ns.id ASC
		LIMIT 1`, propertyID, checkoutNamedStayID, propertyID, checkoutDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var row CleaningNamedStayTarget
	if err := rows.Scan(&row.NamedStayID, &row.PropertyID, &row.DisplayName, &row.StayType, &row.CheckInDate, &row.CheckOutDate); err != nil {
		return nil, err
	}
	return &row, rows.Err()
}

func NamedStayCleaningIdentity(propertyID, namedStayID int64, checkoutDate string) string {
	return fmt.Sprintf("stay:%d:%d:%s", propertyID, namedStayID, checkoutDate)
}

func (s *Store) StartCleaningCalendarSyncRun(ctx context.Context, propertyID int64, trigger string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_calendar_sync_runs (property_id, started_at, status, trigger, created_at)
		VALUES (?, ?, 'running', ?, ?)`, propertyID, now, trigger, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishCleaningCalendarSyncRun(ctx context.Context, runID int64, status string, errMsg *string, seen, upserted, removed int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_sync_runs
		SET finished_at = ?, status = ?, error_message = ?, events_seen = ?, events_upserted = ?, events_removed = ?
		WHERE id = ?`, now, status, nullableString(ptrStringValue(errMsg)), seen, upserted, removed, runID)
	return err
}

func (s *Store) ListCleaningCalendarSyncRuns(ctx context.Context, propertyID int64, limit int) ([]CleaningCalendarSyncRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, started_at, finished_at, status, error_message, events_seen, events_upserted, events_removed, trigger, created_at
		FROM cleaning_calendar_sync_runs
		WHERE property_id = ?
		ORDER BY started_at DESC
		LIMIT ?`, propertyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleaningCalendarSyncRun, 0)
	for rows.Next() {
		var row CleaningCalendarSyncRun
		var started, created string
		var finished sql.NullString
		if err := rows.Scan(&row.ID, &row.PropertyID, &started, &finished, &row.Status, &row.ErrorMessage, &row.EventsSeen, &row.EventsUpserted, &row.EventsRemoved, &row.Trigger, &created); err != nil {
			return nil, err
		}
		row.StartedAt, _ = time.Parse(time.RFC3339, started)
		row.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if finished.Valid && finished.String != "" {
			t, _ := time.Parse(time.RFC3339, finished.String)
			row.FinishedAt = sql.NullTime{Time: t, Valid: true}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) InsertCleaningCalendarEventLog(ctx context.Context, propertyID int64, eventID *int64, syncRunID *int64, action, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_calendar_event_logs (property_id, cleaning_calendar_event_id, sync_run_id, action, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, propertyID, eventID, syncRunID, action, nullableString(message), now)
	return err
}

func (s *Store) scanCleaningCalendarEvents(ctx context.Context, q string, args ...interface{}) ([]CleaningCalendarEvent, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CleaningCalendarEvent, 0)
	for rows.Next() {
		var row CleaningCalendarEvent
		var starts, ends, created, updated string
		var sameDay int
		var lastSynced, lastSeen sql.NullString
		if err := rows.Scan(&row.ID, &row.PropertyID, &row.NamedStayID, &row.CheckoutDate, &row.CleaningKind,
			&row.CleaningIdentity, &row.DesiredHash, &row.GoogleCalendarID, &row.GoogleEventID, &row.CleaningDate, &starts, &ends,
			&sameDay, &row.Title, &row.Status, &row.WarningMessage, &row.ErrorMessage, &row.PendingAction, &row.ScheduleHash, &lastSynced, &lastSeen, &created, &updated); err != nil {
			return nil, err
		}
		row.StartsAt, _ = time.Parse(time.RFC3339, starts)
		row.EndsAt, _ = time.Parse(time.RFC3339, ends)
		row.SameDayArrival = sameDay == 1
		if lastSynced.Valid && lastSynced.String != "" {
			t, _ := time.Parse(time.RFC3339, lastSynced.String)
			row.LastSyncedAt = sql.NullTime{Time: t, Valid: true}
		}
		if lastSeen.Valid && lastSeen.String != "" {
			t, _ := time.Parse(time.RFC3339, lastSeen.String)
			row.LastGoogleSeenAt = sql.NullTime{Time: t, Valid: true}
		}
		row.CreatedAt, _ = time.Parse(time.RFC3339, created)
		row.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, row)
	}
	return out, rows.Err()
}

func nullStringFromTrimmed(v string) sql.NullString {
	v = strings.TrimSpace(v)
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func defaultIfBlank(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func nullInt(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}
