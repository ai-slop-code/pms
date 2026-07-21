package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type OccupancyCalendarView struct {
	PropertyID         int64                       `json:"property_id"`
	Month              string                      `json:"month"`
	RawBlocks          []CalendarRawBookingBlock   `json:"raw_blocks"`
	NamedStays         []CalendarNamedStay         `json:"named_stays"`
	AvailabilityBlocks []CalendarAvailabilityBlock `json:"availability_blocks"`
}

type CalendarRawBookingBlock struct {
	ID                int64                   `json:"id"`
	PropertyID        int64                   `json:"property_id"`
	SourceType        string                  `json:"source_type"`
	SourceEventUID    string                  `json:"source_event_uid"`
	CheckInDate       string                  `json:"check_in_date"`
	CheckOutDate      string                  `json:"check_out_date"`
	Status            string                  `json:"status"`
	RawSummary        *string                 `json:"raw_summary,omitempty"`
	SourceDtstamp     *string                 `json:"source_dtstamp,omitempty"`
	LastSyncRunID     *int64                  `json:"last_sync_run_id,omitempty"`
	ConflictReason    *string                 `json:"conflict_reason,omitempty"`
	CoveredNights     []string                `json:"covered_nights"`
	LegacyOccupancyID *int64                  `json:"legacy_occupancy_id,omitempty"`
	CleaningEvents    []CalendarCleaningEvent `json:"cleaning_events"`
}

type CalendarNamedStay struct {
	ID                   int64                    `json:"id"`
	PropertyID           int64                    `json:"property_id"`
	DisplayName          string                   `json:"display_name"`
	StayType             string                   `json:"stay_type"`
	CheckInDate          string                   `json:"check_in_date"`
	CheckOutDate         string                   `json:"check_out_date"`
	Status               string                   `json:"status"`
	CleaningRequired     bool                     `json:"cleaning_required"`
	ReviewStatus         string                   `json:"review_status"`
	CountsAsSold         bool                     `json:"counts_as_sold"`
	HasFinanceEvidence   bool                     `json:"has_finance_evidence"`
	NukiGenerationStatus string                   `json:"nuki_generation_status"`
	NukiGenerationError  *string                  `json:"nuki_generation_error,omitempty"`
	CoveredNights        []string                 `json:"covered_nights"`
	LegacyOccupancyID    *int64                   `json:"legacy_occupancy_id,omitempty"`
	SourceLinks          []CalendarStaySourceLink `json:"source_links"`
	CleaningEvents       []CalendarCleaningEvent  `json:"cleaning_events"`
}

type CalendarCleaningEvent struct {
	ID             int64   `json:"id"`
	CheckoutDate   string  `json:"checkout_date"`
	CleaningKind   string  `json:"cleaning_kind"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	GoogleEventID  *string `json:"google_event_id,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	WarningMessage *string `json:"warning_message,omitempty"`
}

type CalendarStaySourceLink struct {
	ID                 int64   `json:"id"`
	RawBookingBlockID  *int64  `json:"raw_booking_block_id,omitempty"`
	SourceType         string  `json:"source_type"`
	SourceEventUID     *string `json:"source_event_uid,omitempty"`
	LinkedCheckInDate  string  `json:"linked_check_in_date"`
	LinkedCheckOutDate string  `json:"linked_check_out_date"`
	LinkStatus         string  `json:"link_status"`
	ConflictReason     *string `json:"conflict_reason,omitempty"`
}

type CalendarAvailabilityBlock struct {
	ID            int64    `json:"id"`
	PropertyID    int64    `json:"property_id"`
	BlockType     string   `json:"block_type"`
	StartDate     string   `json:"start_date"`
	EndDate       string   `json:"end_date"`
	Reason        *string  `json:"reason,omitempty"`
	Status        string   `json:"status"`
	CoveredNights []string `json:"covered_nights"`
}

type AvailabilityBlockInput struct {
	BlockType    string
	StartDate    string
	EndDate      string
	Reason       string
	ActingUserID int64
}

func (s *Store) OccupancyCalendarView(ctx context.Context, propertyID int64, month string) (*OccupancyCalendarView, error) {
	start, end, err := calendarMonthRange(month)
	if err != nil {
		return nil, err
	}
	startText := start.Format("2006-01-02")
	endText := end.Format("2006-01-02")
	rawBlocks, err := s.ListCalendarRawBookingBlocks(ctx, propertyID, startText, endText)
	if err != nil {
		return nil, err
	}
	namedStays, err := s.ListCalendarNamedStays(ctx, propertyID, startText, endText)
	if err != nil {
		return nil, err
	}
	availabilityBlocks, err := s.ListCalendarAvailabilityBlocks(ctx, propertyID, startText, endText)
	if err != nil {
		return nil, err
	}
	return &OccupancyCalendarView{
		PropertyID:         propertyID,
		Month:              month,
		RawBlocks:          rawBlocks,
		NamedStays:         namedStays,
		AvailabilityBlocks: availabilityBlocks,
	}, nil
}

func (s *Store) ListCalendarRawBookingBlocks(ctx context.Context, propertyID int64, startDate, endDate string) ([]CalendarRawBookingBlock, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT rb.id, rb.property_id, rb.source_type, rb.source_event_uid, rb.check_in_date, rb.check_out_date,
		       rb.status, rb.raw_summary, rb.source_dtstamp, rb.last_sync_run_id, rb.conflict_reason, osm.old_occupancy_id
		FROM raw_booking_blocks rb
		LEFT JOIN occupancy_stay_migration_map osm ON osm.raw_booking_block_id = rb.id AND osm.migration_kind = 'raw_block'
		WHERE rb.property_id = ? AND rb.check_in_date < ? AND rb.check_out_date > ?
		ORDER BY rb.check_in_date, rb.id`, propertyID, endDate, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CalendarRawBookingBlock{}
	byID := map[int64]int{}
	for rows.Next() {
		var b CalendarRawBookingBlock
		var rawSummary, sourceDtstamp, conflictReason sql.NullString
		var lastSyncRunID, legacyOccupancyID sql.NullInt64
		if err := rows.Scan(&b.ID, &b.PropertyID, &b.SourceType, &b.SourceEventUID, &b.CheckInDate, &b.CheckOutDate,
			&b.Status, &rawSummary, &sourceDtstamp, &lastSyncRunID, &conflictReason, &legacyOccupancyID); err != nil {
			return nil, err
		}
		b.RawSummary = stringPtr(rawSummary)
		b.SourceDtstamp = stringPtr(sourceDtstamp)
		b.LastSyncRunID = int64Ptr(lastSyncRunID)
		b.ConflictReason = stringPtr(conflictReason)
		b.LegacyOccupancyID = int64Ptr(legacyOccupancyID)
		b.CoveredNights = []string{}
		b.CleaningEvents = []CalendarCleaningEvent{}
		byID[b.ID] = len(out)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	if err := s.attachRawCalendarCleaningEvents(ctx, propertyID, startDate, endDate, out, byID); err != nil {
		return nil, err
	}

	nightRows, err := s.DB.QueryContext(ctx, `
		SELECT rbn.raw_booking_block_id, rbn.local_night_date
		FROM raw_booking_block_nights rbn
		JOIN raw_booking_blocks rb ON rb.id = rbn.raw_booking_block_id
		WHERE rbn.property_id = ? AND rbn.active = 1 AND rb.status = 'active'
		  AND rbn.local_night_date >= ? AND rbn.local_night_date < ?
		ORDER BY rbn.local_night_date, rbn.raw_booking_block_id`, propertyID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer nightRows.Close()
	for nightRows.Next() {
		var blockID int64
		var night string
		if err := nightRows.Scan(&blockID, &night); err != nil {
			return nil, err
		}
		if idx, ok := byID[blockID]; ok {
			out[idx].CoveredNights = append(out[idx].CoveredNights, night)
		}
	}
	return out, nightRows.Err()
}

func (s *Store) ListCalendarNamedStays(ctx context.Context, propertyID int64, startDate, endDate string) ([]CalendarNamedStay, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date,
		       ns.status, ns.cleaning_required, ns.review_status,
		       CASE WHEN ns.status = 'active' AND COALESCE(ns.review_status, 'confirmed') = 'confirmed' AND (
		           ns.stay_type = 'booking_com' OR (
		               ns.stay_type = 'external' AND (
		                   ns.manual_revenue_cents IS NOT NULL OR EXISTS (
		                       SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id
		                   )
		               )
		           )
		       ) THEN 1 ELSE 0 END,
		       EXISTS (
		           SELECT 1
		           FROM finance_bookings fb
		           WHERE fb.property_id = ns.property_id
		             AND fb.named_stay_id = ns.id
		             AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
		             AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
		             AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
		                 ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
		       ),
		       ns.nuki_generation_status, ns.nuki_generation_error,
		       osm.old_occupancy_id
		FROM named_stays ns
		LEFT JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id AND osm.migration_kind = 'named_stay'
		WHERE ns.property_id = ? AND ns.status <> 'archived' AND ns.check_in_date < ? AND ns.check_out_date > ?
		ORDER BY ns.check_in_date, ns.id`, propertyID, endDate, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CalendarNamedStay{}
	byID := map[int64]int{}
	for rows.Next() {
		var stay CalendarNamedStay
		var cleaningRequired int
		var countsAsSold int
		var hasFinanceEvidence int
		var reviewStatus, nukiStatus, nukiError sql.NullString
		var legacyOccupancyID sql.NullInt64
		if err := rows.Scan(&stay.ID, &stay.PropertyID, &stay.DisplayName, &stay.StayType, &stay.CheckInDate, &stay.CheckOutDate,
			&stay.Status, &cleaningRequired, &reviewStatus, &countsAsSold, &hasFinanceEvidence,
			&nukiStatus, &nukiError, &legacyOccupancyID); err != nil {
			return nil, err
		}
		stay.CleaningRequired = cleaningRequired == 1
		stay.ReviewStatus = nullStringDefault(reviewStatus, "confirmed")
		stay.CountsAsSold = countsAsSold == 1
		stay.HasFinanceEvidence = hasFinanceEvidence == 1
		stay.NukiGenerationStatus = nullStringDefault(nukiStatus, NukiGenerationNotApplicable)
		stay.NukiGenerationError = stringPtr(nukiError)
		stay.LegacyOccupancyID = int64Ptr(legacyOccupancyID)
		stay.CoveredNights = []string{}
		stay.SourceLinks = []CalendarStaySourceLink{}
		stay.CleaningEvents = []CalendarCleaningEvent{}
		byID[stay.ID] = len(out)
		out = append(out, stay)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	if err := s.attachNamedCalendarCleaningEvents(ctx, propertyID, startDate, endDate, out, byID); err != nil {
		return nil, err
	}

	nightRows, err := s.DB.QueryContext(ctx, `
		SELECT named_stay_id, local_night_date
		FROM named_stay_nights
		WHERE property_id = ? AND active = 1 AND local_night_date >= ? AND local_night_date < ?
		ORDER BY local_night_date, named_stay_id`, propertyID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer nightRows.Close()
	for nightRows.Next() {
		var stayID int64
		var night string
		if err := nightRows.Scan(&stayID, &night); err != nil {
			return nil, err
		}
		if idx, ok := byID[stayID]; ok {
			out[idx].CoveredNights = append(out[idx].CoveredNights, night)
		}
	}
	if err := nightRows.Err(); err != nil {
		return nil, err
	}

	linkRows, err := s.DB.QueryContext(ctx, `
		SELECT id, named_stay_id, raw_booking_block_id, source_type, source_event_uid,
		       linked_check_in_date, linked_check_out_date, link_status, conflict_reason
		FROM stay_source_links
		WHERE property_id = ?
		ORDER BY id`, propertyID)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var stayID int64
		var link CalendarStaySourceLink
		var rawBlockID sql.NullInt64
		var sourceUID, conflictReason sql.NullString
		if err := linkRows.Scan(&link.ID, &stayID, &rawBlockID, &link.SourceType, &sourceUID,
			&link.LinkedCheckInDate, &link.LinkedCheckOutDate, &link.LinkStatus, &conflictReason); err != nil {
			return nil, err
		}
		link.RawBookingBlockID = int64Ptr(rawBlockID)
		link.SourceEventUID = stringPtr(sourceUID)
		link.ConflictReason = stringPtr(conflictReason)
		if idx, ok := byID[stayID]; ok {
			out[idx].SourceLinks = append(out[idx].SourceLinks, link)
		}
	}
	return out, linkRows.Err()
}

func (s *Store) CreateAvailabilityBlock(ctx context.Context, propertyID int64, in AvailabilityBlockInput) (*CalendarAvailabilityBlock, error) {
	blockType, start, end, reason, err := normalizeAvailabilityBlockInput(in)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := availabilityBlockRangeAvailableTx(ctx, tx, propertyID, start, end); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO property_availability_blocks (
			property_id, block_type, start_date, end_date, reason, status, created_by_user_id, updated_by_user_id, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)`, propertyID, blockType, start, end, nullableString(reason), nullableInt64(in.ActingUserID), nullableInt64(in.ActingUserID), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetAvailabilityBlock(ctx, propertyID, id)
}

func (s *Store) UpdateAvailabilityBlock(ctx context.Context, propertyID, blockID int64, in AvailabilityBlockInput) (*CalendarAvailabilityBlock, error) {
	blockType, start, end, reason, err := normalizeAvailabilityBlockInput(in)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM property_availability_blocks WHERE property_id = ? AND id = ?`, propertyID, blockID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, sql.ErrNoRows
	}
	if err := availabilityBlockRangeAvailableTx(ctx, tx, propertyID, start, end); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE property_availability_blocks
		SET block_type = ?, start_date = ?, end_date = ?, reason = ?, updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, blockType, start, end, nullableString(reason), nullableInt64(in.ActingUserID), now, propertyID, blockID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetAvailabilityBlock(ctx, propertyID, blockID)
}

func (s *Store) GetAvailabilityBlock(ctx context.Context, propertyID, blockID int64) (*CalendarAvailabilityBlock, error) {
	var block CalendarAvailabilityBlock
	var reason sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, block_type, start_date, end_date, reason, status
		FROM property_availability_blocks
		WHERE property_id = ? AND id = ?`, propertyID, blockID).
		Scan(&block.ID, &block.PropertyID, &block.BlockType, &block.StartDate, &block.EndDate, &reason, &block.Status)
	if err != nil {
		return nil, err
	}
	block.Reason = stringPtr(reason)
	block.CoveredNights = clippedNights(block.StartDate, block.EndDate, block.StartDate, block.EndDate)
	return &block, nil
}

func normalizeAvailabilityBlockInput(in AvailabilityBlockInput) (string, string, string, string, error) {
	blockType := strings.TrimSpace(in.BlockType)
	if blockType != "closed" && blockType != "off_market" {
		return "", "", "", "", ErrNamedStayInvalidRange
	}
	start, end, err := parseNamedStayRange(in.StartDate, in.EndDate)
	if err != nil {
		return "", "", "", "", err
	}
	return blockType, start.Format("2006-01-02"), end.Format("2006-01-02"), strings.TrimSpace(in.Reason), nil
}

func availabilityBlockRangeAvailableTx(ctx context.Context, tx *sql.Tx, propertyID int64, startDate, endDate string) error {
	var cnt int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM named_stay_nights
		WHERE property_id = ? AND active = 1 AND local_night_date >= ? AND local_night_date < ?`, propertyID, startDate, endDate).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return ErrNamedStayOverlap
	}
	return nil
}

func (s *Store) attachNamedCalendarCleaningEvents(ctx context.Context, propertyID int64, startDate, endDate string, stays []CalendarNamedStay, byID map[int64]int) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.named_stay_id, osm.named_stay_id), c.checkout_date, c.cleaning_kind, c.title, c.status, c.google_event_id, c.error_message, c.warning_message
		FROM cleaning_calendar_events c
		LEFT JOIN occupancy_stay_migration_map osm ON osm.old_occupancy_id = c.occupancy_id AND osm.migration_kind = 'named_stay'
		WHERE c.property_id = ? AND c.status <> 'removed' AND c.checkout_date >= ? AND c.checkout_date <= ?
		  AND COALESCE(c.named_stay_id, osm.named_stay_id) IS NOT NULL`, propertyID, startDate, endDate)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var stayID int64
		event, err := scanCalendarCleaningEvent(rows, &stayID)
		if err != nil {
			return err
		}
		if idx, ok := byID[stayID]; ok {
			stays[idx].CleaningEvents = append(stays[idx].CleaningEvents, event)
		}
	}
	return rows.Err()
}

func (s *Store) attachRawCalendarCleaningEvents(ctx context.Context, propertyID int64, startDate, endDate string, blocks []CalendarRawBookingBlock, byID map[int64]int) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.raw_booking_block_id, osm.raw_booking_block_id), c.checkout_date, c.cleaning_kind, c.title, c.status, c.google_event_id, c.error_message, c.warning_message
		FROM cleaning_calendar_events c
		LEFT JOIN occupancy_stay_migration_map osm ON osm.old_occupancy_id = c.occupancy_id AND osm.migration_kind = 'raw_block'
		WHERE c.property_id = ? AND c.status <> 'removed' AND c.checkout_date >= ? AND c.checkout_date <= ?
		  AND COALESCE(c.raw_booking_block_id, osm.raw_booking_block_id) IS NOT NULL`, propertyID, startDate, endDate)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var blockID int64
		event, err := scanCalendarCleaningEvent(rows, &blockID)
		if err != nil {
			return err
		}
		if idx, ok := byID[blockID]; ok {
			blocks[idx].CleaningEvents = append(blocks[idx].CleaningEvents, event)
		}
	}
	return rows.Err()
}

func scanCalendarCleaningEvent(rows *sql.Rows, ownerID *int64) (CalendarCleaningEvent, error) {
	var event CalendarCleaningEvent
	var googleID, errMsg, warning sql.NullString
	if err := rows.Scan(&event.ID, ownerID, &event.CheckoutDate, &event.CleaningKind, &event.Title, &event.Status, &googleID, &errMsg, &warning); err != nil {
		return event, err
	}
	event.GoogleEventID = stringPtr(googleID)
	event.ErrorMessage = stringPtr(errMsg)
	event.WarningMessage = stringPtr(warning)
	return event, nil
}

func (s *Store) ListCalendarAvailabilityBlocks(ctx context.Context, propertyID int64, startDate, endDate string) ([]CalendarAvailabilityBlock, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, block_type, start_date, end_date, reason, status
		FROM property_availability_blocks
		WHERE property_id = ? AND status = 'active' AND start_date < ? AND end_date > ?
		ORDER BY start_date, id`, propertyID, endDate, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CalendarAvailabilityBlock{}
	for rows.Next() {
		var block CalendarAvailabilityBlock
		var reason sql.NullString
		if err := rows.Scan(&block.ID, &block.PropertyID, &block.BlockType, &block.StartDate, &block.EndDate, &reason, &block.Status); err != nil {
			return nil, err
		}
		block.Reason = stringPtr(reason)
		block.CoveredNights = clippedNights(block.StartDate, block.EndDate, startDate, endDate)
		out = append(out, block)
	}
	return out, rows.Err()
}

func calendarMonthRange(month string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, time.Time{}, ErrNamedStayInvalidRange
	}
	return start, start.AddDate(0, 1, 0), nil
}

func clippedNights(checkIn, checkOut, windowStart, windowEnd string) []string {
	ci, err1 := time.Parse("2006-01-02", maxDateText(checkIn, windowStart))
	co, err2 := time.Parse("2006-01-02", minDateText(checkOut, windowEnd))
	if err1 != nil || err2 != nil || !co.After(ci) {
		return []string{}
	}
	return nightsUTC(ci, co)
}

func minDateText(a, b string) string {
	if a < b {
		return a
	}
	return b
}

func maxDateText(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func stringPtr(v sql.NullString) *string {
	if !v.Valid || v.String == "" {
		return nil
	}
	s := v.String
	return &s
}

func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	i := v.Int64
	return &i
}

func nullStringDefault(v sql.NullString, fallback string) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return fallback
}
