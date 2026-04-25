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
}

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
	q := `
		SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id
		FROM occupancies WHERE property_id = ? AND source_event_uid = ?`
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
	q := `
		SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id
		FROM occupancies WHERE property_id = ? AND id = ?`
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
	if len(keepUIDs) == 0 {
		_, err := s.DB.ExecContext(ctx, `
			UPDATE occupancies SET status = 'deleted_from_source', last_synced_at = ?
			WHERE property_id = ? AND source_type = ? AND status NOT IN ('deleted_from_source')
			AND end_at >= ?`,
			now, propertyID, sourceType, now)
		return err
	}
	ph := make([]string, len(keepUIDs))
	args := make([]interface{}, 0, len(keepUIDs)+4)
	args = append(args, now, propertyID, sourceType, now)
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
	q := `
		SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id
		FROM occupancies WHERE property_id = ?`
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
	q := `
		SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id
		FROM occupancies WHERE property_id = ? AND start_at < ? AND end_at > ?
		ORDER BY start_at ASC`
	return s.scanOccupancies(ctx, q, propertyID, endUTC.Format(time.RFC3339), startUTC.Format(time.RFC3339))
}

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
		if err := rows.Scan(&o.ID, &o.PropertyID, &o.SourceType, &o.SourceEventUID, &start, &end, &o.Status, &o.RawSummary, &o.GuestDisplayName, &o.ContentHash, &imp, &last, &runID); err != nil {
			return nil, err
		}
		o.StartAt, _ = time.Parse(time.RFC3339, start)
		o.EndAt, _ = time.Parse(time.RFC3339, end)
		o.ImportedAt, _ = time.Parse(time.RFC3339, imp)
		o.LastSyncedAt, _ = time.Parse(time.RFC3339, last)
		o.LastSyncRunID = runID
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
	q := `
		SELECT id, property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary, guest_display_name, content_hash, imported_at, last_synced_at, last_sync_run_id
		FROM occupancies WHERE property_id = ? AND status NOT IN ('deleted_from_source', 'cancelled')
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
