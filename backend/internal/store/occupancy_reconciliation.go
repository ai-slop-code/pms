package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrNamedStayOutsideBlock = errors.New("stay range is outside the latest Booking.com block")
	ErrNamedStayOverlap      = errors.New("stay range overlaps another active stay or closure")
	ErrNamedStayInvalidRange = errors.New("invalid stay range")
	ErrUpstreamBlockNotFound = errors.New("upstream booking block not found")
)

func (s *Store) propertyLocationTx(ctx context.Context, tx *sql.Tx, propertyID int64) *time.Location {
	var tz string
	if err := tx.QueryRowContext(ctx, `SELECT timezone FROM properties WHERE id = ?`, propertyID).Scan(&tz); err != nil {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// FinishOccupancySyncRunDetailed records the full PMS_19 §12 counter set.
func (s *Store) FinishOccupancySyncRunDetailed(ctx context.Context, runID int64, status string, errMsg *string, httpStatus *int, c *SyncCounters) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var em interface{}
	if errMsg != nil {
		em = *errMsg
	}
	var hs interface{}
	if httpStatus != nil {
		hs = *httpStatus
	}
	del := 0
	if c.DeletionEnabled {
		del = 1
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE occupancy_sync_runs SET finished_at = ?, status = ?, error_message = ?, http_status = ?,
			events_seen = ?, occupancies_upserted = ?,
			upstream_events_parsed = ?, parse_errors = ?, representations_inserted = ?, representations_updated = ?,
			representations_unchanged = ?, representations_superseded = ?, representations_deleted_from_source = ?,
			duplicate_nights_resolved = ?, legacy_generated_rows_converted = ?, named_stays_deleted_from_source = ?,
			provisional_cleaning_events_created = ?, provisional_cleaning_events_removed = ?, deletion_enabled = ?,
			raw_blocks_inserted = ?, raw_blocks_updated = ?, raw_blocks_unchanged = ?,
			raw_blocks_deleted_from_source = ?, raw_block_conflicts = ?
		WHERE id = ?`,
		now, status, em, hs, c.UpstreamEventsSeen, legacyOccupanciesUpserted,
		c.UpstreamEventsParsed, c.ParseErrors, c.RepresentationsInserted, c.RepresentationsUpdated,
		c.RepresentationsUnchanged, c.RepresentationsSuperseded, c.RepresentationsDeletedFromSource,
		c.DuplicateNightsResolved, c.LegacyGeneratedRowsConverted, c.NamedStaysDeletedFromSource,
		c.ProvisionalCleaningEventsCreated, c.ProvisionalCleaningEventsRemoved, del,
		c.RawBlocksInserted, c.RawBlocksUpdated, c.RawBlocksUnchanged, c.RawBlocksDeletedFromSource, c.RawBlockConflicts, runID)
	return err
}

// DesiredBlock is a parsed upstream Booking.com event handed to the store for
// reconciliation (decoupled from the occupancy package to avoid an import cycle).
type DesiredBlock struct {
	UID         string
	Start       time.Time
	End         time.Time
	Summary     string
	ContentHash string
	Cancelled   bool
	DTStamp     time.Time
}

// ReconcileBookingICSSync applies a full successful feed to the canonical raw
// booking-block tables. It deliberately does not consult or mutate legacy
// occupancies or occupancy_nights.
func (s *Store) ReconcileBookingICSSync(ctx context.Context, propertyID int64, sourceType string, blocks []DesiredBlock, now time.Time, counters *SyncCounters) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	seen := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		seen[b.UID] = true
	}
	loc := s.propertyLocationTx(ctx, tx, propertyID)
	if err := s.markRawBookingBlocksDisappearedTx(ctx, tx, propertyID, sourceType, seen, now, loc, counters); err != nil {
		return err
	}

	for _, b := range blocks {
		if err := s.upsertRawBookingBlockTx(ctx, tx, propertyID, sourceType, b, now, counters); err != nil {
			return err
		}
	}
	newConflicts, err := recomputeSourceLinkHealthTx(ctx, tx, propertyID, now.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	if counters != nil {
		counters.RawBlockConflicts += newConflicts
	}

	return tx.Commit()
}

func (s *Store) upsertRawBookingBlockTx(ctx context.Context, tx *sql.Tx, propertyID int64, sourceType string, b DesiredBlock, now time.Time, counters *SyncCounters) error {
	if sourceType == "" {
		sourceType = UpstreamSourceBookingICS
	}
	nowStr := now.UTC().Format(time.RFC3339)
	checkIn := toUTCMidnight(b.Start).Format("2006-01-02")
	checkOut := toUTCMidnight(b.End).Format("2006-01-02")
	status := "active"
	if b.Cancelled {
		status = StatusDeletedFromSource
	}
	contentHash := b.ContentHash
	if contentHash == "" {
		contentHash = b.UID + ":" + checkIn + ":" + checkOut + ":" + b.Summary
	}
	var summary interface{}
	if strings.TrimSpace(b.Summary) != "" {
		summary = b.Summary
	}
	var dtstamp interface{}
	if !b.DTStamp.IsZero() {
		dtstamp = b.DTStamp.UTC().Format(time.RFC3339)
	}
	syncRunID := nullableSyncRunID(counters)

	type existingRawBlock struct {
		id          int64
		checkIn     string
		checkOut    string
		status      string
		contentHash string
	}
	var existing existingRawBlock
	err := tx.QueryRowContext(ctx, `
		SELECT id, check_in_date, check_out_date, status, content_hash
		FROM raw_booking_blocks
		WHERE property_id = ? AND source_type = ? AND source_event_uid = ?`, propertyID, sourceType, b.UID).
		Scan(&existing.id, &existing.checkIn, &existing.checkOut, &existing.status, &existing.contentHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	inserted := errors.Is(err, sql.ErrNoRows)

	deletedAt := interface{}(nil)
	if status == StatusDeletedFromSource {
		deletedAt = nowStr
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO raw_booking_blocks (
			property_id, source_type, source_event_uid, check_in_date, check_out_date, status,
			raw_summary, content_hash, source_dtstamp, first_seen_sync_run_id, last_sync_run_id,
			imported_at, last_synced_at, deleted_from_source_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_type, source_event_uid) DO UPDATE SET
			check_in_date = excluded.check_in_date,
			check_out_date = excluded.check_out_date,
			status = excluded.status,
			raw_summary = excluded.raw_summary,
			content_hash = excluded.content_hash,
			source_dtstamp = COALESCE(excluded.source_dtstamp, raw_booking_blocks.source_dtstamp),
			first_seen_sync_run_id = COALESCE(raw_booking_blocks.first_seen_sync_run_id, excluded.first_seen_sync_run_id),
			last_sync_run_id = excluded.last_sync_run_id,
			last_synced_at = excluded.last_synced_at,
			deleted_from_source_at = excluded.deleted_from_source_at,
			conflict_reason = NULL,
			updated_at = excluded.updated_at`,
		propertyID, sourceType, b.UID, checkIn, checkOut, status,
		summary, contentHash, dtstamp, syncRunID, syncRunID,
		nowStr, nowStr, deletedAt, nowStr, nowStr)
	if err != nil {
		return err
	}
	blockID := existing.id
	if inserted {
		blockID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		if counters != nil {
			counters.RawBlocksInserted++
		}
	} else if counters != nil {
		if existing.checkIn != checkIn || existing.checkOut != checkOut || existing.status != status || existing.contentHash != contentHash {
			counters.RawBlocksUpdated++
		} else {
			counters.RawBlocksUnchanged++
		}
	}

	var activeNights []string
	if status == "active" {
		activeNights = nightsUTC(b.Start, b.End)
	}
	return replaceRawBookingBlockNightsTx(ctx, tx, propertyID, blockID, activeNights, nowStr)
}

func (s *Store) markRawBookingBlocksDisappearedTx(ctx context.Context, tx *sql.Tx, propertyID int64, sourceType string, seen map[string]bool, now time.Time, loc *time.Location, counters *SyncCounters) error {
	if sourceType == "" {
		sourceType = UpstreamSourceBookingICS
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, source_event_uid, check_out_date
		FROM raw_booking_blocks
		WHERE property_id = ? AND source_type = ? AND status = 'active'`, propertyID, sourceType)
	if err != nil {
		return err
	}
	defer rows.Close()
	today := now.In(loc).Format("2006-01-02")
	nowStr := now.UTC().Format(time.RFC3339)
	syncRunID := nullableSyncRunID(counters)
	type rawGone struct {
		id       int64
		uid      string
		checkout string
	}
	var gone []rawGone
	for rows.Next() {
		var r rawGone
		if err := rows.Scan(&r.id, &r.uid, &r.checkout); err != nil {
			return err
		}
		if !seen[r.uid] && r.checkout >= today {
			gone = append(gone, r)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, r := range gone {
		if _, err := tx.ExecContext(ctx, `
			UPDATE raw_booking_blocks
			SET status = 'deleted_from_source', last_sync_run_id = ?, last_synced_at = ?, deleted_from_source_at = COALESCE(deleted_from_source_at, ?), updated_at = ?
			WHERE id = ?`, syncRunID, nowStr, nowStr, nowStr, r.id); err != nil {
			return err
		}
		if err := replaceRawBookingBlockNightsTx(ctx, tx, propertyID, r.id, nil, nowStr); err != nil {
			return err
		}
		if counters != nil {
			counters.RawBlocksDeletedFromSource++
		}
	}
	return nil
}

func replaceRawBookingBlockNightsTx(ctx context.Context, tx *sql.Tx, propertyID, rawBlockID int64, activeNights []string, nowStr string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE raw_booking_block_nights SET active = 0, updated_at = ? WHERE raw_booking_block_id = ?`, nowStr, rawBlockID); err != nil {
		return err
	}
	for _, night := range activeNights {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO raw_booking_block_nights (property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
			VALUES (?, ?, ?, 1, ?, ?)
			ON CONFLICT(property_id, raw_booking_block_id, local_night_date) DO UPDATE SET
				active = 1,
				updated_at = excluded.updated_at`, propertyID, rawBlockID, night, nowStr, nowStr); err != nil {
			return err
		}
	}
	return nil
}

func nullableSyncRunID(c *SyncCounters) interface{} {
	if c != nil && c.SyncRunID > 0 {
		return c.SyncRunID
	}
	return nil
}
