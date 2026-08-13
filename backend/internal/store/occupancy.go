package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type OccupancySource struct {
	ID         int64
	PropertyID int64
	SourceType string
	Active     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type OccupancySyncRun struct {
	ID                               int64
	PropertyID                       int64
	StartedAt                        time.Time
	FinishedAt                       sql.NullTime
	Status                           string
	ErrorMessage                     sql.NullString
	EventsSeen                       int
	OccupanciesUpserted              int
	HTTPStatus                       sql.NullInt64
	Trigger                          string
	CreatedAt                        time.Time
	UpstreamEventsParsed             int
	ParseErrors                      int
	RepresentationsInserted          int
	RepresentationsUpdated           int
	RepresentationsUnchanged         int
	RepresentationsSuperseded        int
	RepresentationsDeletedFromSource int
	DuplicateNightsResolved          int
	LegacyGeneratedRowsConverted     int
	NamedStaysDeletedFromSource      int
	ProvisionalCleaningEventsCreated int
	ProvisionalCleaningEventsRemoved int
	DeletionEnabled                  bool
	RawBlocksInserted                int
	RawBlocksUpdated                 int
	RawBlocksUnchanged               int
	RawBlocksDeletedFromSource       int
	RawBlockConflicts                int
}

type SyncCounters struct {
	UpstreamEventsSeen               int
	UpstreamEventsParsed             int
	ParseErrors                      int
	RepresentationsInserted          int
	RepresentationsUpdated           int
	RepresentationsUnchanged         int
	RepresentationsSuperseded        int
	RepresentationsDeletedFromSource int
	DuplicateNightsResolved          int
	LegacyGeneratedRowsConverted     int
	NamedStaysDeletedFromSource      int
	ProvisionalCleaningEventsCreated int
	ProvisionalCleaningEventsRemoved int
	DeletionEnabled                  bool
	SyncRunID                        int64
	RawBlocksInserted                int
	RawBlocksUpdated                 int
	RawBlocksUnchanged               int
	RawBlocksDeletedFromSource       int
	RawBlockConflicts                int
}

// The column remains in the historical schema but no longer represents sync work.
const legacyOccupanciesUpserted = 0

// These values remain shared by current named-stay/raw-block behavior and the
// separately retained PMS 21 migration implementation.
const (
	ClosureStateClosed                 = "closed"
	ClosureStateExternalSale           = "external_sale"
	StayOutcomeCancelledNonRefundable  = "cancelled_non_refundable"
	StayOutcomeNoShow                  = "no_show"
	RepresentationUnnamedBlock         = "unnamed_block"
	RepresentationNamedStay            = "named_stay"
	RepresentationManualClosure        = "manual_closure"
	RepresentationExternalSale         = "external_sale"
	RepresentationSyntheticFinance     = "synthetic_finance"
	RepresentationLegacyGeneratedNight = "legacy_generated_night"
	UpstreamSourceBookingICS           = "booking_ics"
	StatusDeletedFromSource            = "deleted_from_source"
	manualSplitSourceType              = "manual"
)

func (s *Store) InsertOccupancySourceTx(ctx context.Context, tx *sql.Tx, propertyID int64, now string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO occupancy_sources (property_id, source_type, active, created_at, updated_at)
		VALUES (?, 'booking_ics', 1, ?, ?)
		ON CONFLICT(property_id) DO NOTHING`, propertyID, now, now)
	return err
}

func (s *Store) GetOccupancySource(ctx context.Context, propertyID int64) (*OccupancySource, error) {
	var source OccupancySource
	var active int
	var createdAt, updatedAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, source_type, active, created_at, updated_at
		FROM occupancy_sources WHERE property_id = ?`, propertyID).
		Scan(&source.ID, &source.PropertyID, &source.SourceType, &active, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	source.Active = active == 1
	source.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	source.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &source, nil
}

func (s *Store) UpdateOccupancySource(ctx context.Context, propertyID int64, active *bool, sourceType *string) error {
	source, err := s.GetOccupancySource(ctx, propertyID)
	if err != nil {
		return err
	}
	if active != nil {
		source.Active = *active
	}
	if sourceType != nil {
		source.SourceType = *sourceType
	}
	activeValue := 0
	if source.Active {
		activeValue = 1
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE occupancy_sources SET source_type = ?, active = ?, updated_at = ? WHERE property_id = ?`,
		source.SourceType, activeValue, time.Now().UTC().Format(time.RFC3339), propertyID)
	return err
}

func (s *Store) StartOccupancySyncRun(ctx context.Context, propertyID int64, trigger string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancy_sync_runs (property_id, started_at, status, trigger, created_at)
		VALUES (?, ?, 'running', ?, ?)`, propertyID, now, trigger, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishOccupancySyncRun(ctx context.Context, runID int64, status string, errMsg *string, httpStatus *int, eventsSeen, _ int) error {
	var errorMessage any
	if errMsg != nil {
		errorMessage = *errMsg
	}
	var statusCode any
	if httpStatus != nil {
		statusCode = *httpStatus
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE occupancy_sync_runs SET finished_at = ?, status = ?, error_message = ?, http_status = ?, events_seen = ?, occupancies_upserted = ?
		WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), status, errorMessage, statusCode, eventsSeen, legacyOccupanciesUpserted, runID)
	return err
}

func (s *Store) InsertOccupancyRawEvent(ctx context.Context, propertyID, syncRunID int64, uid, raw, summary, startRFC, endRFC string, seq int, icalStatus, hash string) error {
	return s.InsertOccupancyRawEventDetailed(ctx, propertyID, syncRunID, UpstreamSourceBookingICS, uid, raw, summary, startRFC, endRFC, seq, icalStatus, "", hash)
}

func (s *Store) InsertOccupancyRawEventDetailed(ctx context.Context, propertyID, syncRunID int64, sourceType, uid, raw, summary, startRFC, endRFC string, seq int, icalStatus, dtstamp, hash string) error {
	if sourceType == "" {
		sourceType = UpstreamSourceBookingICS
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancy_raw_events (property_id, sync_run_id, source_type, source_event_uid, raw_component, summary, event_start, event_end, sequence_num, ical_status, dtstamp, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, sync_run_id, source_type, source_event_uid) DO UPDATE SET
			raw_component = excluded.raw_component,
			summary = excluded.summary,
			event_start = excluded.event_start,
			event_end = excluded.event_end,
			sequence_num = excluded.sequence_num,
			ical_status = excluded.ical_status,
			dtstamp = excluded.dtstamp,
			content_hash = excluded.content_hash`,
		propertyID, syncRunID, sourceType, uid, raw, summary, startRFC, endRFC, seq, icalStatus, nullableString(dtstamp), hash, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListOccupancySyncRuns(ctx context.Context, propertyID int64, limit int) ([]OccupancySyncRun, error) {
	return s.ListOccupancySyncRunsPaged(ctx, propertyID, limit, 0)
}

func (s *Store) ListOccupancySyncRunsPaged(ctx context.Context, propertyID int64, limit, offset int) ([]OccupancySyncRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, started_at, finished_at, status, error_message, events_seen, occupancies_upserted, http_status, trigger, created_at,
		       upstream_events_parsed, parse_errors, representations_inserted, representations_updated, representations_unchanged,
		       representations_superseded, representations_deleted_from_source, duplicate_nights_resolved, legacy_generated_rows_converted,
		       named_stays_deleted_from_source, provisional_cleaning_events_created, provisional_cleaning_events_removed, deletion_enabled,
		       raw_blocks_inserted, raw_blocks_updated, raw_blocks_unchanged, raw_blocks_deleted_from_source, raw_block_conflicts
		FROM occupancy_sync_runs WHERE property_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`, propertyID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancySyncRun
	for rows.Next() {
		var run OccupancySyncRun
		var startedAt, createdAt string
		var finishedAt sql.NullString
		var deletionEnabled int
		if err := rows.Scan(&run.ID, &run.PropertyID, &startedAt, &finishedAt, &run.Status, &run.ErrorMessage, &run.EventsSeen, &run.OccupanciesUpserted, &run.HTTPStatus, &run.Trigger, &createdAt,
			&run.UpstreamEventsParsed, &run.ParseErrors, &run.RepresentationsInserted, &run.RepresentationsUpdated, &run.RepresentationsUnchanged,
			&run.RepresentationsSuperseded, &run.RepresentationsDeletedFromSource, &run.DuplicateNightsResolved, &run.LegacyGeneratedRowsConverted,
			&run.NamedStaysDeletedFromSource, &run.ProvisionalCleaningEventsCreated, &run.ProvisionalCleaningEventsRemoved, &deletionEnabled,
			&run.RawBlocksInserted, &run.RawBlocksUpdated, &run.RawBlocksUnchanged, &run.RawBlocksDeletedFromSource, &run.RawBlockConflicts); err != nil {
			return nil, err
		}
		run.DeletionEnabled = deletionEnabled == 1
		run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		run.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if finishedAt.Valid && finishedAt.String != "" {
			value, _ := time.Parse(time.RFC3339, finishedAt.String)
			run.FinishedAt = sql.NullTime{Time: value, Valid: true}
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) ListPropertyIDsWithICSURL(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id FROM properties p
		INNER JOIN property_secrets ps ON ps.property_id = p.id
		INNER JOIN occupancy_sources os ON os.property_id = p.id
		WHERE p.active = 1 AND os.active = 1 AND ps.booking_ics_url IS NOT NULL AND TRIM(ps.booking_ics_url) != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func parseOccupancySplitDate(value string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, ErrNamedStayInvalidRange
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC), nil
}

func toUTCMidnight(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullNullableString(value sql.NullString) any {
	if value.Valid && strings.TrimSpace(value.String) != "" {
		return value.String
	}
	return nil
}

func ptrStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
