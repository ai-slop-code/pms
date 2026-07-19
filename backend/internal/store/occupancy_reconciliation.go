package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PMS_19 upstream ownership + representation reconciliation. This file owns the
// per-night desired-representation build (§7.3), named-stay reconciliation
// (§7.4), disappearance handling (§7.5), and deterministic duplicate
// resolution (§7.6). occupancy_nights is the night-level source of truth.

// DeriveUpstreamUID maps a legacy/synthetic source_event_uid back to the
// Booking.com upstream UID that owns it (§11 repair).
//
//	manual_split:UID:role:YYYYMMDD -> UID
//	UID#night-YYYYMMDD             -> UID
//	UID                            -> UID
func DeriveUpstreamUID(sourceEventUID string) string {
	uid := strings.TrimSpace(sourceEventUID)
	if strings.HasPrefix(uid, manualSplitUIDPrefix) {
		rest := strings.TrimPrefix(uid, manualSplitUIDPrefix)
		parts := strings.Split(rest, ":")
		if len(parts) >= 3 {
			// last two tokens are role + date; the rest is the UID.
			return strings.Join(parts[:len(parts)-2], ":")
		}
		return rest
	}
	if i := strings.Index(uid, "#night-"); i >= 0 {
		return uid[:i]
	}
	return uid
}

// deriveRepresentationKind classifies an existing row for backfill (§6.1).
func deriveRepresentationKind(o *Occupancy) string {
	if o.ClosureState.Valid {
		switch o.ClosureState.String {
		case ClosureStateClosed:
			return RepresentationManualClosure
		case ClosureStateExternalSale:
			return RepresentationExternalSale
		}
	}
	if strings.Contains(o.SourceEventUID, "#night-") {
		if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
			return RepresentationNamedStay
		}
		return RepresentationLegacyGeneratedNight
	}
	if strings.HasPrefix(o.SourceEventUID, manualSplitUIDPrefix) {
		if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
			return RepresentationNamedStay
		}
		return RepresentationUnnamedBlock
	}
	if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
		return RepresentationNamedStay
	}
	return RepresentationUnnamedBlock
}

// ListOccupanciesForUpstreamUID returns every representation row PMS derived
// from one upstream UID across booking_ics, legacy generated, named-stay, and
// manual-split source types (§14 step 4).
func (s *Store) ListOccupanciesForUpstreamUID(ctx context.Context, propertyID int64, upstreamUID string) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND upstream_event_uid = ? ORDER BY id ASC`
	return s.scanOccupancies(ctx, q, propertyID, upstreamUID)
}

func isActiveStatus(status string) bool {
	return status == "active" || status == "updated"
}

func isOperatorRow(o *Occupancy) bool {
	if o.ClosureState.Valid {
		return true
	}
	if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
		return true
	}
	switch o.RepresentationKind.String {
	case RepresentationNamedStay, RepresentationManualClosure, RepresentationExternalSale, RepresentationLegacyGeneratedNight:
		return true
	}
	// manual split children (before/after/night) are separate rows.
	return o.SourceType == manualSplitSourceType
}

// representationPriority implements §7.6 ordering (lower = wins).
func representationPriority(o *Occupancy) int {
	switch {
	case o.RepresentationKind.String == RepresentationNamedStay:
		return 1 // operator-selected named stay
	case o.ClosureState.Valid:
		return 2 // manual closure / external sale
	case o.RepresentationKind.String == RepresentationLegacyGeneratedNight:
		return 4 // legacy generated night row
	case o.SourceType == UpstreamSourceBookingICS && o.RepresentationKind.String == RepresentationUnnamedBlock:
		return 3 // unnamed block from a current feed
	case o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "":
		return 1 // named (representation not yet classified) still ranks as a named stay
	default:
		return 5 // older aggregate / source row
	}
}

// checkoutTodayOrFuture reports whether the row's checkout is today or later in
// UTC (routine sync deletion only touches current/future rows, §7.5).
func checkoutTodayOrFuture(o *Occupancy, now time.Time) bool {
	return checkoutTodayOrFutureInLocation(o, now, time.UTC)
}

func checkoutTodayOrFutureInLocation(o *Occupancy, now time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	checkout := toUTCMidnight(o.EndAt).Format("2006-01-02")
	today := now.In(loc).Format("2006-01-02")
	return checkout >= today
}

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

// reconcileUpstreamCoverageTx rebuilds per-night coverage for one upstream UID.
// sourceNights is the currently-blocked night set from the latest successful
// feed (empty means the UID disappeared). It enforces:
//   - named stays / closures win their nights,
//   - the aggregate unnamed_block fills only unclaimed source nights,
//   - out-of-range today/future named rows become deleted_from_source (§7.4).
func (s *Store) reconcileUpstreamCoverageTx(ctx context.Context, tx *sql.Tx, propertyID int64, upstreamUID string, sourceNights []string, now time.Time, loc *time.Location, counters *SyncCounters) error {
	rows, err := s.listUpstreamRowsTx(ctx, tx, propertyID, upstreamUID)
	if err != nil {
		return err
	}
	nowStr := now.UTC().Format(time.RFC3339)
	sourceSet := make(map[string]bool, len(sourceNights))
	for _, n := range sourceNights {
		sourceSet[n] = true
	}

	// Partition rows: the aggregate (source_event_uid == UID) is the filler
	// unless the operator labelled it directly.
	var aggregate *Occupancy
	var operators []*Occupancy
	for i := range rows {
		r := &rows[i]
		if !isActiveStatus(r.Status) || r.SupersededAt.Valid {
			continue
		}
		if r.SourceEventUID == upstreamUID {
			aggregate = r
		}
	}
	aggIsFiller := aggregate != nil && !aggregate.ClosureState.Valid &&
		!(aggregate.GuestDisplayName.Valid && strings.TrimSpace(aggregate.GuestDisplayName.String) != "") &&
		aggregate.RepresentationKind.String != RepresentationNamedStay
	for i := range rows {
		r := &rows[i]
		if !isActiveStatus(r.Status) || r.SupersededAt.Valid {
			continue
		}
		if aggIsFiller && r.ID == aggregate.ID {
			continue
		}
		if isOperatorRow(r) || (aggregate != nil && r.ID == aggregate.ID) {
			operators = append(operators, r)
		}
	}
	sort.SliceStable(operators, func(a, b int) bool {
		pa, pb := representationPriority(operators[a]), representationPriority(operators[b])
		if pa != pb {
			return pa < pb
		}
		if c := compareDtstamp(operators[a], operators[b]); c != 0 {
			return c > 0
		}
		if !operators[a].LastSyncedAt.Equal(operators[b].LastSyncedAt) {
			return operators[a].LastSyncedAt.After(operators[b].LastSyncedAt)
		}
		return operators[a].ID < operators[b].ID
	})

	// Deactivate everything first so re-activation is conflict-free.
	for i := range rows {
		if err := deactivateOccupancyNightsTx(ctx, tx, rows[i].ID); err != nil {
			return err
		}
	}

	claimed := make(map[string]bool)
	for _, r := range operators {
		var active []string
		for _, n := range nightsUTC(r.StartAt, r.EndAt) {
			if sourceSet[n] && !claimed[n] {
				active = append(active, n)
				claimed[n] = true
			}
		}
		if len(active) == 0 {
			// Out-of-range / fully superseded. Today/future named rows are
			// deleted_from_source; past rows stay for audit.
			if checkoutTodayOrFutureInLocation(r, now, loc) {
				if err := s.markRowDeletedFromSourceTx(ctx, tx, r, nowStr, SupersededSourceDeleted); err != nil {
					return err
				}
				if counters != nil {
					counters.RepresentationsDeletedFromSource++
					counters.RepresentationsSuperseded++
					if r.RepresentationKind.String == RepresentationNamedStay {
						counters.NamedStaysDeletedFromSource++
					}
					if r.RepresentationKind.String == RepresentationLegacyGeneratedNight {
						counters.LegacyGeneratedRowsConverted++
					}
				}
			}
			continue
		}
		if err := setOccupancyNightsTx(ctx, tx, propertyID, r.ID, active, nullOrString(r.UpstreamSourceType), upstreamUID, true); err != nil {
			return err
		}
	}

	if aggIsFiller {
		var fill []string
		for _, n := range sourceNights {
			if !claimed[n] {
				fill = append(fill, n)
			}
		}
		if err := setOccupancyNightsTx(ctx, tx, propertyID, aggregate.ID, fill, nullOrString(aggregate.UpstreamSourceType), upstreamUID, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) listUpstreamRowsTx(ctx context.Context, tx *sql.Tx, propertyID int64, upstreamUID string) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND upstream_event_uid = ? ORDER BY id ASC`
	rows, err := tx.QueryContext(ctx, q, propertyID, upstreamUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOccupancyRows(rows)
}

func (s *Store) markRowDeletedFromSourceTx(ctx context.Context, tx *sql.Tx, o *Occupancy, nowStr, reason string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE occupancies SET status = 'deleted_from_source', superseded_at = COALESCE(superseded_at, ?), superseded_reason = COALESCE(superseded_reason, ?), last_synced_at = ?
		WHERE id = ?`, nowStr, reason, nowStr, o.ID); err != nil {
		return err
	}
	return deactivateOccupancyNightsTx(ctx, tx, o.ID)
}

func nullOrString(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

// MarkUpstreamDisappearedTx marks every current/future representation owned by
// a disappeared UID as deleted_from_source (§7.5).
func (s *Store) markUpstreamDisappearedTx(ctx context.Context, tx *sql.Tx, propertyID int64, upstreamUID string, now time.Time, loc *time.Location, counters *SyncCounters) error {
	rows, err := s.listUpstreamRowsTx(ctx, tx, propertyID, upstreamUID)
	if err != nil {
		return err
	}
	nowStr := now.UTC().Format(time.RFC3339)
	for i := range rows {
		r := &rows[i]
		if !isActiveStatus(r.Status) {
			continue
		}
		if !checkoutTodayOrFutureInLocation(r, now, loc) {
			// Past row: leave for audit but ensure its nights are inactive.
			if err := deactivateOccupancyNightsTx(ctx, tx, r.ID); err != nil {
				return err
			}
			continue
		}
		if err := s.markRowDeletedFromSourceTx(ctx, tx, r, nowStr, SupersededSourceDeleted); err != nil {
			return err
		}
		if counters != nil {
			counters.RepresentationsDeletedFromSource++
			counters.RepresentationsSuperseded++
			if r.RepresentationKind.String == RepresentationNamedStay {
				counters.NamedStaysDeletedFromSource++
			}
			if r.RepresentationKind.String == RepresentationLegacyGeneratedNight {
				counters.LegacyGeneratedRowsConverted++
			}
		}
	}
	return nil
}

// ---- Named stay flow (§5.1 / §7.4 / §11A) ----

var (
	ErrNamedStayOutsideBlock = errors.New("stay range is outside the latest Booking.com block")
	ErrNamedStayOverlap      = errors.New("stay range overlaps another active stay or closure")
	ErrNamedStayInvalidRange = errors.New("invalid stay range")
	ErrUpstreamBlockNotFound = errors.New("upstream booking block not found")
)

// aggregateForUpstreamTx returns the active aggregate booking_ics row for a UID.
func (s *Store) aggregateForUpstreamTx(ctx context.Context, tx *sql.Tx, propertyID int64, upstreamUID string) (*Occupancy, error) {
	rows, err := s.listUpstreamRowsTx(ctx, tx, propertyID, upstreamUID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		r := &rows[i]
		if r.SourceEventUID == upstreamUID && isActiveStatus(r.Status) {
			return r, nil
		}
	}
	return nil, ErrUpstreamBlockNotFound
}

// CreateNamedStay materialises an operator-selected guest stay inside a block.
func (s *Store) CreateNamedStay(ctx context.Context, propertyID int64, upstreamUID, checkIn, checkOut, guestName string, userID int64) (int64, error) {
	ci, err := parseOccupancySplitDate(checkIn)
	if err != nil {
		return 0, ErrNamedStayInvalidRange
	}
	co, err := parseOccupancySplitDate(checkOut)
	if err != nil {
		return 0, ErrNamedStayInvalidRange
	}
	if !co.After(ci) {
		return 0, ErrNamedStayInvalidRange
	}
	guestName = strings.TrimSpace(guestName)
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	agg, err := s.aggregateForUpstreamTx(ctx, tx, propertyID, upstreamUID)
	if err != nil {
		return 0, err
	}
	blockStart := toUTCMidnight(agg.StartAt)
	blockEnd := toUTCMidnight(agg.EndAt)
	if ci.Before(blockStart) || co.After(blockEnd) {
		return 0, ErrNamedStayOutsideBlock
	}
	nights := nightsUTC(ci, co)
	// Overlap check against other active operator rows for this UID.
	for _, n := range nights {
		var cnt int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM occupancy_nights n
			JOIN occupancies o ON o.id = n.occupancy_id
			WHERE n.property_id = ? AND n.local_night_date = ? AND n.active = 1
			  AND o.source_event_uid <> ?`, propertyID, n, upstreamUID).Scan(&cnt); err != nil {
			return 0, err
		}
		if cnt > 0 {
			return 0, ErrNamedStayOverlap
		}
	}
	uid := fmt.Sprintf("named:%s:%s", upstreamUID, ci.Format("20060102"))
	var summary interface{}
	if agg.RawSummary.Valid {
		summary = agg.RawSummary.String
	}
	contentHash := fmt.Sprintf("named-stay:%s:%s:%s", upstreamUID, ci.Format(time.RFC3339), co.Format(time.RFC3339))
	res, err := tx.ExecContext(ctx, `
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			raw_summary, guest_display_name, content_hash, imported_at, last_synced_at,
			upstream_source_type, upstream_event_uid, representation_kind, representation_date)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			status = 'active',
			guest_display_name = excluded.guest_display_name,
			content_hash = excluded.content_hash,
			last_synced_at = excluded.last_synced_at,
			representation_kind = excluded.representation_kind,
			representation_date = excluded.representation_date,
			superseded_at = NULL,
			superseded_reason = NULL`,
		propertyID, manualSplitSourceType, uid, ci.Format(time.RFC3339), co.Format(time.RFC3339),
		summary, nullableString(guestName), contentHash, nowStr, nowStr,
		UpstreamSourceBookingICS, upstreamUID, RepresentationNamedStay, nullableRepresentationDate(nights))
	if err != nil {
		return 0, err
	}
	occID, _ := res.LastInsertId()
	if occID == 0 {
		_ = tx.QueryRowContext(ctx, `SELECT id FROM occupancies WHERE property_id = ? AND source_event_uid = ?`, propertyID, uid).Scan(&occID)
	}
	sourceNights := nightsUTC(agg.StartAt, agg.EndAt)
	loc := s.propertyLocationTx(ctx, tx, propertyID)
	if err := s.reconcileUpstreamCoverageTx(ctx, tx, propertyID, upstreamUID, sourceNights, now, loc, nil); err != nil {
		return 0, err
	}
	// PMS_19 §10.1: relink an existing generated Nuki code from a superseded
	// legacy split row when its window matches this named stay (avoids a PIN
	// change). Non-matching codes are left for the revocation flow.
	if _, err := s.RelinkSupersededNukiCodesTx(ctx, tx, propertyID, occID, upstreamUID, ci.Format("2006-01-02"), co.Format("2006-01-02")); err != nil {
		return 0, err
	}
	// PMS_19 §10.4: move the finance mapping to the named stay only when it
	// unambiguously covers the entire block (the whole payout maps to this
	// one stay). Sub-range names leave the mapping on the aggregate.
	if ci.Equal(blockStart) && co.Equal(blockEnd) {
		if _, err := s.MoveFinanceMappingTx(ctx, tx, propertyID, agg.ID, occID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return occID, nil
}

func nullableRepresentationDate(nights []string) interface{} {
	if len(nights) == 1 {
		return nights[0]
	}
	return nil
}

// UpdateNamedStay edits a named stay range/name and re-reconciles coverage.
func (s *Store) UpdateNamedStay(ctx context.Context, propertyID, occupancyID int64, checkIn, checkOut, guestName *string) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := s.getOccupancyByIDTx(ctx, tx, propertyID, occupancyID)
	if err != nil {
		return err
	}
	if !row.UpstreamEventUID.Valid {
		return ErrNamedStayInvalidRange
	}
	upstreamUID := row.UpstreamEventUID.String
	agg, err := s.aggregateForUpstreamTx(ctx, tx, propertyID, upstreamUID)
	if err != nil {
		return err
	}
	ci := toUTCMidnight(row.StartAt)
	co := toUTCMidnight(row.EndAt)
	if checkIn != nil {
		ci, err = parseOccupancySplitDate(*checkIn)
		if err != nil {
			return ErrNamedStayInvalidRange
		}
	}
	if checkOut != nil {
		co, err = parseOccupancySplitDate(*checkOut)
		if err != nil {
			return ErrNamedStayInvalidRange
		}
	}
	if !co.After(ci) {
		return ErrNamedStayInvalidRange
	}
	if ci.Before(toUTCMidnight(agg.StartAt)) || co.After(toUTCMidnight(agg.EndAt)) {
		return ErrNamedStayOutsideBlock
	}
	nights := nightsUTC(ci, co)
	for _, n := range nights {
		var cnt int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM occupancy_nights n
			JOIN occupancies o ON o.id = n.occupancy_id
			WHERE n.property_id = ? AND n.local_night_date = ? AND n.active = 1 AND o.id <> ?
			  AND NOT (
			    o.upstream_event_uid = ?
			    AND COALESCE(o.representation_kind, '') = ?
			    AND o.closure_state IS NULL
			  )`,
			propertyID, n, occupancyID, upstreamUID, RepresentationUnnamedBlock).Scan(&cnt); err != nil {
			return err
		}
		if cnt > 0 {
			return ErrNamedStayOverlap
		}
	}
	name := row.GuestDisplayName
	if guestName != nil {
		name = sql.NullString{String: strings.TrimSpace(*guestName), Valid: strings.TrimSpace(*guestName) != ""}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE occupancies SET start_at = ?, end_at = ?, guest_display_name = ?, status = 'active',
			superseded_at = NULL, superseded_reason = NULL, representation_date = ?, last_synced_at = ?
		WHERE id = ?`,
		ci.Format(time.RFC3339), co.Format(time.RFC3339), nullNullableString(name), nullableRepresentationDate(nights), nowStr, occupancyID); err != nil {
		return err
	}
	sourceNights := nightsUTC(agg.StartAt, agg.EndAt)
	loc := s.propertyLocationTx(ctx, tx, propertyID)
	if err := s.reconcileUpstreamCoverageTx(ctx, tx, propertyID, upstreamUID, sourceNights, now, loc, nil); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteNamedStay reverts a named stay to unnamed block coverage (or
// deleted_from_source when the source no longer covers those nights) (§7.4).
func (s *Store) DeleteNamedStay(ctx context.Context, propertyID, occupancyID int64) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row, err := s.getOccupancyByIDTx(ctx, tx, propertyID, occupancyID)
	if err != nil {
		return err
	}
	if !row.UpstreamEventUID.Valid {
		return ErrNamedStayInvalidRange
	}
	upstreamUID := row.UpstreamEventUID.String
	if _, err := tx.ExecContext(ctx, `
		UPDATE occupancies SET status = 'deleted_from_source', superseded_at = ?, superseded_reason = ?, last_synced_at = ?
		WHERE id = ?`, nowStr, SupersededReplacedByAggregate, nowStr, occupancyID); err != nil {
		return err
	}
	if err := deactivateOccupancyNightsTx(ctx, tx, occupancyID); err != nil {
		return err
	}
	var sourceNights []string
	if agg, err := s.aggregateForUpstreamTx(ctx, tx, propertyID, upstreamUID); err == nil {
		sourceNights = nightsUTC(agg.StartAt, agg.EndAt)
	}
	loc := s.propertyLocationTx(ctx, tx, propertyID)
	if err := s.reconcileUpstreamCoverageTx(ctx, tx, propertyID, upstreamUID, sourceNights, now, loc, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) getOccupancyByIDTx(ctx context.Context, tx *sql.Tx, propertyID, occupancyID int64) (*Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND id = ?`
	rows, err := tx.QueryContext(ctx, q, propertyID, occupancyID)
	if err != nil {
		return nil, err
	}
	list, err := scanOccupancyRows(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

func nullNullableString(n sql.NullString) interface{} {
	if n.Valid && strings.TrimSpace(n.String) != "" {
		return n.String
	}
	return nil
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
	upserted := c.RepresentationsInserted + c.RepresentationsUpdated
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
		now, status, em, hs, c.UpstreamEventsSeen, upserted,
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

// ReconcileBookingICSSync applies a full successful feed in one property-scoped
// transaction (§7.1 steps 7-9): upsert each aggregate, rebuild per-night
// coverage, and mark disappeared UIDs deleted_from_source.
func (s *Store) ReconcileBookingICSSync(ctx context.Context, propertyID int64, sourceType string, blocks []DesiredBlock, now time.Time, counters *SyncCounters) error {
	nowStr := now.UTC().Format(time.RFC3339)
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

	// Release disappeared UIDs first (§7.5) so their nights are free before a
	// new UID claims the same night (e.g. a rebooking under a changed UID).
	uids, err := s.listActiveUpstreamUIDsTx(ctx, tx, propertyID)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if seen[uid] {
			continue
		}
		if err := s.markUpstreamDisappearedTx(ctx, tx, propertyID, uid, now, loc, counters); err != nil {
			return err
		}
	}
	if counters != nil && counters.RawBlocksDualWrite {
		if err := s.markRawBookingBlocksDisappearedTx(ctx, tx, propertyID, sourceType, seen, now, loc, counters); err != nil {
			return err
		}
	}

	for _, b := range blocks {
		if counters != nil && counters.RawBlocksDualWrite {
			if err := s.upsertRawBookingBlockTx(ctx, tx, propertyID, sourceType, b, now, counters); err != nil {
				return err
			}
		}
		existing, err := s.getOccupancyBySourceEventUIDTx(ctx, tx, propertyID, b.UID)
		if err != nil {
			return err
		}
		status := "active"
		if b.Cancelled {
			status = "cancelled"
		} else if existing != nil {
			if existing.ContentHash != b.ContentHash || existing.Status == "cancelled" || existing.Status == StatusDeletedFromSource {
				status = "updated"
			}
		}
		var summary interface{}
		if b.Summary != "" {
			summary = b.Summary
		}
		var dtstamp interface{}
		if !b.DTStamp.IsZero() {
			dtstamp = b.DTStamp.UTC().Format(time.RFC3339)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO occupancies (property_id, source_type, source_event_uid, start_at, end_at, status,
				raw_summary, content_hash, imported_at, last_synced_at,
				upstream_source_type, upstream_event_uid, representation_kind, source_dtstamp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
				start_at = excluded.start_at,
				end_at = excluded.end_at,
				status = excluded.status,
				raw_summary = excluded.raw_summary,
				content_hash = excluded.content_hash,
				last_synced_at = excluded.last_synced_at,
				upstream_source_type = excluded.upstream_source_type,
				upstream_event_uid = excluded.upstream_event_uid,
				representation_kind = COALESCE(occupancies.representation_kind, excluded.representation_kind),
				source_dtstamp = COALESCE(excluded.source_dtstamp, occupancies.source_dtstamp),
				superseded_at = CASE WHEN excluded.status = 'cancelled' THEN occupancies.superseded_at ELSE NULL END,
				superseded_reason = CASE WHEN excluded.status = 'cancelled' THEN occupancies.superseded_reason ELSE NULL END`,
			propertyID, sourceType, b.UID, b.Start.UTC().Format(time.RFC3339), b.End.UTC().Format(time.RFC3339), status,
			summary, b.ContentHash, nowStr, nowStr,
			UpstreamSourceBookingICS, b.UID, RepresentationUnnamedBlock, dtstamp); err != nil {
			return err
		}
		if counters != nil {
			switch {
			case existing == nil:
				counters.RepresentationsInserted++
			case existing.ContentHash != b.ContentHash:
				counters.RepresentationsUpdated++
			default:
				counters.RepresentationsUnchanged++
			}
		}
		var sourceNights []string
		if !b.Cancelled {
			sourceNights = nightsUTC(b.Start, b.End)
		}
		if err := s.reconcileUpstreamCoverageTx(ctx, tx, propertyID, b.UID, sourceNights, now, loc, counters); err != nil {
			return err
		}
	}
	if counters != nil && counters.RawBlocksDualWrite {
		newConflicts, err := recomputeSourceLinkHealthTx(ctx, tx, propertyID, nowStr)
		if err != nil {
			return err
		}
		counters.RawBlockConflicts += newConflicts
	}

	return tx.Commit()
}

func (s *Store) getOccupancyBySourceEventUIDTx(ctx context.Context, tx *sql.Tx, propertyID int64, uid string) (*Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND source_event_uid = ?`
	rows, err := tx.QueryContext(ctx, q, propertyID, uid)
	if err != nil {
		return nil, err
	}
	list, err := scanOccupancyRows(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
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

func (s *Store) listActiveUpstreamUIDsTx(ctx context.Context, tx *sql.Tx, propertyID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT upstream_event_uid FROM occupancies
		WHERE property_id = ? AND upstream_source_type = ? AND upstream_event_uid IS NOT NULL
		  AND status IN ('active','updated')`, propertyID, UpstreamSourceBookingICS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// BackfillUpstreamOwnership derives upstream ownership fields for pre-PMS_19
// rows and builds initial night coverage. Idempotent: ownership is only set
// where NULL, and coverage is only built for properties that have none yet
// (§14 step 3).
func (s *Store) BackfillUpstreamOwnership(ctx context.Context) error {
	q := occupancySelectColumns + ` FROM occupancies WHERE upstream_source_type IS NULL`
	rows, err := s.scanOccupancies(ctx, q)
	if err != nil {
		return err
	}
	for i := range rows {
		o := &rows[i]
		uid := DeriveUpstreamUID(o.SourceEventUID)
		kind := deriveRepresentationKind(o)
		nights := nightsUTC(o.StartAt, o.EndAt)
		var repDate interface{}
		if len(nights) == 1 {
			repDate = nights[0]
		}
		if _, err := s.DB.ExecContext(ctx, `
			UPDATE occupancies SET upstream_source_type = ?, upstream_event_uid = ?, representation_kind = ?, representation_date = ?
			WHERE id = ?`, UpstreamSourceBookingICS, uid, kind, repDate, o.ID); err != nil {
			return err
		}
	}
	// Build coverage per property when absent.
	propRows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT property_id FROM occupancies`)
	if err != nil {
		return err
	}
	var props []int64
	for propRows.Next() {
		var p int64
		if err := propRows.Scan(&p); err != nil {
			propRows.Close()
			return err
		}
		props = append(props, p)
	}
	propRows.Close()
	now := time.Now().UTC()
	for _, p := range props {
		var cnt int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancy_nights WHERE property_id = ?`, p).Scan(&cnt); err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		if err := s.backfillPropertyCoverage(ctx, p, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) backfillPropertyCoverage(ctx context.Context, propertyID int64, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := s.listActivePropertyRowsTx(ctx, tx, propertyID)
	if err != nil {
		return err
	}
	_, _, rowNights := assignNightWinners(rows)
	// Startup backfill is conservative: it builds first-wins coverage and never
	// supersedes rows. Latent duplicates are left for the explicit repair path.
	for i := range rows {
		if err := clearOccupancyNightsTx(ctx, tx, rows[i].ID); err != nil {
			return err
		}
	}
	for i := range rows {
		r := &rows[i]
		nights := rowNights[r.ID]
		if len(nights) == 0 {
			continue
		}
		if err := setOccupancyNightsTx(ctx, tx, propertyID, r.ID, nights, nullOrString(r.UpstreamSourceType), nullOrString(r.UpstreamEventUID), true); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) listActivePropertyRowsTx(ctx context.Context, tx *sql.Tx, propertyID int64) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND status IN ('active','updated') AND superseded_at IS NULL ORDER BY id ASC`
	rows, err := tx.QueryContext(ctx, q, propertyID)
	if err != nil {
		return nil, err
	}
	return scanOccupancyRows(rows)
}

// assignNightWinners resolves capacity-one ownership of every property-local
// night across all active rows using the §7.6 priority order. It returns the
// winning row per night, the losing row ids per night, and the winning nights
// per row.
func assignNightWinners(rows []Occupancy) (winnerByNight map[string]*Occupancy, losersByNight map[string][]int64, rowNights map[int64][]string) {
	ordered := make([]*Occupancy, len(rows))
	for i := range rows {
		ordered[i] = &rows[i]
	}
	sort.SliceStable(ordered, func(a, b int) bool { return repairBeats(ordered[a], ordered[b]) })
	winnerByNight = map[string]*Occupancy{}
	losersByNight = map[string][]int64{}
	rowNights = map[int64][]string{}
	for _, r := range ordered {
		for _, n := range nightsUTC(r.StartAt, r.EndAt) {
			if w, ok := winnerByNight[n]; ok {
				if w.ID != r.ID {
					losersByNight[n] = append(losersByNight[n], r.ID)
				}
				continue
			}
			winnerByNight[n] = r
			rowNights[r.ID] = append(rowNights[r.ID], n)
		}
	}
	return winnerByNight, losersByNight, rowNights
}
