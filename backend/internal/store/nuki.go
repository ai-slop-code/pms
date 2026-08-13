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

type NukiAccessCodeWithStay struct {
	Code          NukiAccessCode
	StayReference string
	StaySummary   sql.NullString
	StayStatus    string
	StayStart     time.Time
	StayEnd       time.Time
}

type NukiStay struct {
	NamedStayID     int64
	PropertyID      int64
	DisplayName     string
	StayType        string
	ReviewStatus    string
	CheckInDate     string
	CheckOutDate    string
	Status          string
	SourceReference sql.NullString
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
	SourceEventUID      string
	RawSummary          sql.NullString
	GuestDisplayName    sql.NullString
	StayType            string
	StartAt             time.Time
	EndAt               time.Time
	StayStatus          string
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

func (s *Store) GetNukiCodeByNamedStayID(ctx context.Context, propertyID, stayID int64) (*NukiAccessCode, error) {
	rows, err := s.scanNukiCodes(ctx, `
		SELECT id, property_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
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
		SELECT id, property_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
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
	if !c.NamedStayID.Valid || c.NamedStayID.Int64 <= 0 {
		return errors.New("nuki code requires named_stay_id")
	}
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
	stayID := c.NamedStayID.Int64
	var revoked interface{}
	if c.RevokedAt.Valid {
		revoked = c.RevokedAt.Time.UTC().Format(time.RFC3339)
	}
	args := []interface{}{c.PropertyID, stayID, c.CodeLabel, masked, pinPlain, ext,
		c.ValidFrom.UTC().Format(time.RFC3339), c.ValidUntil.UTC().Format(time.RFC3339), c.Status, errMsg, runID, now, now, revoked}
	setSQL := `code_label = excluded.code_label, access_code_masked = excluded.access_code_masked,
			generated_pin_plain = excluded.generated_pin_plain, external_nuki_id = excluded.external_nuki_id,
			valid_from = excluded.valid_from, valid_until = excluded.valid_until, status = excluded.status,
			error_message = excluded.error_message, last_sync_run_id = excluded.last_sync_run_id,
			updated_at = excluded.updated_at, revoked_at = excluded.revoked_at`
	if c.ID > 0 {
		_, err := s.DB.ExecContext(ctx, `
			UPDATE nuki_access_codes SET named_stay_id = ?, code_label = ?, access_code_masked = ?,
				generated_pin_plain = ?, external_nuki_id = ?, valid_from = ?, valid_until = ?, status = ?,
				error_message = ?, last_sync_run_id = ?, updated_at = ?, revoked_at = ?
			WHERE property_id = ? AND id = ?`, stayID, c.CodeLabel, masked, pinPlain, ext,
			c.ValidFrom.UTC().Format(time.RFC3339), c.ValidUntil.UTC().Format(time.RFC3339), c.Status, errMsg, runID, now, revoked, c.PropertyID, c.ID)
		return err
	}
	query := `INSERT INTO nuki_access_codes (property_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, named_stay_id) WHERE named_stay_id IS NOT NULL DO UPDATE SET ` + setSQL
	_, err := s.DB.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) ListNukiCodes(ctx context.Context, propertyID int64, scope string) ([]NukiAccessCodeWithStay, error) {
	q := `
		SELECT nac.id, nac.property_id, nac.named_stay_id, nac.code_label, nac.access_code_masked, nac.generated_pin_plain, nac.external_nuki_id, nac.valid_from, nac.valid_until, nac.status, nac.error_message, nac.last_sync_run_id, nac.created_at, nac.updated_at, nac.revoked_at,
		       COALESCE(ns.source_reference, ''), ns.display_name, ns.status,
		       ns.check_in_date, ns.check_out_date
		FROM nuki_access_codes nac
		JOIN named_stays ns ON ns.id = nac.named_stay_id AND ns.property_id = nac.property_id
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
	var out []NukiAccessCodeWithStay
	for rows.Next() {
		var r NukiAccessCodeWithStay
		var validFrom, validUntil, created, updated string
		var revoked sql.NullString
		var oStart, oEnd string
		if err := rows.Scan(
			&r.Code.ID, &r.Code.PropertyID, &r.Code.NamedStayID, &r.Code.CodeLabel, &r.Code.AccessCodeMasked, &r.Code.GeneratedPINPlain, &r.Code.ExternalNukiID,
			&validFrom, &validUntil, &r.Code.Status, &r.Code.ErrorMessage, &r.Code.LastSyncRunID, &created, &updated, &revoked,
			&r.StayReference, &r.StaySummary, &r.StayStatus, &oStart, &oEnd,
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
		r.StayStart, _ = time.ParseInLocation("2006-01-02", oStart, time.UTC)
		r.StayEnd, _ = time.ParseInLocation("2006-01-02", oEnd, time.UTC)
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
		if err := rows.Scan(&c.ID, &c.PropertyID, &c.NamedStayID, &c.CodeLabel, &c.AccessCodeMasked, &c.GeneratedPINPlain, &c.ExternalNukiID, &validFrom, &validUntil, &c.Status, &c.ErrorMessage, &c.LastSyncRunID, &created, &updated, &revoked); err != nil {
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

func (s *Store) ListNamedStaysForNukiSync(ctx context.Context, propertyID int64) ([]NukiStay, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type,
		       COALESCE(ns.review_status, 'confirmed'), ns.check_in_date, ns.check_out_date,
		       ns.status, ns.source_reference
		FROM named_stays ns
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
		SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type,
		       COALESCE(ns.review_status, 'confirmed'), ns.check_in_date, ns.check_out_date,
		       ns.status, ns.source_reference
		FROM named_stays ns
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
		if err := rows.Scan(&r.NamedStayID, &r.PropertyID, &r.DisplayName, &r.StayType, &r.ReviewStatus, &r.CheckInDate, &r.CheckOutDate, &r.Status, &r.SourceReference); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListNukiCodesForCleanup(ctx context.Context, propertyID int64, nowUTC time.Time) ([]NukiAccessCode, error) {
	q := `
		SELECT id, property_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, error_message, last_sync_run_id, created_at, updated_at, revoked_at
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
		SELECT ns.id, COALESCE(ns.source_reference, ''), ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
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
		LEFT JOIN nuki_access_codes nac ON nac.property_id = ns.property_id AND nac.named_stay_id = ns.id
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
		if err := rows.Scan(&r.StayID, &r.SourceEventUID, &r.GuestDisplayName, &r.StayType, &checkIn, &checkOut, &r.StayStatus, &r.GeneratedCodeID, &r.GeneratedLabel, &r.GeneratedStatus, &r.GeneratedMasked, &r.GeneratedPIN, &vf, &vu, &r.GeneratedError, &upd); err != nil {
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
