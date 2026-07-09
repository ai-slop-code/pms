package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	ID                  int64
	PropertyID          int64
	StartedAt           time.Time
	FinishedAt          sql.NullTime
	Status              string
	ErrorMessage        sql.NullString
	EventsSeen          int
	OccupanciesUpserted int
	HTTPStatus          sql.NullInt64
	Trigger             string
	CreatedAt           time.Time
}

type Occupancy struct {
	ID               int64
	PropertyID       int64
	SourceType       string
	SourceEventUID   string
	StartAt          time.Time
	EndAt            time.Time
	Status           string
	RawSummary       sql.NullString
	GuestDisplayName sql.NullString
	ContentHash      string
	ImportedAt       time.Time
	LastSyncedAt     time.Time
	LastSyncRunID    sql.NullInt64
	// Closure / external-sale labelling. When ClosureState is non-NULL the
	// row is excluded from active-status analytics in different ways depending
	// on the value (see PMS_14 §4):
	//   - 'closed'        → night drops out of nights_sold and available_nights
	//   - 'external_sale' → night counts as sold + available, with the
	//                       operator-entered net amount feeding gross_revenue.
	ClosureState                     sql.NullString
	ClosureReason                    sql.NullString
	ClosureCategory                  sql.NullString
	ClosedByUserID                   sql.NullInt64
	ClosedAt                         sql.NullTime
	ExternalNetAmountCents           sql.NullInt64
	ExternalCurrency                 sql.NullString
	ExternalChannel                  sql.NullString
	FinanceBookingID                 sql.NullInt64
	StayOutcome                      sql.NullString
	StayOutcomeReason                sql.NullString
	StayOutcomeMarkedByUserID        sql.NullInt64
	StayOutcomeMarkedAt              sql.NullTime
	CleaningCalendarExcluded         bool
	CleaningCalendarExclusionReason  sql.NullString
	CleaningCalendarExcludedByUserID sql.NullInt64
	CleaningCalendarExcludedAt       sql.NullTime
}

// Closure state constants (occupancies.closure_state values).
const (
	ClosureStateClosed                = "closed"
	ClosureStateExternalSale          = "external_sale"
	StayOutcomeCancelledNonRefundable = "cancelled_non_refundable"
	StayOutcomeNoShow                 = "no_show"
	manualSplitSourceType             = "manual"
	manualSplitUIDPrefix              = "manual_split:"
)

type UpcomingOccupancy struct {
	ID               int64
	SourceEventUID   string
	RawSummary       sql.NullString
	GuestDisplayName sql.NullString
	StartAt          time.Time
	EndAt            time.Time
	Status           string
}

type OccupancyAPIToken struct {
	ID         int64
	PropertyID int64
	Label      sql.NullString
	CreatedAt  time.Time
	LastUsedAt sql.NullTime
}

func (s *Store) InsertOccupancySourceTx(ctx context.Context, tx *sql.Tx, propertyID int64, now string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO occupancy_sources (property_id, source_type, active, created_at, updated_at)
		VALUES (?, 'booking_ics', 1, ?, ?)
		ON CONFLICT(property_id) DO NOTHING`, propertyID, now, now)
	return err
}

func (s *Store) GetOccupancySource(ctx context.Context, propertyID int64) (*OccupancySource, error) {
	var o OccupancySource
	var active int
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, source_type, active, created_at, updated_at
		FROM occupancy_sources WHERE property_id = ?`, propertyID).
		Scan(&o.ID, &o.PropertyID, &o.SourceType, &active, &created, &updated)
	if err != nil {
		return nil, err
	}
	o.Active = active == 1
	o.CreatedAt, _ = time.Parse(time.RFC3339, created)
	o.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &o, nil
}

func (s *Store) UpdateOccupancySource(ctx context.Context, propertyID int64, active *bool, sourceType *string) error {
	src, err := s.GetOccupancySource(ctx, propertyID)
	if err != nil {
		return err
	}
	if active != nil {
		src.Active = *active
	}
	if sourceType != nil {
		src.SourceType = *sourceType
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a := 0
	if src.Active {
		a = 1
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE occupancy_sources SET source_type = ?, active = ?, updated_at = ? WHERE property_id = ?`,
		src.SourceType, a, now, propertyID)
	return err
}

func (s *Store) StartOccupancySyncRun(ctx context.Context, propertyID int64, trigger string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancy_sync_runs (property_id, started_at, status, trigger, created_at)
		VALUES (?, ?, 'running', ?, ?)`, propertyID, now, trigger, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishOccupancySyncRun(ctx context.Context, runID int64, status string, errMsg *string, httpStatus *int, eventsSeen, upserted int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var em interface{}
	if errMsg != nil {
		em = *errMsg
	}
	var hs interface{}
	if httpStatus != nil {
		hs = *httpStatus
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE occupancy_sync_runs SET finished_at = ?, status = ?, error_message = ?, http_status = ?, events_seen = ?, occupancies_upserted = ?
		WHERE id = ?`, now, status, em, hs, eventsSeen, upserted, runID)
	return err
}

func (s *Store) InsertOccupancyRawEvent(ctx context.Context, propertyID, syncRunID int64, uid, raw, summary, startRFC, endRFC string, seq int, icalStatus, hash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancy_raw_events (property_id, sync_run_id, source_event_uid, raw_component, summary, event_start, event_end, sequence_num, ical_status, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, sync_run_id, source_event_uid) DO UPDATE SET
			raw_component = excluded.raw_component,
			summary = excluded.summary,
			event_start = excluded.event_start,
			event_end = excluded.event_end,
			sequence_num = excluded.sequence_num,
			ical_status = excluded.ical_status,
			content_hash = excluded.content_hash`, propertyID, syncRunID, uid, raw, summary, startRFC, endRFC, seq, icalStatus, hash, now)
	return err
}

func (s *Store) UpsertOccupancy(ctx context.Context, o *Occupancy, syncRunID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	start := o.StartAt.UTC().Format(time.RFC3339)
	end := o.EndAt.UTC().Format(time.RFC3339)
	var guest interface{}
	if o.GuestDisplayName.Valid {
		guest = o.GuestDisplayName.String
	}
	var sum interface{}
	if o.RawSummary.Valid {
		sum = o.RawSummary.String
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancies (property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			status = excluded.status,
			raw_summary = excluded.raw_summary,
			guest_display_name = COALESCE(excluded.guest_display_name, occupancies.guest_display_name),
			content_hash = excluded.content_hash,
			last_synced_at = excluded.last_synced_at,
			last_sync_run_id = excluded.last_sync_run_id`,
		o.PropertyID, o.SourceType, o.SourceEventUID, start, end, o.Status, sum, guest, o.ContentHash, now, now, syncRunID)
	return err
}

func (s *Store) UpdateOccupancyGuestDisplayName(ctx context.Context, propertyID, occupancyID int64, name *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var guest interface{}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n != "" {
			guest = n
		}
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET guest_display_name = ?, last_synced_at = ?
		WHERE property_id = ? AND id = ?`, guest, now, propertyID, occupancyID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetOccupancyBySourceEventUID(ctx context.Context, propertyID int64, sourceEventUID string) (*Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND source_event_uid = ?`
	rows, err := s.scanOccupancies(ctx, q, propertyID, sourceEventUID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) GetOccupancyByID(ctx context.Context, propertyID, occupancyID int64) (*Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND id = ?`
	rows, err := s.scanOccupancies(ctx, q, propertyID, occupancyID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return &rows[0], nil
}

func (s *Store) MarkOccupanciesDeletedFromSource(ctx context.Context, propertyID int64, sourceType string, keepUIDs []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	deleteCutoff := toUTCMidnight(time.Now().UTC()).Format(time.RFC3339)
	if len(keepUIDs) == 0 {
		_, err := s.DB.ExecContext(ctx, `
			UPDATE occupancies SET status = 'deleted_from_source', last_synced_at = ?
			WHERE property_id = ? AND source_type = ? AND status NOT IN ('deleted_from_source')
			AND end_at >= ?`,
			now, propertyID, sourceType, deleteCutoff)
		return err
	}
	ph := make([]string, len(keepUIDs))
	args := make([]interface{}, 0, len(keepUIDs)+4)
	args = append(args, now, propertyID, sourceType, deleteCutoff)
	for i, u := range keepUIDs {
		ph[i] = "?"
		args = append(args, u)
	}
	q := fmt.Sprintf(`
		UPDATE occupancies SET status = 'deleted_from_source', last_synced_at = ?
		WHERE property_id = ? AND source_type = ? AND status NOT IN ('deleted_from_source')
		AND end_at >= ?
		AND source_event_uid NOT IN (%s)`, strings.Join(ph, ","))
	_, err := s.DB.ExecContext(ctx, q, args...)
	return err
}

func (s *Store) ListOccupancies(ctx context.Context, propertyID int64, month string, loc *time.Location, statusFilter *string, limit, offset int) ([]Occupancy, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if month != "" {
		if loc == nil {
			loc = time.UTC
		}
		list, err := s.ListOccupanciesOverlappingMonthInTZ(ctx, propertyID, month, loc)
		if err != nil {
			return nil, err
		}
		if statusFilter != nil && *statusFilter != "" {
			var filtered []Occupancy
			for _, o := range list {
				if o.Status == *statusFilter {
					filtered = append(filtered, o)
				}
			}
			list = filtered
		}
		if offset >= len(list) {
			return nil, nil
		}
		end := offset + limit
		if end > len(list) {
			end = len(list)
		}
		return list[offset:end], nil
	}
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ?`
	args := []interface{}{propertyID}
	if statusFilter != nil && *statusFilter != "" {
		q += ` AND status = ?`
		args = append(args, *statusFilter)
	}
	q += ` ORDER BY start_at ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	return s.scanOccupancies(ctx, q, args...)
}

func (s *Store) ListOccupanciesOverlappingMonthInTZ(ctx context.Context, propertyID int64, month string, loc *time.Location) ([]Occupancy, error) {
	var y, mi int
	if _, err := fmt.Sscanf(month, "%d-%d", &y, &mi); err != nil || mi < 1 || mi > 12 {
		return nil, fmt.Errorf("month must be YYYY-MM")
	}
	mon := time.Month(mi)
	start := time.Date(y, mon, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	return s.ListOccupanciesBetween(ctx, propertyID, start.UTC(), end.UTC())
}

func (s *Store) ListOccupanciesBetween(ctx context.Context, propertyID int64, startUTC, endUTC time.Time) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND start_at < ? AND end_at > ?
		ORDER BY start_at ASC`
	return s.scanOccupancies(ctx, q, propertyID, endUTC.Format(time.RFC3339), startUTC.Format(time.RFC3339))
}

// occupancySelectColumns is the canonical column list for Occupancy rows so
// scanOccupancies stays in lockstep with every caller.
const occupancySelectColumns = `
	SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status,
	       raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id,
	       closure_state, closure_reason, closure_category, closed_by_user_id, closed_at,
	       external_net_amount_cents, external_currency, external_channel, finance_booking_id,
	       stay_outcome, stay_outcome_reason, stay_outcome_marked_by_user_id, stay_outcome_marked_at,
	       cleaning_calendar_excluded, cleaning_calendar_exclusion_reason,
	       cleaning_calendar_excluded_by_user_id, cleaning_calendar_excluded_at`

func (s *Store) scanOccupancies(ctx context.Context, q string, args ...interface{}) ([]Occupancy, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Occupancy
	for rows.Next() {
		var o Occupancy
		var start, end, imp, last string
		var runID sql.NullInt64
		var closedAt sql.NullString
		var outcomeMarkedAt sql.NullString
		var cleaningExcluded int
		var cleaningExcludedAt sql.NullString
		if err := rows.Scan(
			&o.ID, &o.PropertyID, &o.SourceType, &o.SourceEventUID, &start, &end, &o.Status,
			&o.RawSummary, &o.GuestDisplayName, &o.ContentHash, &imp, &last, &runID,
			&o.ClosureState, &o.ClosureReason, &o.ClosureCategory, &o.ClosedByUserID, &closedAt,
			&o.ExternalNetAmountCents, &o.ExternalCurrency, &o.ExternalChannel, &o.FinanceBookingID,
			&o.StayOutcome, &o.StayOutcomeReason, &o.StayOutcomeMarkedByUserID, &outcomeMarkedAt,
			&cleaningExcluded, &o.CleaningCalendarExclusionReason, &o.CleaningCalendarExcludedByUserID, &cleaningExcludedAt,
		); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imp)
		o.LastSyncedAt, _ = time.Parse(time.RFC3339, last)
		o.LastSyncRunID = runID
		if closedAt.Valid && closedAt.String != "" {
			t, _ := time.Parse(time.RFC3339, closedAt.String)
			o.ClosedAt = sql.NullTime{Time: t, Valid: true}
		}
		if outcomeMarkedAt.Valid && outcomeMarkedAt.String != "" {
			t, _ := time.Parse(time.RFC3339, outcomeMarkedAt.String)
			o.StayOutcomeMarkedAt = sql.NullTime{Time: t, Valid: true}
		}
		o.CleaningCalendarExcluded = cleaningExcluded == 1
		if cleaningExcludedAt.Valid && cleaningExcludedAt.String != "" {
			t, _ := time.Parse(time.RFC3339, cleaningExcludedAt.String)
			o.CleaningCalendarExcludedAt = sql.NullTime{Time: t, Valid: true}
		}
		out = append(out, o)
	}
	return out, rows.Err()
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
		SELECT id, property_id, started_at, finished_at, status, error_message, events_seen, occupancies_upserted, http_status, trigger, created_at
		FROM occupancy_sync_runs WHERE property_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`, propertyID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancySyncRun
	for rows.Next() {
		var r OccupancySyncRun
		var started, created string
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &r.PropertyID, &started, &finished, &r.Status, &r.ErrorMessage, &r.EventsSeen, &r.OccupanciesUpserted, &r.HTTPStatus, &r.Trigger, &created); err != nil {
			return nil, err
		}
		r.StartedAt, _ = time.Parse(time.RFC3339, started)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if finished.Valid && finished.String != "" {
			t, _ := time.Parse(time.RFC3339, finished.String)
			r.FinishedAt = sql.NullTime{Time: t, Valid: true}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateOccupancyAPIToken(ctx context.Context, propertyID int64, tokenHash string, label *string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var lab interface{}
	if label != nil {
		lab = *label
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO occupancy_api_tokens (property_id, token_hash, label, created_at) VALUES (?, ?, ?, ?)`,
		propertyID, tokenHash, lab, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListOccupancyAPITokens(ctx context.Context, propertyID int64) ([]OccupancyAPIToken, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, label, created_at, last_used_at FROM occupancy_api_tokens WHERE property_id = ? ORDER BY created_at DESC`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyAPIToken
	for rows.Next() {
		var t OccupancyAPIToken
		var created string
		var last sql.NullString
		if err := rows.Scan(&t.ID, &t.PropertyID, &t.Label, &created, &last); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if last.Valid && last.String != "" {
			tt, _ := time.Parse(time.RFC3339, last.String)
			t.LastUsedAt = sql.NullTime{Time: tt, Valid: true}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOccupancyAPIToken(ctx context.Context, id, propertyID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM occupancy_api_tokens WHERE id = ? AND property_id = ?`, id, propertyID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ValidateOccupancyExportToken(ctx context.Context, propertyID int64, tokenHash string) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM occupancy_api_tokens WHERE property_id = ? AND token_hash = ?`, propertyID, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.DB.ExecContext(ctx, `UPDATE occupancy_api_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	return true, nil
}

func (s *Store) ListOccupanciesForExport(ctx context.Context, propertyID int64) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies WHERE property_id = ? AND status NOT IN ('deleted_from_source', 'cancelled')
		ORDER BY start_at ASC`
	return s.scanOccupancies(ctx, q, propertyID)
}

func (s *Store) ListUpcomingOccupancies(ctx context.Context, propertyID int64, limit int) ([]UpcomingOccupancy, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, source_event_uid, raw_summary, guest_display_name, start_at, end_at, status
		FROM occupancies
		WHERE property_id = ?
		  AND status IN ('active', 'updated')
		  AND end_at >= ?
		ORDER BY start_at ASC
		LIMIT ?`,
		propertyID, time.Now().UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UpcomingOccupancy, 0)
	for rows.Next() {
		var row UpcomingOccupancy
		var start, end string
		if err := rows.Scan(&row.ID, &row.SourceEventUID, &row.RawSummary, &row.GuestDisplayName, &start, &end, &row.Status); err != nil {
			return nil, err
		}
		row.StartAt, _ = time.Parse(time.RFC3339, start)
		row.EndAt, _ = time.Parse(time.RFC3339, end)
		out = append(out, row)
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

// ErrOccupancyAlreadyLabelled indicates that the row already has a closure /
// external-sale label and the caller must explicitly Reopen first. Operators
// should never silently overwrite a label.
var ErrOccupancyAlreadyLabelled = errors.New("occupancy already has a manual label; clear it first")

// ErrInvalidOccupancySplit indicates that a requested split range does not map
// cleanly to whole nights inside the occupancy stay window.
var ErrInvalidOccupancySplit = errors.New("invalid occupancy split range")

var ErrInvalidStayOutcome = errors.New("invalid stay outcome")

var ErrOccupancyOutcomeConflict = errors.New("occupancy already has a stay outcome; clear it first")

var ErrOccupancyOutcomeIneligible = errors.New("occupancy is not eligible for a stay outcome")

var ErrOccupancyCleaningCalendarExclusionIneligible = errors.New("occupancy is not eligible for cleaning calendar exclusion")

// CloseOccupancy marks the row as off-the-market closed. Sets closed_by_user_id
// + closed_at audit columns and clears any external-sale fields. Refuses to
// overwrite an existing label.
func (s *Store) CloseOccupancy(ctx context.Context, propertyID, occupancyID, userID int64, reason, category string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET closure_state = 'closed',
		    closure_reason = ?,
		    closure_category = ?,
		    closed_by_user_id = ?,
		    closed_at = ?,
		    external_net_amount_cents = NULL,
		    external_currency = NULL,
		    external_channel = NULL,
		    last_synced_at = ?
		WHERE property_id = ? AND id = ? AND closure_state IS NULL AND stay_outcome IS NULL`,
		nullableString(reason), nullableString(category), userID, now, now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("close occupancy: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return s.checkOccupancyLabelled(ctx, propertyID, occupancyID)
	}
	return nil
}

func (s *Store) CloseOccupancyNight(ctx context.Context, propertyID, occupancyID, userID int64, nightDate, reason, category string) error {
	row, err := s.GetOccupancyByID(ctx, propertyID, occupancyID)
	if err != nil {
		return err
	}
	if row.ClosureState.Valid || row.StayOutcome.Valid {
		return ErrOccupancyAlreadyLabelled
	}
	night, err := time.Parse("2006-01-02", strings.TrimSpace(nightDate))
	if err != nil {
		return fmt.Errorf("invalid night")
	}
	night = time.Date(night.Year(), night.Month(), night.Day(), 0, 0, 0, 0, time.UTC)
	nightEnd := night.AddDate(0, 0, 1)
	start := toUTCMidnight(row.StartAt)
	end := toUTCMidnight(row.EndAt)
	if night.Before(start) || !nightEnd.After(start) || nightEnd.After(end) {
		return sql.ErrNoRows
	}
	if start.Equal(night) && end.Equal(nightEnd) {
		return s.CloseOccupancy(ctx, propertyID, occupancyID, userID, reason, category)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE occupancies
		SET status = 'deleted_from_source', last_synced_at = ?
		WHERE property_id = ? AND id = ?`, now, propertyID, occupancyID); err != nil {
		return err
	}
	baseUID := row.SourceEventUID
	if start.Before(night) {
		if err := s.insertManualSplitOccupancyTx(ctx, tx, row, baseUID, "before", start, night, nil, nil, userID, now); err != nil {
			return err
		}
	}
	if err := s.insertManualSplitOccupancyTx(ctx, tx, row, baseUID, "closed", night, nightEnd, &reason, &category, userID, now); err != nil {
		return err
	}
	if nightEnd.Before(end) {
		if err := s.insertManualSplitOccupancyTx(ctx, tx, row, baseUID, "after", nightEnd, end, nil, nil, userID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SplitOccupancyIntoNights(ctx context.Context, propertyID, occupancyID int64) error {
	return s.SplitOccupancyIntoNightRange(ctx, propertyID, occupancyID, "", "")
}

func (s *Store) SplitOccupancyIntoNightRange(ctx context.Context, propertyID, occupancyID int64, startDate, endDate string) error {
	row, err := s.GetOccupancyByID(ctx, propertyID, occupancyID)
	if err != nil {
		return err
	}
	if row.ClosureState.Valid || row.StayOutcome.Valid {
		return ErrOccupancyAlreadyLabelled
	}
	start := toUTCMidnight(row.StartAt)
	end := toUTCMidnight(row.EndAt)
	days := int(end.Sub(start).Hours() / 24)
	if days <= 1 || !start.AddDate(0, 0, days).Equal(end) {
		return ErrInvalidOccupancySplit
	}
	splitStart := start
	splitEnd := end
	startDate = strings.TrimSpace(startDate)
	endDate = strings.TrimSpace(endDate)
	if startDate != "" || endDate != "" {
		if startDate == "" || endDate == "" {
			return ErrInvalidOccupancySplit
		}
		splitStart, err = parseOccupancySplitDate(startDate)
		if err != nil {
			return err
		}
		splitEnd, err = parseOccupancySplitDate(endDate)
		if err != nil {
			return err
		}
		if splitStart.Before(start) || splitEnd.After(end) || !splitEnd.After(splitStart) {
			return ErrInvalidOccupancySplit
		}
	}
	splitDays := int(splitEnd.Sub(splitStart).Hours() / 24)
	if splitDays <= 0 || !splitStart.AddDate(0, 0, splitDays).Equal(splitEnd) {
		return ErrInvalidOccupancySplit
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE occupancies
		SET status = 'deleted_from_source', last_synced_at = ?
		WHERE property_id = ? AND id = ?`, now, propertyID, occupancyID); err != nil {
		return err
	}
	if start.Before(splitStart) {
		if err := s.insertManualSplitOccupancyTx(ctx, tx, row, row.SourceEventUID, "before", start, splitStart, nil, nil, 0, now); err != nil {
			return err
		}
	}
	for i := 0; i < splitDays; i++ {
		night := splitStart.AddDate(0, 0, i)
		if err := s.insertManualSplitOccupancyTx(ctx, tx, row, row.SourceEventUID, "night", night, night.AddDate(0, 0, 1), nil, nil, 0, now); err != nil {
			return err
		}
	}
	if splitEnd.Before(end) {
		if err := s.insertManualSplitOccupancyTx(ctx, tx, row, row.SourceEventUID, "after", splitEnd, end, nil, nil, 0, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func parseOccupancySplitDate(v string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, ErrInvalidOccupancySplit
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), nil
}

func (s *Store) HasManualSplitForSourceEventUID(ctx context.Context, propertyID int64, sourceUID string) (bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT id
		FROM occupancies
		WHERE property_id = ?
		  AND source_type = ?
		  AND source_event_uid LIKE ?
		  AND status IN ('active', 'updated')
		LIMIT 1`, propertyID, manualSplitSourceType, manualSplitUIDPrefix+sourceUID+":%").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) insertManualSplitOccupancyTx(ctx context.Context, tx *sql.Tx, base *Occupancy, baseUID, role string, start, end time.Time, reason, category *string, userID int64, now string) error {
	uid := fmt.Sprintf("%s%s:%s:%s", manualSplitUIDPrefix, baseUID, role, start.Format("20060102"))
	status := "active"
	closureState := interface{}(nil)
	closureReason := interface{}(nil)
	closureCategory := interface{}(nil)
	closedBy := interface{}(nil)
	closedAt := interface{}(nil)
	if role == "closed" {
		closureState = ClosureStateClosed
		closureReason = nullableString(ptrStringValue(reason))
		closureCategory = nullableString(ptrStringValue(category))
		closedBy = userID
		closedAt = now
	}
	var summary interface{}
	if base.RawSummary.Valid {
		summary = base.RawSummary.String
	}
	var guest interface{}
	if base.GuestDisplayName.Valid {
		guest = base.GuestDisplayName.String
	}
	contentHash := fmt.Sprintf("manual-split:%d:%s:%s:%s", base.ID, role, start.Format(time.RFC3339), end.Format(time.RFC3339))
	_, err := tx.ExecContext(ctx, `
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id,
			closure_state, closure_reason, closure_category, closed_by_user_id, closed_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			status = excluded.status,
			raw_summary = excluded.raw_summary,
			guest_display_name = excluded.guest_display_name,
			content_hash = excluded.content_hash,
			last_synced_at = excluded.last_synced_at,
			closure_state = excluded.closure_state,
			closure_reason = excluded.closure_reason,
			closure_category = excluded.closure_category,
			closed_by_user_id = excluded.closed_by_user_id,
			closed_at = excluded.closed_at`,
		base.PropertyID, manualSplitSourceType, uid, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), status,
		summary, guest, contentHash, now, now, closureState, closureReason, closureCategory, closedBy, closedAt)
	return err
}

func ptrStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func toUTCMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// MarkOccupancyExternalSale labels the row as sold via a different channel. The
// operator-entered net amount feeds gross_revenue but does not change the
// nights_sold / available_nights tally. Refuses to overwrite an existing label.
func (s *Store) MarkOccupancyExternalSale(ctx context.Context, propertyID, occupancyID, userID, netAmountCents int64, currency, channel, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET closure_state = 'external_sale',
		    closure_reason = ?,
		    closure_category = NULL,
		    closed_by_user_id = ?,
		    closed_at = ?,
		    external_net_amount_cents = ?,
		    external_currency = ?,
		    external_channel = ?,
		    last_synced_at = ?
		WHERE property_id = ? AND id = ? AND closure_state IS NULL AND stay_outcome IS NULL`,
		nullableString(reason), userID, now, netAmountCents, nullableString(currency), nullableString(channel), now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("mark external sale: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return s.checkOccupancyLabelled(ctx, propertyID, occupancyID)
	}
	return nil
}

// ReopenOccupancy removes any closure / external-sale label, restoring the row
// to its original ICS-driven status. No-op (returns sql.ErrNoRows) if the row
// is already unlabelled.
func (s *Store) ReopenOccupancy(ctx context.Context, propertyID, occupancyID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET closure_state = NULL,
		    closure_reason = NULL,
		    closure_category = NULL,
		    closed_by_user_id = NULL,
		    closed_at = NULL,
		    external_net_amount_cents = NULL,
		    external_currency = NULL,
		    external_channel = NULL,
		    last_synced_at = ?
		WHERE property_id = ? AND id = ? AND closure_state IS NOT NULL`,
		now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("reopen occupancy: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Either not found or not labelled; differentiate so the handler can
		// return a clean 404 vs 409.
		_, err := s.GetOccupancyByID(ctx, propertyID, occupancyID)
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return nil
}

// checkOccupancyLabelled is called after a 0-row UPDATE to decide whether the
// caller saw "row missing" (sql.ErrNoRows) or "already labelled"
// (ErrOccupancyAlreadyLabelled).
func (s *Store) checkOccupancyLabelled(ctx context.Context, propertyID, occupancyID int64) error {
	row, err := s.GetOccupancyByID(ctx, propertyID, occupancyID)
	if err != nil {
		return err
	}
	if row.ClosureState.Valid || row.StayOutcome.Valid {
		return ErrOccupancyAlreadyLabelled
	}
	// Should not happen unless a concurrent writer changed the row between
	// the UPDATE and this SELECT; treat as a transient failure.
	return errors.New("occupancy not updated")
}

func validStayOutcome(outcome string) bool {
	switch outcome {
	case StayOutcomeCancelledNonRefundable, StayOutcomeNoShow:
		return true
	default:
		return false
	}
}

func (s *Store) MarkOccupancyStayOutcome(ctx context.Context, propertyID, occupancyID, userID int64, outcome, reason string) error {
	if !validStayOutcome(outcome) {
		return ErrInvalidStayOutcome
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE occupancies
		SET stay_outcome = ?,
		    stay_outcome_reason = ?,
		    stay_outcome_marked_by_user_id = ?,
		    stay_outcome_marked_at = ?,
		    last_synced_at = ?
		WHERE property_id = ?
		  AND id = ?
		  AND source_type = 'booking_ics'
		  AND status IN ('active', 'updated')
		  AND closure_state IS NULL
		  AND (stay_outcome IS NULL OR stay_outcome = ?)`,
		outcome, nullableString(reason), userID, now, now, propertyID, occupancyID, outcome)
	if err != nil {
		return fmt.Errorf("mark stay outcome: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		row, getErr := s.GetOccupancyByID(ctx, propertyID, occupancyID)
		if getErr != nil {
			return getErr
		}
		if row.StayOutcome.Valid && row.StayOutcome.String != outcome {
			return ErrOccupancyOutcomeConflict
		}
		return ErrOccupancyOutcomeIneligible
	}
	if err := syncFinanceOutcomeOverrideTx(ctx, tx, propertyID, occupancyID, outcome, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClearOccupancyStayOutcome(ctx context.Context, propertyID, occupancyID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE occupancies
		SET stay_outcome = NULL,
		    stay_outcome_reason = NULL,
		    stay_outcome_marked_by_user_id = NULL,
		    stay_outcome_marked_at = NULL,
		    last_synced_at = ?
		WHERE property_id = ? AND id = ? AND stay_outcome IS NOT NULL`, now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("clear stay outcome: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, getErr := s.GetOccupancyByID(ctx, propertyID, occupancyID)
		if getErr != nil {
			return getErr
		}
		return sql.ErrNoRows
	}
	if err := syncFinanceOutcomeOverrideTx(ctx, tx, propertyID, occupancyID, "", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOccupancyCleaningCalendarExcluded(ctx context.Context, propertyID, occupancyID, userID int64, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET cleaning_calendar_excluded = 1,
		    cleaning_calendar_exclusion_reason = ?,
		    cleaning_calendar_excluded_by_user_id = ?,
		    cleaning_calendar_excluded_at = ?,
		    last_synced_at = ?
		WHERE property_id = ?
		  AND id = ?
		  AND status IN ('active', 'updated')
		  AND (closure_state IS NULL OR closure_state <> 'closed')
		  AND stay_outcome IS NULL`, nullableString(reason), userID, now, now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("mark cleaning calendar excluded: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, getErr := s.GetOccupancyByID(ctx, propertyID, occupancyID)
		if getErr != nil {
			return getErr
		}
		return ErrOccupancyCleaningCalendarExclusionIneligible
	}
	return nil
}

func (s *Store) ClearOccupancyCleaningCalendarExcluded(ctx context.Context, propertyID, occupancyID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE occupancies
		SET cleaning_calendar_excluded = 0,
		    cleaning_calendar_exclusion_reason = NULL,
		    cleaning_calendar_excluded_by_user_id = NULL,
		    cleaning_calendar_excluded_at = NULL,
		    last_synced_at = ?
		WHERE property_id = ? AND id = ? AND cleaning_calendar_excluded = 1`, now, propertyID, occupancyID)
	if err != nil {
		return fmt.Errorf("clear cleaning calendar excluded: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, getErr := s.GetOccupancyByID(ctx, propertyID, occupancyID)
		return getErr
	}
	return nil
}

func syncFinanceOutcomeOverrideTx(ctx context.Context, tx *sql.Tx, propertyID, occupancyID int64, outcome, markedAt string) error {
	var outcomeValue interface{}
	var markedAtValue interface{}
	if outcome != "" {
		outcomeValue = outcome
		markedAtValue = markedAt
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE finance_bookings
		SET outcome_override = ?, outcome_override_marked_at = ?, updated_at = ?
		WHERE property_id = ?
		  AND (occupancy_id = ? OR id = (SELECT finance_booking_id FROM occupancies WHERE property_id = ? AND id = ?))`,
		outcomeValue, markedAtValue, markedAt, propertyID, occupancyID, propertyID, occupancyID)
	return err
}

func nullableString(v string) interface{} {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}
