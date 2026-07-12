package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	CleaningCalendarStatusPending = "pending"
	CleaningCalendarStatusSynced  = "synced"
	CleaningCalendarStatusError   = "error"
	CleaningCalendarStatusRemoved = "removed"

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
	OccupancyID      int64
	UpstreamEventUID sql.NullString
	CheckoutDate     sql.NullString
	CleaningKind     string
	GoogleCalendarID string
	GoogleEventID    sql.NullString
	CleaningDate     string
	StartsAt         time.Time
	EndsAt           time.Time
	SameDayArrival   bool
	NextOccupancyID  sql.NullInt64
	Title            string
	Status           string
	WarningMessage   sql.NullString
	ErrorMessage     sql.NullString
	LastSyncedAt     sql.NullTime
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PMS_19 §5.4 cleaning_kind values.
const (
	CleaningKindProvisionalBlock = "provisional_block"
	CleaningKindNamedStay        = "named_stay"
)

const cleaningCalendarColumns = `id, property_id, occupancy_id, upstream_event_uid, checkout_date, cleaning_kind,
	google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
	same_day_arrival, next_occupancy_id, title, status, warning_message, error_message, last_synced_at, created_at, updated_at`

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

// ListCleaningEligibleOccupancies returns cleaning-eligible occupancies that
// overlap [fromUTC, toUTC). PMS_19 §5.4 needs the whole block (not just its end)
// so every provisional per-night checkout inside the window is covered.
func (s *Store) ListCleaningEligibleOccupancies(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]Occupancy, error) {
	q := occupancySelectColumns + `
		FROM occupancies
		WHERE property_id = ?
		  AND status IN ('active', 'updated')
		  AND superseded_at IS NULL
		  AND (closure_state IS NULL OR closure_state <> 'closed')
		  AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		  AND cleaning_calendar_excluded = 0
		  AND start_at < ?
		  AND end_at > ?
		ORDER BY end_at ASC`
	return s.scanOccupancies(ctx, q, propertyID, toUTC.UTC().Format(time.RFC3339), fromUTC.UTC().Format(time.RFC3339))
}

func (s *Store) ListCleaningCalendarCheckoutCandidates(ctx context.Context, propertyID int64, fromUTC, toUTC time.Time) ([]Occupancy, error) {
	q := occupancySelectColumns + `
		FROM occupancies
		WHERE property_id = ?
		  AND status IN ('active', 'updated')
		  AND superseded_at IS NULL
		  AND (closure_state IS NULL OR closure_state <> 'closed')
		  AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		  AND cleaning_calendar_excluded = 0
		  AND end_at >= ?
		  AND end_at < ?
		ORDER BY end_at ASC`
	return s.scanOccupancies(ctx, q, propertyID, fromUTC.UTC().Format(time.RFC3339), toUTC.UTC().Format(time.RFC3339))
}

func (s *Store) FindCleaningCalendarSameDayArrival(ctx context.Context, propertyID, checkoutOccupancyID int64, dayStartUTC, dayEndUTC time.Time) (*Occupancy, error) {
	q := occupancySelectColumns + `
		FROM occupancies
		WHERE property_id = ?
		  AND id <> ?
		  AND status IN ('active', 'updated')
		  AND superseded_at IS NULL
		  AND (closure_state IS NULL OR closure_state <> 'closed')
		  AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		  AND start_at >= ?
		  AND start_at < ?
		ORDER BY start_at ASC, id ASC
		LIMIT 1`
	rows, err := s.scanOccupancies(ctx, q, propertyID, checkoutOccupancyID, dayStartUTC.UTC().Format(time.RFC3339), dayEndUTC.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) UpsertCleaningCalendarEvent(ctx context.Context, event *CleaningCalendarEvent) (*CleaningCalendarEvent, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	sameDay := 0
	if event.SameDayArrival {
		sameDay = 1
	}
	var next interface{}
	if event.NextOccupancyID.Valid {
		next = event.NextOccupancyID.Int64
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
	kind := event.CleaningKind
	if kind == "" {
		kind = CleaningKindNamedStay
	}
	// occupancy_id is no longer unique (PMS_19 §5.4 provisional per-night), so
	// this occupancy-keyed helper does a manual update-or-insert.
	res, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_events SET
			upstream_event_uid = ?, checkout_date = ?, cleaning_kind = ?,
			google_calendar_id = ?, google_event_id = COALESCE(?, google_event_id),
			cleaning_date = ?, starts_at = ?, ends_at = ?, same_day_arrival = ?,
			next_occupancy_id = ?, title = ?, status = ?, warning_message = ?, error_message = ?,
			last_synced_at = COALESCE(?, last_synced_at), updated_at = ?
		WHERE property_id = ? AND occupancy_id = ?`,
		nullStr(event.UpstreamEventUID), nullStr(event.CheckoutDate), kind,
		event.GoogleCalendarID, googleEventID,
		event.CleaningDate, event.StartsAt.UTC().Format(time.RFC3339), event.EndsAt.UTC().Format(time.RFC3339), sameDay,
		next, event.Title, event.Status, warning, errMsg, lastSynced, now,
		event.PropertyID, event.OccupancyID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.DB.ExecContext(ctx, `
			INSERT INTO cleaning_calendar_events (
				property_id, occupancy_id, upstream_event_uid, checkout_date, cleaning_kind,
				google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
				same_day_arrival, next_occupancy_id, title, status, warning_message, error_message, last_synced_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			event.PropertyID, event.OccupancyID, nullStr(event.UpstreamEventUID), nullStr(event.CheckoutDate), kind,
			event.GoogleCalendarID, googleEventID, event.CleaningDate,
			event.StartsAt.UTC().Format(time.RFC3339), event.EndsAt.UTC().Format(time.RFC3339), sameDay, next, event.Title,
			event.Status, warning, errMsg, lastSynced, now, now); err != nil {
			return nil, err
		}
	}
	return s.GetCleaningCalendarEventByOccupancy(ctx, event.PropertyID, event.OccupancyID)
}

func (s *Store) GetCleaningCalendarEventByOccupancy(ctx context.Context, propertyID, occupancyID int64) (*CleaningCalendarEvent, error) {
	rows, err := s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND occupancy_id = ?
		ORDER BY checkout_date ASC, id ASC`, propertyID, occupancyID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

// GetCleaningCalendarEventByIdentity looks up an event by the PMS_19 §5.4
// identity key so per-night provisional events stay idempotent.
func (s *Store) GetCleaningCalendarEventByIdentity(ctx context.Context, propertyID int64, upstreamUID, checkoutDate, cleaningKind string) (*CleaningCalendarEvent, error) {
	rows, err := s.scanCleaningCalendarEvents(ctx, `
		SELECT `+cleaningCalendarColumns+`
		FROM cleaning_calendar_events
		WHERE property_id = ? AND upstream_event_uid = ? AND checkout_date = ? AND cleaning_kind = ?`,
		propertyID, upstreamUID, checkoutDate, cleaningKind)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

// UpsertCleaningCalendarEventByIdentity upserts on the identity key so a block
// can own several provisional per-night events without collisions (§5.4).
func (s *Store) UpsertCleaningCalendarEventByIdentity(ctx context.Context, event *CleaningCalendarEvent) (*CleaningCalendarEvent, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	sameDay := 0
	if event.SameDayArrival {
		sameDay = 1
	}
	var next interface{}
	if event.NextOccupancyID.Valid {
		next = event.NextOccupancyID.Int64
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
	kind := event.CleaningKind
	if kind == "" {
		kind = CleaningKindNamedStay
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO cleaning_calendar_events (
			property_id, occupancy_id, upstream_event_uid, checkout_date, cleaning_kind,
			google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
			same_day_arrival, next_occupancy_id, title, status, warning_message, error_message, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, upstream_event_uid, checkout_date, cleaning_kind)
			WHERE upstream_event_uid IS NOT NULL AND checkout_date IS NOT NULL DO UPDATE SET
			occupancy_id = excluded.occupancy_id,
			google_calendar_id = excluded.google_calendar_id,
			google_event_id = COALESCE(excluded.google_event_id, cleaning_calendar_events.google_event_id),
			cleaning_date = excluded.cleaning_date,
			starts_at = excluded.starts_at,
			ends_at = excluded.ends_at,
			same_day_arrival = excluded.same_day_arrival,
			next_occupancy_id = excluded.next_occupancy_id,
			title = excluded.title,
			status = excluded.status,
			warning_message = excluded.warning_message,
			error_message = excluded.error_message,
			last_synced_at = COALESCE(excluded.last_synced_at, cleaning_calendar_events.last_synced_at),
			updated_at = excluded.updated_at`,
		event.PropertyID, event.OccupancyID, nullStr(event.UpstreamEventUID), nullStr(event.CheckoutDate), kind,
		event.GoogleCalendarID, googleEventID, event.CleaningDate,
		event.StartsAt.UTC().Format(time.RFC3339), event.EndsAt.UTC().Format(time.RFC3339), sameDay, next, event.Title,
		event.Status, warning, errMsg, lastSynced, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetCleaningCalendarEventByIdentity(ctx, event.PropertyID, nullOrString(event.UpstreamEventUID), nullOrString(event.CheckoutDate), kind)
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

func (s *Store) MarkCleaningCalendarEventRemoved(ctx context.Context, propertyID, eventID int64, errMsg *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE cleaning_calendar_events
		SET status = 'removed', error_message = ?, updated_at = ?
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
		SET google_event_id = COALESCE(?, google_event_id), status = ?, error_message = ?, last_synced_at = COALESCE(?, last_synced_at), updated_at = ?
		WHERE property_id = ? AND id = ?`, gid, status, nullableString(ptrStringValue(errMsg)), synced, now.Format(time.RFC3339), propertyID, eventID)
	return err
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
		var lastSynced sql.NullString
		if err := rows.Scan(&row.ID, &row.PropertyID, &row.OccupancyID, &row.UpstreamEventUID, &row.CheckoutDate, &row.CleaningKind,
			&row.GoogleCalendarID, &row.GoogleEventID, &row.CleaningDate, &starts, &ends,
			&sameDay, &row.NextOccupancyID, &row.Title, &row.Status, &row.WarningMessage, &row.ErrorMessage, &lastSynced, &created, &updated); err != nil {
			return nil, err
		}
		row.StartsAt, _ = time.Parse(time.RFC3339, starts)
		row.EndsAt, _ = time.Parse(time.RFC3339, ends)
		row.SameDayArrival = sameDay == 1
		if lastSynced.Valid && lastSynced.String != "" {
			t, _ := time.Parse(time.RFC3339, lastSynced.String)
			row.LastSyncedAt = sql.NullTime{Time: t, Valid: true}
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
