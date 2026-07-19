package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type NukiSyncRun struct {
	ID             int64
	PropertyID     int64
	StartedAt      time.Time
	FinishedAt     sql.NullTime
	Status         string
	Trigger        string
	ErrorMessage   sql.NullString
	ProcessedCount int
	CreatedCount   int
	UpdatedCount   int
	RevokedCount   int
	FailedCount    int
	CreatedAt      time.Time
}

type NukiAccessCode struct {
	ID                int64
	PropertyID        int64
	OccupancyID       sql.NullInt64
	NamedStayID       sql.NullInt64
	CodeLabel         string
	AccessCodeMasked  sql.NullString
	GeneratedPINPlain sql.NullString
	ExternalNukiID    sql.NullString
	ValidFrom         time.Time
	ValidUntil        time.Time
	Status            string
	ErrorMessage      sql.NullString
	LastSyncRunID     sql.NullInt64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RevokedAt         sql.NullTime
}

type NukiAccessCodeWithOccupancy struct {
	Code             NukiAccessCode
	OccupancyUID     string
	OccupancySummary sql.NullString
	OccupancyStatus  string
	OccupancyStart   time.Time
	OccupancyEnd     time.Time
}

type NukiStay struct {
	NamedStayID       int64
	PropertyID        int64
	LegacyOccupancyID sql.NullInt64
	DisplayName       string
	StayType          string
	ReviewStatus      string
	CheckInDate       string
	CheckOutDate      string
	Status            string
	SourceReference   sql.NullString
}

type NukiKeypadCode struct {
	ID               int64
	PropertyID       int64
	ExternalNukiID   string
	Name             sql.NullString
	AccessCodeMasked sql.NullString
	ValidFrom        sql.NullTime
	ValidUntil       sql.NullTime
	Enabled          bool
	PMSLinked        bool
	RawJSON          sql.NullString
	LastSeenAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UpcomingStayWithCode struct {
	StayID              int64
	LegacyOccupancyID   sql.NullInt64
	OccupancyID         sql.NullInt64
	SourceEventUID      string
	RawSummary          sql.NullString
	GuestDisplayName    sql.NullString
	StayType            string
	StartAt             time.Time
	EndAt               time.Time
	OccupancyStatus     string
	GeneratedCodeID     sql.NullInt64
	GeneratedLabel      sql.NullString
	GeneratedStatus     sql.NullString
	GeneratedMasked     sql.NullString
	GeneratedPIN        sql.NullString
	GeneratedValidFrom  sql.NullTime
	GeneratedValidUntil sql.NullTime
	GeneratedError      sql.NullString
	GeneratedUpdated    sql.NullTime
}

func (s *Store) StartNukiSyncRun(ctx context.Context, propertyID int64, trigger string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO nuki_sync_runs (property_id, started_at, status, trigger, created_at)
		VALUES (?, ?, 'running', ?, ?)`, propertyID, now, trigger, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishNukiSyncRun(ctx context.Context, runID int64, status string, errMsg *string, processed, createdN, updatedN, revokedN, failedN int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var em interface{}
	if errMsg != nil {
		em = *errMsg
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_sync_runs
		SET finished_at = ?, status = ?, error_message = ?, processed_count = ?, created_count = ?, updated_count = ?, revoked_count = ?, failed_count = ?
		WHERE id = ?`,
		now, status, em, processed, createdN, updatedN, revokedN, failedN, runID)
	return err
}

func (s *Store) ListNukiSyncRuns(ctx context.Context, propertyID int64, limit, offset int) ([]NukiSyncRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, started_at, finished_at, status, trigger, error_message, processed_count, created_count, updated_count, revoked_count, failed_count, created_at
		FROM nuki_sync_runs WHERE property_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`, propertyID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiSyncRun
	for rows.Next() {
		var r NukiSyncRun
		var started, created string
		var finished sql.NullString
		if err := rows.Scan(&r.ID, &r.PropertyID, &started, &finished, &r.Status, &r.Trigger, &r.ErrorMessage, &r.ProcessedCount, &r.CreatedCount, &r.UpdatedCount, &r.RevokedCount, &r.FailedCount, &created); err != nil {
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

func (s *Store) PruneNukiSyncRuns(ctx context.Context, propertyID int64, keep int) error {
	if keep < 0 {
		keep = 0
	}
	if keep == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM nuki_sync_runs WHERE property_id = ?`, propertyID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM nuki_sync_runs
		WHERE property_id = ?
		  AND id NOT IN (
		      SELECT id FROM nuki_sync_runs
		      WHERE property_id = ?
		      ORDER BY started_at DESC
		      LIMIT ?
		  )`,
		propertyID, propertyID, keep)
	return err
}

func (s *Store) GetNukiCodeByOccupancyID(ctx context.Context, propertyID, occupancyID int64) (*NukiAccessCode, error) {
	rows, err := s.scanNukiCodes(ctx, `
		SELECT id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
		FROM nuki_access_codes WHERE property_id = ? AND occupancy_id = ?`, propertyID, occupancyID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) GetNukiCodeByNamedStayID(ctx context.Context, propertyID, stayID int64) (*NukiAccessCode, error) {
	rows, err := s.scanNukiCodes(ctx, `
		SELECT id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
		FROM nuki_access_codes WHERE property_id = ? AND named_stay_id = ?`, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) GetNukiCodeByID(ctx context.Context, propertyID, codeID int64) (*NukiAccessCode, error) {
	rows, err := s.scanNukiCodes(ctx, `
		SELECT id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
		FROM nuki_access_codes WHERE property_id = ? AND id = ?`, propertyID, codeID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) UpsertNukiCode(ctx context.Context, c *NukiAccessCode) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var masked interface{}
	if c.AccessCodeMasked.Valid {
		masked = c.AccessCodeMasked.String
	}
	var pinPlain interface{}
	if c.GeneratedPINPlain.Valid {
		enc, err := s.Crypto.Encrypt(c.GeneratedPINPlain.String)
		if err != nil {
			return err
		}
		pinPlain = enc
	}
	var ext interface{}
	if c.ExternalNukiID.Valid {
		ext = c.ExternalNukiID.String
	}
	var errMsg interface{}
	if c.ErrorMessage.Valid {
		errMsg = c.ErrorMessage.String
	}
	var runID interface{}
	if c.LastSyncRunID.Valid {
		runID = c.LastSyncRunID.Int64
	}
	var stayID interface{}
	if c.NamedStayID.Valid {
		stayID = c.NamedStayID.Int64
	}
	var occupancyID interface{}
	if c.OccupancyID.Valid {
		occupancyID = c.OccupancyID.Int64
	}
	var revoked interface{}
	if c.RevokedAt.Valid {
		revoked = c.RevokedAt.Time.UTC().Format(time.RFC3339)
	}
	args := []interface{}{c.PropertyID, occupancyID, stayID, c.CodeLabel, masked, pinPlain, ext,
		c.ValidFrom.UTC().Format(time.RFC3339), c.ValidUntil.UTC().Format(time.RFC3339), c.Status, errMsg, runID, now, now, revoked}
	setSQL := `occupancy_id = COALESCE(excluded.occupancy_id, nuki_access_codes.occupancy_id),
			named_stay_id = COALESCE(excluded.named_stay_id, nuki_access_codes.named_stay_id),
			code_label = excluded.code_label, access_code_masked = excluded.access_code_masked,
			generated_pin_plain = excluded.generated_pin_plain, external_nuki_id = excluded.external_nuki_id,
			valid_from = excluded.valid_from, valid_until = excluded.valid_until, status = excluded.status,
			error_message = excluded.error_message, last_sync_run_id = excluded.last_sync_run_id,
			updated_at = excluded.updated_at, revoked_at = excluded.revoked_at`
	if c.ID > 0 {
		_, err := s.DB.ExecContext(ctx, `
			UPDATE nuki_access_codes SET occupancy_id = ?, named_stay_id = ?, code_label = ?, access_code_masked = ?,
				generated_pin_plain = ?, external_nuki_id = ?, valid_from = ?, valid_until = ?, status = ?,
				error_message = ?, last_sync_run_id = ?, updated_at = ?, revoked_at = ?
			WHERE property_id = ? AND id = ?`, occupancyID, stayID, c.CodeLabel, masked, pinPlain, ext,
			c.ValidFrom.UTC().Format(time.RFC3339), c.ValidUntil.UTC().Format(time.RFC3339), c.Status, errMsg, runID, now, revoked, c.PropertyID, c.ID)
		return err
	}
	base := `INSERT INTO nuki_access_codes (property_id, occupancy_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	var query string
	if c.NamedStayID.Valid {
		query = base + ` ON CONFLICT(property_id, named_stay_id) WHERE named_stay_id IS NOT NULL DO UPDATE SET ` + setSQL
	} else {
		if !c.OccupancyID.Valid {
			return errors.New("nuki code requires named_stay_id or occupancy_id")
		}
		query = base + ` ON CONFLICT(property_id, occupancy_id) WHERE named_stay_id IS NULL AND occupancy_id IS NOT NULL DO UPDATE SET ` + setSQL
	}
	_, err := s.DB.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) ListNukiCodes(ctx context.Context, propertyID int64, scope string) ([]NukiAccessCodeWithOccupancy, error) {
	q := `
		SELECT nac.id, nac.property_id, nac.occupancy_id, nac.named_stay_id, nac.code_label, nac.access_code_masked, nac.generated_pin_plain, nac.external_nuki_id, nac.valid_from, nac.valid_until, nac.status, nac.error_message, nac.last_sync_run_id, nac.created_at, nac.updated_at, nac.revoked_at,
		       COALESCE(ns.source_reference, o.source_event_uid, ''),
		       COALESCE(ns.display_name, o.raw_summary), COALESCE(ns.status, o.status, ''),
		       COALESCE(ns.check_in_date, o.start_at), COALESCE(ns.check_out_date, o.end_at)
		FROM nuki_access_codes nac
		LEFT JOIN named_stays ns ON ns.id = nac.named_stay_id AND ns.property_id = nac.property_id
		LEFT JOIN occupancies o ON o.id = nac.occupancy_id AND o.property_id = nac.property_id
		WHERE nac.property_id = ?`
	switch scope {
	case "active":
		q += ` AND nac.status IN ('generated', 'not_generated')`
	case "historical":
		q += ` AND nac.status = 'revoked'`
	}
	q += ` ORDER BY nac.valid_from DESC`
	rows, err := s.DB.QueryContext(ctx, q, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiAccessCodeWithOccupancy
	for rows.Next() {
		var r NukiAccessCodeWithOccupancy
		var validFrom, validUntil, created, updated string
		var revoked sql.NullString
		var oStart, oEnd string
		if err := rows.Scan(
			&r.Code.ID, &r.Code.PropertyID, &r.Code.OccupancyID, &r.Code.NamedStayID, &r.Code.CodeLabel, &r.Code.AccessCodeMasked, &r.Code.GeneratedPINPlain, &r.Code.ExternalNukiID,
			&validFrom, &validUntil, &r.Code.Status, &r.Code.ErrorMessage, &r.Code.LastSyncRunID, &created, &updated, &revoked,
			&r.OccupancyUID, &r.OccupancySummary, &r.OccupancyStatus, &oStart, &oEnd,
		); err != nil {
			return nil, err
		}
		if err := s.decryptNS(&r.Code.GeneratedPINPlain); err != nil {
			return nil, err
		}
		r.Code.ValidFrom, _ = time.Parse(time.RFC3339, validFrom)
		r.Code.ValidUntil, _ = time.Parse(time.RFC3339, validUntil)
		r.Code.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.Code.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if revoked.Valid && revoked.String != "" {
			t, _ := time.Parse(time.RFC3339, revoked.String)
			r.Code.RevokedAt = sql.NullTime{Time: t, Valid: true}
		}
		r.OccupancyStart, _ = time.Parse(time.RFC3339, oStart)
		r.OccupancyEnd, _ = time.Parse(time.RFC3339, oEnd)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) scanNukiCodes(ctx context.Context, q string, args ...interface{}) ([]NukiAccessCode, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiAccessCode
	for rows.Next() {
		var c NukiAccessCode
		var validFrom, validUntil, created, updated string
		var revoked sql.NullString
		if err := rows.Scan(&c.ID, &c.PropertyID, &c.OccupancyID, &c.NamedStayID, &c.CodeLabel, &c.AccessCodeMasked, &c.GeneratedPINPlain, &c.ExternalNukiID, &validFrom, &validUntil, &c.Status, &c.ErrorMessage, &c.LastSyncRunID, &created, &updated, &revoked); err != nil {
			return nil, err
		}
		if err := s.decryptNS(&c.GeneratedPINPlain); err != nil {
			return nil, err
		}
		c.ValidFrom, _ = time.Parse(time.RFC3339, validFrom)
		c.ValidUntil, _ = time.Parse(time.RFC3339, validUntil)
		c.CreatedAt, _ = time.Parse(time.RFC3339, created)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if revoked.Valid && revoked.String != "" {
			t, _ := time.Parse(time.RFC3339, revoked.String)
			c.RevokedAt = sql.NullTime{Time: t, Valid: true}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListOccupanciesForNukiSync(ctx context.Context, propertyID int64) ([]Occupancy, error) {
	// Closure-labelled rows are excluded from Nuki sync — closed nights have
	// no guest, externally-sold nights have a guest who arrives outside the
	// Booking.com flow (PMS_14 §3.4).
	//
	// PMS_19 §10.1 / §5.6: Nuki codes are only for active, non-superseded named
	// stays. Unnamed Booking.com block nights (no guest name) must never receive
	// a guest code, and superseded representations are ineligible.
	q := occupancySelectColumns + ` FROM occupancies
		WHERE property_id = ? AND status IN ('active', 'updated') AND closure_state IS NULL
		AND superseded_at IS NULL
		AND guest_display_name IS NOT NULL AND TRIM(guest_display_name) <> ''
		AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show')) AND end_at >= ?
		ORDER BY start_at ASC`
	return s.scanOccupancies(ctx, q, propertyID, time.Now().UTC().Format(time.RFC3339))
}

func (s *Store) ListOccupanciesForNukiRevocation(ctx context.Context, propertyID int64) ([]Occupancy, error) {
	q := occupancySelectColumns + ` FROM occupancies
		WHERE property_id = ? AND (status IN ('cancelled', 'deleted_from_source') OR stay_outcome IN ('cancelled_non_refundable', 'no_show') OR superseded_at IS NOT NULL)
		ORDER BY start_at ASC`
	return s.scanOccupancies(ctx, q, propertyID)
}

func (s *Store) ListNamedStaysForNukiSync(ctx context.Context, propertyID int64) ([]NukiStay, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, osm.old_occupancy_id, ns.display_name, ns.stay_type,
		       COALESCE(ns.review_status, 'confirmed'), ns.check_in_date, ns.check_out_date,
		       ns.status, ns.source_reference
		FROM named_stays ns
		LEFT JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id AND osm.migration_kind = 'named_stay'
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.stay_type IN ('booking_com', 'external')
		  AND (ns.stay_outcome IS NULL OR ns.stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		  AND ns.check_out_date >= ?
		ORDER BY ns.check_in_date ASC, ns.id ASC`, propertyID, time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	return scanNukiStays(rows)
}

func (s *Store) ListNamedStaysForNukiRevocation(ctx context.Context, propertyID int64) ([]NukiStay, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, osm.old_occupancy_id, ns.display_name, ns.stay_type,
		       COALESCE(ns.review_status, 'confirmed'), ns.check_in_date, ns.check_out_date,
		       ns.status, ns.source_reference
		FROM named_stays ns
		LEFT JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id AND osm.migration_kind = 'named_stay'
		WHERE ns.property_id = ?
		  AND (
		      ns.status IN ('cancelled', 'archived')
		      OR COALESCE(ns.review_status, 'confirmed') <> 'confirmed'
		      OR ns.stay_type NOT IN ('booking_com', 'external')
		      OR ns.stay_outcome IN ('cancelled_non_refundable', 'no_show')
		  )
		ORDER BY ns.check_in_date ASC, ns.id ASC`, propertyID)
	if err != nil {
		return nil, err
	}
	return scanNukiStays(rows)
}

func scanNukiStays(rows *sql.Rows) ([]NukiStay, error) {
	defer rows.Close()
	out := []NukiStay{}
	for rows.Next() {
		var r NukiStay
		if err := rows.Scan(&r.NamedStayID, &r.PropertyID, &r.LegacyOccupancyID, &r.DisplayName, &r.StayType, &r.ReviewStatus, &r.CheckInDate, &r.CheckOutDate, &r.Status, &r.SourceReference); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListNukiCodesForCleanup(ctx context.Context, propertyID int64, nowUTC time.Time) ([]NukiAccessCode, error) {
	q := `
		SELECT id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
		FROM nuki_access_codes
		WHERE property_id = ? AND status = 'generated' AND valid_until < ?
		ORDER BY valid_until ASC`
	return s.scanNukiCodes(ctx, q, propertyID, nowUTC.UTC().Format(time.RFC3339))
}

func (s *Store) InsertNukiEventLog(ctx context.Context, propertyID int64, codeID *int64, runID *int64, eventType, message, payloadJSON string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var cid interface{}
	if codeID != nil {
		cid = *codeID
	}
	var rid interface{}
	if runID != nil {
		rid = *runID
	}
	var payload interface{}
	if payloadJSON != "" {
		payload = payloadJSON
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO nuki_event_logs (property_id, nuki_access_code_id, sync_run_id, event_type, message, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		propertyID, cid, rid, eventType, message, payload, now)
	return err
}

func (s *Store) ListPropertyIDsWithNukiConfig(ctx context.Context) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id FROM properties p
		INNER JOIN property_secrets ps ON ps.property_id = p.id
		WHERE p.active = 1
		  AND ps.nuki_api_token IS NOT NULL AND TRIM(ps.nuki_api_token) != ''
		  AND ps.nuki_smartlock_id IS NOT NULL AND TRIM(ps.nuki_smartlock_id) != ''`)
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

func (s *Store) DeleteNukiCodeByID(ctx context.Context, propertyID, codeID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM nuki_access_codes WHERE property_id = ? AND id = ?`, propertyID, codeID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MustGetNukiCodeByID(ctx context.Context, propertyID, codeID int64) (*NukiAccessCode, error) {
	code, err := s.GetNukiCodeByID(ctx, propertyID, codeID)
	if err != nil {
		return nil, err
	}
	if code == nil {
		return nil, sql.ErrNoRows
	}
	return code, nil
}

func (s *Store) IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func (s *Store) UpsertNukiKeypadCode(ctx context.Context, row *NukiKeypadCode) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var name interface{}
	if row.Name.Valid {
		name = row.Name.String
	}
	var masked interface{}
	if row.AccessCodeMasked.Valid {
		masked = row.AccessCodeMasked.String
	}
	var from interface{}
	if row.ValidFrom.Valid {
		from = row.ValidFrom.Time.UTC().Format(time.RFC3339)
	}
	var until interface{}
	if row.ValidUntil.Valid {
		until = row.ValidUntil.Time.UTC().Format(time.RFC3339)
	}
	var raw interface{}
	if row.RawJSON.Valid {
		raw = row.RawJSON.String
	}
	enabled := 0
	if row.Enabled {
		enabled = 1
	}
	lastSeen := now
	if !row.LastSeenAt.IsZero() {
		lastSeen = row.LastSeenAt.UTC().Format(time.RFC3339)
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO nuki_keypad_codes (property_id, external_nuki_id, name, access_code_masked, valid_from, valid_until, enabled, raw_json, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, external_nuki_id) DO UPDATE SET
			name = excluded.name,
			access_code_masked = excluded.access_code_masked,
			valid_from = excluded.valid_from,
			valid_until = excluded.valid_until,
			enabled = excluded.enabled,
			raw_json = excluded.raw_json,
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at`,
		row.PropertyID, row.ExternalNukiID, name, masked, from, until, enabled, raw, lastSeen, now, now)
	return err
}

func (s *Store) NormalizeNukiKeypadWindows(ctx context.Context, propertyID int64) error {
	// Repair pathological cached rows from older API parsing variants
	// (e.g. unix epoch 0 shown as 1970, or until earlier than from).
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_keypad_codes
		SET valid_from = NULL,
		    valid_until = NULL,
		    updated_at = ?
		WHERE property_id = ?
		  AND (
		      (valid_until IS NOT NULL AND strftime('%s', valid_until) <= 0)
		      OR (valid_from IS NOT NULL AND strftime('%s', valid_from) <= 0)
		      OR (
		          valid_from IS NOT NULL
		          AND valid_until IS NOT NULL
		          AND strftime('%s', valid_until) < strftime('%s', valid_from)
		      )
		  )`,
		time.Now().UTC().Format(time.RFC3339), propertyID)
	return err
}

func (s *Store) DeleteMissingNukiKeypadCodes(ctx context.Context, propertyID int64, keepExternalIDs []string) error {
	if len(keepExternalIDs) == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM nuki_keypad_codes WHERE property_id = ?`, propertyID)
		return err
	}
	ph := make([]string, len(keepExternalIDs))
	args := make([]interface{}, 0, len(keepExternalIDs)+1)
	args = append(args, propertyID)
	for i, id := range keepExternalIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	q := `DELETE FROM nuki_keypad_codes WHERE property_id = ? AND external_nuki_id NOT IN (` + strings.Join(ph, ",") + `)`
	_, err := s.DB.ExecContext(ctx, q, args...)
	return err
}

func nukiLabelWindowLinkPredicate(accessAlias, keypadAlias string) string {
	return fmt.Sprintf(`
		COALESCE(TRIM(%[1]s.code_label), '') != ''
		AND COALESCE(TRIM(%[2]s.name), '') != ''
		AND LOWER(TRIM(%[1]s.code_label)) = LOWER(TRIM(%[2]s.name))
		AND (
			(
				%[1]s.valid_from IS NOT NULL
				AND %[1]s.valid_until IS NOT NULL
				AND %[2]s.valid_from IS NOT NULL
				AND %[2]s.valid_until IS NOT NULL
				AND (
					(strftime('%%s', %[1]s.valid_from) < strftime('%%s', %[2]s.valid_until)
					 AND strftime('%%s', %[1]s.valid_until) > strftime('%%s', %[2]s.valid_from))
					OR
					(date(%[1]s.valid_from) = date(%[2]s.valid_from)
					 AND date(%[1]s.valid_until) = date(%[2]s.valid_until))
				)
			)
			OR LOWER(TRIM(%[1]s.code_label)) LIKE 'booking-%%'
		)`, accessAlias, keypadAlias)
}

func (s *Store) ListNukiKeypadCodes(ctx context.Context, propertyID int64) ([]NukiKeypadCode, error) {
	query := `
		SELECT kc.id, kc.property_id, kc.external_nuki_id, kc.name, kc.access_code_masked, kc.valid_from, kc.valid_until, kc.enabled,
		       EXISTS(
		           SELECT 1 FROM nuki_access_codes nac
		           WHERE nac.property_id = kc.property_id
		             AND nac.status = 'generated'
		             AND (
		                 nac.external_nuki_id = kc.external_nuki_id
		                 OR (` + nukiLabelWindowLinkPredicate("nac", "kc") + `)
		             )
		       ) AS pms_linked,
		       kc.raw_json, kc.last_seen_at, kc.created_at, kc.updated_at
		FROM nuki_keypad_codes kc
		WHERE kc.property_id = ?
		ORDER BY kc.enabled DESC, COALESCE(kc.valid_until, '9999-12-31T00:00:00Z') ASC, kc.updated_at DESC`
	rows, err := s.DB.QueryContext(ctx, query, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiKeypadCode
	for rows.Next() {
		var r NukiKeypadCode
		var validFrom, validUntil sql.NullString
		var enabled int
		var linked int
		var lastSeen, created, updated string
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.ExternalNukiID, &r.Name, &r.AccessCodeMasked, &validFrom, &validUntil, &enabled, &linked, &r.RawJSON, &lastSeen, &created, &updated); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		r.PMSLinked = linked == 1
		if validFrom.Valid && validFrom.String != "" {
			t, _ := time.Parse(time.RFC3339, validFrom.String)
			r.ValidFrom = sql.NullTime{Time: t, Valid: true}
		}
		if validUntil.Valid && validUntil.String != "" {
			t, _ := time.Parse(time.RFC3339, validUntil.String)
			r.ValidUntil = sql.NullTime{Time: t, Valid: true}
		}
		r.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeen)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNukiKeypadCodeByExternalID(ctx context.Context, propertyID int64, externalID string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM nuki_keypad_codes WHERE property_id = ? AND external_nuki_id = ?`, propertyID, externalID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MarkNukiAccessCodesDeletedByExternalID(ctx context.Context, propertyID int64, externalID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_access_codes
		SET status = 'revoked',
		    access_code_masked = NULL,
		    generated_pin_plain = NULL,
		    external_nuki_id = NULL,
		    error_message = NULL,
		    revoked_at = ?,
		    updated_at = ?
		WHERE property_id = ? AND external_nuki_id = ?`,
		now, now, propertyID, externalID)
	return err
}

func (s *Store) ReconcileNukiAccessCodesWithKeypad(ctx context.Context, propertyID int64, keepExternalIDs []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if len(keepExternalIDs) == 0 {
		q := `
			UPDATE nuki_access_codes
			SET status = 'revoked',
			    access_code_masked = NULL,
			    generated_pin_plain = NULL,
			    external_nuki_id = NULL,
			    error_message = NULL,
			    revoked_at = ?,
			    updated_at = ?
			WHERE property_id = ?
			  AND external_nuki_id IS NOT NULL
			  AND status = 'generated'
			  AND NOT EXISTS (
			      SELECT 1
			      FROM nuki_keypad_codes nk
			      WHERE nk.property_id = nuki_access_codes.property_id
			        AND (` + nukiLabelWindowLinkPredicate("nuki_access_codes", "nk") + `)
			  )`
		_, err := s.DB.ExecContext(ctx, q, now, now, propertyID)
		return err
	}
	ph := make([]string, len(keepExternalIDs))
	args := make([]interface{}, 0, len(keepExternalIDs)+3)
	args = append(args, now, now, propertyID)
	for i, id := range keepExternalIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	q := `
		UPDATE nuki_access_codes
		SET status = 'revoked',
		    access_code_masked = NULL,
		    generated_pin_plain = NULL,
		    external_nuki_id = NULL,
		    error_message = NULL,
		    revoked_at = ?,
		    updated_at = ?
		WHERE property_id = ?
		  AND external_nuki_id IS NOT NULL
		  AND status = 'generated'
		  AND external_nuki_id NOT IN (` + strings.Join(ph, ",") + `)
		  AND NOT EXISTS (
		      SELECT 1
		      FROM nuki_keypad_codes nk
		      WHERE nk.property_id = nuki_access_codes.property_id
		        AND (` + nukiLabelWindowLinkPredicate("nuki_access_codes", "nk") + `)
		  )`
	_, err := s.DB.ExecContext(ctx, q, args...)
	return err
}

func (s *Store) UpdateNukiKeypadCodeEnabled(ctx context.Context, propertyID int64, externalID string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_keypad_codes
		SET enabled = ?, updated_at = ?
		WHERE property_id = ? AND external_nuki_id = ?`,
		enabledInt, now, propertyID, externalID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetNukiKeypadCodeByExternalID(ctx context.Context, propertyID int64, externalID string) (*NukiKeypadCode, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, property_id, external_nuki_id, name, access_code_masked, valid_from, valid_until, enabled, raw_json, last_seen_at, created_at, updated_at
		FROM nuki_keypad_codes
		WHERE property_id = ? AND external_nuki_id = ?`, propertyID, externalID)
	var r NukiKeypadCode
	var validFrom, validUntil sql.NullString
	var enabledInt int
	var lastSeen, created, updated string
	if err := row.Scan(&r.ID, &r.PropertyID, &r.ExternalNukiID, &r.Name, &r.AccessCodeMasked, &validFrom, &validUntil, &enabledInt, &r.RawJSON, &lastSeen, &created, &updated); err != nil {
		return nil, err
	}
	r.Enabled = enabledInt == 1
	if validFrom.Valid && validFrom.String != "" {
		t, _ := time.Parse(time.RFC3339, validFrom.String)
		r.ValidFrom = sql.NullTime{Time: t, Valid: true}
	}
	if validUntil.Valid && validUntil.String != "" {
		t, _ := time.Parse(time.RFC3339, validUntil.String)
		r.ValidUntil = sql.NullTime{Time: t, Valid: true}
	}
	r.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeen)
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &r, nil
}

func (s *Store) ListUpcomingStaysForNuki(ctx context.Context, propertyID int64, limit int) ([]UpcomingStayWithCode, error) {
	if limit <= 0 || limit > 300 {
		limit = 120
	}
	query := `
		SELECT ns.id, osm.old_occupancy_id, COALESCE(osm.old_occupancy_id, nac.occupancy_id),
		       COALESCE(ns.source_reference, ''), ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
		       nac.id, nac.code_label,
		       CASE
		           WHEN nac.id IS NULL THEN 'not_generated'
		           WHEN nac.status = 'generated' AND (
		               (nac.external_nuki_id IS NOT NULL AND TRIM(nac.external_nuki_id) != '')
		               OR nk.id IS NOT NULL
		           ) THEN 'generated'
		           WHEN nac.status = 'generated' THEN 'not_generated'
		           ELSE nac.status
		       END,
		       CASE
		           WHEN nac.status = 'generated' AND (
		               (nac.external_nuki_id IS NOT NULL AND TRIM(nac.external_nuki_id) != '')
		               OR nk.id IS NOT NULL
		           )
		               THEN COALESCE(NULLIF(nac.access_code_masked, ''), nk.access_code_masked)
		           ELSE NULL
		       END,
		       CASE
		           WHEN nac.status = 'generated' AND (
		               (nac.external_nuki_id IS NOT NULL AND TRIM(nac.external_nuki_id) != '')
		               OR nk.id IS NOT NULL
		           )
		               THEN nac.generated_pin_plain
		           ELSE NULL
		       END,
		       nac.valid_from, nac.valid_until,
		       nac.error_message, nac.updated_at
		FROM named_stays ns
		LEFT JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id AND osm.migration_kind = 'named_stay'
		LEFT JOIN nuki_access_codes nac ON nac.property_id = ns.property_id AND (
		    nac.named_stay_id = ns.id
		    OR (nac.named_stay_id IS NULL AND osm.old_occupancy_id IS NOT NULL AND nac.occupancy_id = osm.old_occupancy_id)
		)
		LEFT JOIN nuki_keypad_codes nk ON nk.property_id = nac.property_id AND (
		    nk.external_nuki_id = nac.external_nuki_id
		    OR (` + nukiLabelWindowLinkPredicate("nac", "nk") + `)
		)
		WHERE ns.property_id = ?
		  AND ns.status = 'active'
		  AND COALESCE(ns.review_status, 'confirmed') = 'confirmed'
		  AND ns.stay_type IN ('booking_com', 'external')
		  AND ns.check_out_date >= ?
		ORDER BY COALESCE(nac.valid_from, ns.check_in_date) ASC, ns.id ASC
		LIMIT ?`
	nowDate := time.Now().UTC().Format("2006-01-02")
	rows, err := s.DB.QueryContext(ctx, query, propertyID, nowDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpcomingStayWithCode
	for rows.Next() {
		var r UpcomingStayWithCode
		var checkIn, checkOut string
		var upd, vf, vu sql.NullString
		if err := rows.Scan(&r.StayID, &r.LegacyOccupancyID, &r.OccupancyID, &r.SourceEventUID, &r.GuestDisplayName, &r.StayType, &checkIn, &checkOut, &r.OccupancyStatus, &r.GeneratedCodeID, &r.GeneratedLabel, &r.GeneratedStatus, &r.GeneratedMasked, &r.GeneratedPIN, &vf, &vu, &r.GeneratedError, &upd); err != nil {
			return nil, err
		}
		if err := s.decryptNS(&r.GeneratedPIN); err != nil {
			return nil, err
		}
		r.StartAt, _ = time.ParseInLocation("2006-01-02", checkIn, time.UTC)
		r.EndAt, _ = time.ParseInLocation("2006-01-02", checkOut, time.UTC)
		if vf.Valid && vf.String != "" {
			t, _ := time.Parse(time.RFC3339, vf.String)
			r.GeneratedValidFrom = sql.NullTime{Time: t, Valid: true}
		}
		if vu.Valid && vu.String != "" {
			t, _ := time.Parse(time.RFC3339, vu.String)
			r.GeneratedValidUntil = sql.NullTime{Time: t, Valid: true}
		}
		if upd.Valid && upd.String != "" {
			t, _ := time.Parse(time.RFC3339, upd.String)
			r.GeneratedUpdated = sql.NullTime{Time: t, Valid: true}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(out))
	deduped := make([]UpcomingStayWithCode, 0, len(out))
	for _, r := range out {
		if seen[r.StayID] {
			continue
		}
		seen[r.StayID] = true
		deduped = append(deduped, r)
	}
	return deduped, nil
}

func (s *Store) HasShorterGeneratedNukiCodeForSourceUID(ctx context.Context, propertyID int64, sourceUID string, targetStart, targetEnd time.Time) (bool, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT nac.valid_from, nac.valid_until
		FROM nuki_access_codes nac
		INNER JOIN occupancies o ON o.id = nac.occupancy_id
		WHERE nac.property_id = ?
		  AND o.property_id = nac.property_id
		  AND o.source_event_uid = ?
		  AND o.guest_display_name IS NOT NULL
		  AND TRIM(o.guest_display_name) != ''
		  AND nac.status = 'generated'`, propertyID, sourceUID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	targetStart = targetStart.UTC()
	targetEnd = targetEnd.UTC()
	for rows.Next() {
		var fromRaw, untilRaw string
		if err := rows.Scan(&fromRaw, &untilRaw); err != nil {
			return false, err
		}
		from, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			continue
		}
		until, err := time.Parse(time.RFC3339, untilRaw)
		if err != nil {
			continue
		}
		from = from.UTC()
		until = until.UTC()
		if !from.Before(targetStart) && !until.After(targetEnd) && until.Sub(from) < targetEnd.Sub(targetStart) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// RelinkSupersededNukiCodesTx implements PMS_19 §10.1: when a legacy generated
// split row is superseded by a newly-created named stay covering the same
// window, the existing Nuki code is relinked (same PIN) if its validity window
// matches the named stay, avoiding a PIN change for the guest. Returns the
// number of codes relinked. Codes that do not match are left in place and the
// normal revocation flow revokes them.
func (s *Store) RelinkSupersededNukiCodesTx(ctx context.Context, tx *sql.Tx, propertyID, namedStayOccID int64, upstreamUID, checkIn, checkOut string) (int, error) {
	// If the named stay already owns a code, we cannot move another onto it
	// (the table is unique on (property_id, occupancy_id)).
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM nuki_access_codes WHERE property_id = ? AND occupancy_id = ?`, propertyID, namedStayOccID).Scan(&existing); err != nil {
		return 0, err
	}
	if existing > 0 {
		return 0, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT nac.id, nac.valid_from, nac.valid_until
		FROM nuki_access_codes nac
		JOIN occupancies o ON o.id = nac.occupancy_id
		WHERE nac.property_id = ?
		  AND o.upstream_event_uid = ?
		  AND o.id <> ?
		  AND (
		      o.superseded_at IS NOT NULL
		      OR NOT EXISTS (
		          SELECT 1 FROM occupancy_nights n
		          WHERE n.occupancy_id = o.id AND n.active = 1
		      )
		  )
		  AND nac.status = 'generated'
		  AND nac.revoked_at IS NULL
		ORDER BY nac.id ASC`, propertyID, upstreamUID, namedStayOccID)
	if err != nil {
		return 0, err
	}
	type cand struct {
		id                 int64
		validFrom, validTo string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.validFrom, &c.validTo); err != nil {
			rows.Close()
			return 0, err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	relinked := 0
	for _, c := range cands {
		vf, _ := time.Parse(time.RFC3339, c.validFrom)
		vt, _ := time.Parse(time.RFC3339, c.validTo)
		// "Validity window exactly matches" is evaluated on the property-local
		// night dates: check-in day == code start day, check-out day == code end.
		if vf.UTC().Format("2006-01-02") != checkIn || vt.UTC().Format("2006-01-02") != checkOut {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE nuki_access_codes SET occupancy_id = ?, updated_at = ? WHERE id = ? AND property_id = ?`,
			namedStayOccID, now, c.id, propertyID); err != nil {
			return relinked, err
		}
		relinked++
		break // only one code can occupy the named stay row
	}
	return relinked, nil
}
