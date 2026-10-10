package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	NukiOperationCreatePending = "create_pending"
	NukiOperationActive        = "active"
	NukiOperationNeedsReview   = "needs_review"
)

type NukiManagedCredential struct {
	ID                   int64
	PropertyID           int64
	NamedStayID          sql.NullInt64
	NukiAccessCodeID     sql.NullInt64
	SmartlockID          string
	RemoteID             sql.NullString
	Provenance           string
	ProvenanceReference  sql.NullString
	DesiredLabel         string
	DesiredValidFrom     time.Time
	DesiredValidUntil    time.Time
	DesiredEnabled       bool
	OperationState       string
	OperationRevision    int64
	ObservedJSON         sql.NullString
	ObservedAt           sql.NullTime
	LatestError          sql.NullString
	LastAttemptAt        sql.NullTime
	NextRetryAt          sql.NullTime
	PendingPIN           sql.NullString
	RequestAcceptedAt    sql.NullTime
	CompletionObservedAt sql.NullTime
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (s *Store) CreateNukiManagedCredential(ctx context.Context, c *NukiManagedCredential) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	from := c.DesiredValidFrom.UTC().Format(time.RFC3339Nano)
	until := c.DesiredValidUntil.UTC().Format(time.RFC3339Nano)
	accepted := managedNullableTime(c.RequestAcceptedAt)
	completed := managedNullableTime(c.CompletionObservedAt)
	observed := managedNullableTime(c.ObservedAt)
	attempt := managedNullableTime(c.LastAttemptAt)
	retry := managedNullableTime(c.NextRetryAt)
	pending := sql.NullString{}
	if c.PendingPIN.Valid {
		enc, err := s.Crypto.Encrypt(c.PendingPIN.String)
		if err != nil {
			return 0, err
		}
		pending = sql.NullString{String: enc, Valid: true}
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO nuki_managed_credentials (
			property_id, named_stay_id, nuki_access_code_id, smartlock_id, remote_id,
			provenance, provenance_reference, desired_label, desired_valid_from,
			desired_valid_until, desired_enabled, operation_state, operation_revision,
			observed_json, observed_at, latest_error, last_attempt_at, next_retry_at,
			pending_pin, request_accepted_at, completion_observed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.PropertyID, managedNullableInt(c.NamedStayID), managedNullableInt(c.NukiAccessCodeID), c.SmartlockID,
		managedNullableString(c.RemoteID), c.Provenance, managedNullableString(c.ProvenanceReference), c.DesiredLabel,
		from, until, managedBoolInt(c.DesiredEnabled), c.OperationState, c.OperationRevision,
		managedNullableString(c.ObservedJSON), observed, managedNullableString(c.LatestError), attempt, retry,
		managedNullableString(pending), accepted, completed, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) HasPendingNukiManagedCredentialForStay(ctx context.Context, propertyID, stayID int64) (bool, error) {
	var pending int
	err := s.DB.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM nuki_managed_credentials
			WHERE property_id = ? AND named_stay_id = ?
			  AND operation_state IN ('create_pending', 'update_pending', 'delete_pending')
		)`, propertyID, stayID).Scan(&pending)
	return pending == 1, err
}

// ListDueNukiManagedCreations returns persisted creation work, including rows
// from before a restart whose retry timestamp was never written.
func (s *Store) ListDueNukiManagedCreations(ctx context.Context, propertyID int64, now time.Time, limit int) ([]NukiManagedCredential, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	nowText := now.UTC().Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, named_stay_id, nuki_access_code_id, smartlock_id,
		       remote_id, provenance, provenance_reference, desired_label,
		       desired_valid_from, desired_valid_until, desired_enabled,
		       operation_state, operation_revision, observed_json, observed_at,
		       latest_error, last_attempt_at, next_retry_at, pending_pin,
		       request_accepted_at, completion_observed_at, created_at, updated_at
		FROM nuki_managed_credentials
		WHERE property_id = ? AND operation_state = 'create_pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY id ASC LIMIT ?`, propertyID, nowText, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiManagedCredential
	for rows.Next() {
		c, err := scanManagedCredential(rows)
		if err != nil {
			return nil, err
		}
		if err := s.decryptNS(&c.PendingPIN); err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) LatestNukiManagedCredentialForStay(ctx context.Context, propertyID, stayID int64) (*NukiManagedCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, named_stay_id, nuki_access_code_id, smartlock_id,
		       remote_id, provenance, provenance_reference, desired_label,
		       desired_valid_from, desired_valid_until, desired_enabled,
		       operation_state, operation_revision, observed_json, observed_at,
		       latest_error, last_attempt_at, next_retry_at, pending_pin,
		       request_accepted_at, completion_observed_at, created_at, updated_at
		FROM nuki_managed_credentials WHERE property_id = ? AND named_stay_id = ?
		ORDER BY id DESC LIMIT 1`, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	c, err := scanManagedCredential(rows)
	if err != nil {
		return nil, err
	}
	if err := s.decryptNS(&c.PendingPIN); err != nil {
		return nil, err
	}
	return c, nil
}

// ClaimNukiManagedCreation advances the revision only when the loaded
// revision is still current. This serializes API retries and workers without
// holding a SQLite transaction across provider I/O.
func (s *Store) ClaimNukiManagedCreation(ctx context.Context, id, revision int64, now time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_revision = operation_revision + 1,
		    last_attempt_at = ?, updated_at = ?
		WHERE id = ? AND operation_state = 'create_pending'
		  AND operation_revision = ?`, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), id, revision)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, sql.ErrNoRows
	}
	return revision + 1, nil
}

func (s *Store) ListDueNukiManagedDeletions(ctx context.Context, propertyID int64, now time.Time, limit int) ([]NukiManagedCredential, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, property_id, named_stay_id, nuki_access_code_id, smartlock_id,
		       remote_id, provenance, provenance_reference, desired_label,
		       desired_valid_from, desired_valid_until, desired_enabled,
		       operation_state, operation_revision, observed_json, observed_at,
		       latest_error, last_attempt_at, next_retry_at, pending_pin,
		       request_accepted_at, completion_observed_at, created_at, updated_at
		FROM nuki_managed_credentials
		WHERE property_id = ? AND operation_state = 'delete_pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY id ASC LIMIT ?`, propertyID, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiManagedCredential
	for rows.Next() {
		c, err := scanManagedCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) ListDueNukiManagedUpdates(ctx context.Context, propertyID int64, now time.Time, limit int) ([]NukiManagedCredential, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, property_id, named_stay_id, nuki_access_code_id, smartlock_id, remote_id, provenance, provenance_reference, desired_label, desired_valid_from, desired_valid_until, desired_enabled, operation_state, operation_revision, observed_json, observed_at, latest_error, last_attempt_at, next_retry_at, pending_pin, request_accepted_at, completion_observed_at, created_at, updated_at FROM nuki_managed_credentials WHERE property_id=? AND operation_state='update_pending' AND (next_retry_at IS NULL OR next_retry_at <= ?) ORDER BY id LIMIT ?`, propertyID, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NukiManagedCredential
	for rows.Next() {
		c, err := scanManagedCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) NukiManagedRemoteOwner(ctx context.Context, propertyID int64, smartlockID, remoteID string, excludeID int64) (bool, error) {
	var owned int
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM nuki_managed_credentials
		WHERE property_id = ? AND smartlock_id = ? AND remote_id = ?
		  AND id <> ? AND operation_state <> 'deleted')`, propertyID, smartlockID, remoteID, excludeID).Scan(&owned)
	return owned == 1, err
}

type managedScanner interface{ Scan(...interface{}) error }

func scanManagedCredential(row managedScanner) (*NukiManagedCredential, error) {
	var c NukiManagedCredential
	var from, until, createdAt, updatedAt string
	var observedAt, attemptAt, retryAt, acceptedAt, completedAt sql.NullString
	var enabled int
	if err := row.Scan(&c.ID, &c.PropertyID, &c.NamedStayID, &c.NukiAccessCodeID, &c.SmartlockID,
		&c.RemoteID, &c.Provenance, &c.ProvenanceReference, &c.DesiredLabel, &from, &until,
		&enabled, &c.OperationState, &c.OperationRevision, &c.ObservedJSON, &observedAt,
		&c.LatestError, &attemptAt, &retryAt, &c.PendingPIN, &acceptedAt, &completedAt,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.DesiredValidFrom, _ = time.Parse(time.RFC3339Nano, from)
	c.DesiredValidUntil, _ = time.Parse(time.RFC3339Nano, until)
	c.DesiredEnabled = enabled != 0
	parseTime := func(v sql.NullString) sql.NullTime {
		if !v.Valid || v.String == "" {
			return sql.NullTime{}
		}
		t, err := time.Parse(time.RFC3339Nano, v.String)
		return sql.NullTime{Time: t, Valid: err == nil}
	}
	c.ObservedAt = parseTime(observedAt)
	c.LastAttemptAt = parseTime(attemptAt)
	c.NextRetryAt = parseTime(retryAt)
	c.RequestAcceptedAt = parseTime(acceptedAt)
	c.CompletionObservedAt = parseTime(completedAt)
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &c, nil
}

func (s *Store) StartNukiManagedDeletion(ctx context.Context, c *NukiManagedCredential) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = 'delete_pending', operation_revision = operation_revision + 1,
		    latest_error = NULL, last_attempt_at = ?, updated_at = ?
		WHERE property_id = ? AND smartlock_id = ? AND remote_id = ? AND operation_state <> 'deleted'`,
		now, now, c.PropertyID, c.SmartlockID, c.RemoteID.String)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		if err := s.DB.QueryRowContext(ctx, `SELECT id, operation_revision FROM nuki_managed_credentials WHERE property_id=? AND smartlock_id=? AND remote_id=? ORDER BY id DESC LIMIT 1`, c.PropertyID, c.SmartlockID, c.RemoteID.String).Scan(&c.ID, &c.OperationRevision); err != nil {
			return err
		}
		return nil
	}
	c.OperationState = "delete_pending"
	c.OperationRevision = 1
	id, err := s.CreateNukiManagedCredential(ctx, c)
	if err == nil {
		c.ID = id
	}
	return err
}

func (s *Store) MarkNukiManagedDeletionAttempt(ctx context.Context, id, revision int64, errMessage string, nextRetry time.Time, accepted bool) error {
	return s.UpdateNukiManagedOperation(ctx, id, "delete_pending", revision+1, "", errMessage, nextRetry, accepted, false)
}

func (s *Store) LinkNukiManagedAccessCode(ctx context.Context, intentID, codeID int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE nuki_managed_credentials SET nuki_access_code_id = ?, updated_at = ? WHERE id = ?`, codeID, time.Now().UTC().Format(time.RFC3339Nano), intentID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) StartNukiManagedUpdate(ctx context.Context, propertyID int64, smartlockID, remoteID, label string, from, until time.Time, enabled bool, codeID int64) (int64, int64, error) {
	var id, revision int64
	err := s.DB.QueryRowContext(ctx, `SELECT id, operation_revision FROM nuki_managed_credentials WHERE property_id=? AND smartlock_id=? AND remote_id=? ORDER BY id DESC LIMIT 1`, propertyID, smartlockID, remoteID).Scan(&id, &revision)
	if err == sql.ErrNoRows {
		return 0, 0, errors.New("nuki_legacy_identity_requires_validation")
	}
	if err != nil {
		return 0, 0, err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE nuki_managed_credentials SET operation_state='update_pending', operation_revision=?, desired_label=?, desired_valid_from=?, desired_valid_until=?, desired_enabled=?, nuki_access_code_id=COALESCE(?, nuki_access_code_id), latest_error=NULL, next_retry_at=NULL, updated_at=? WHERE id=? AND operation_revision=? AND operation_state='active'`, revision+1, label, from.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano), managedBoolInt(enabled), nullableInt(codeID), time.Now().UTC().Format(time.RFC3339Nano), id, revision)
	if err != nil {
		return 0, 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if n != 1 {
		return 0, 0, sql.ErrNoRows
	}
	return id, revision + 1, nil
}

func (s *Store) ConfirmNukiManagedUpdate(ctx context.Context, id, expectedRevision int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE nuki_managed_credentials SET operation_state='active', operation_revision=?, latest_error=NULL, next_retry_at=NULL, completion_observed_at=?, updated_at=? WHERE id=? AND operation_revision=? AND operation_state='update_pending'`, expectedRevision+1, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id, expectedRevision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ManagedNukiCredentialForRemote(ctx context.Context, propertyID int64, smartlockID, remoteID string) (*NukiManagedCredential, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, property_id, named_stay_id, nuki_access_code_id, smartlock_id, remote_id, provenance, provenance_reference, desired_label, desired_valid_from, desired_valid_until, desired_enabled, operation_state, operation_revision, observed_json, observed_at, latest_error, last_attempt_at, next_retry_at, pending_pin, request_accepted_at, completion_observed_at, created_at, updated_at FROM nuki_managed_credentials WHERE property_id=? AND smartlock_id=? AND remote_id=? ORDER BY id DESC LIMIT 1`, propertyID, smartlockID, remoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	c, err := scanManagedCredential(rows)
	if err != nil {
		return nil, err
	}
	if err := s.decryptNS(&c.PendingPIN); err != nil {
		return nil, err
	}
	return c, nil
}

// ApplyNukiIncidentRecovery performs the projection, managed provenance, and
// audit write in one transaction. It never stores or creates a PIN.
func (s *Store) ApplyNukiIncidentRecovery(ctx context.Context, propertyID, stayID, codeID int64, smartlockID, remoteID, label string, from, until time.Time, remotePresent bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(e error) error { _ = tx.Rollback(); return e }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := "deleted"
	eventMessage := "incident recovery recorded confirmed remote absence"
	if remotePresent {
		status = "delete_pending"
		eventMessage = "incident recovery recorded; deletion confirmation pending"
	}
	res, err := tx.ExecContext(ctx, `UPDATE nuki_access_codes SET external_nuki_id=?, status='revoked', access_code_masked=NULL, generated_pin_plain=NULL, error_message=NULL, updated_at=?, revoked_at=? WHERE property_id=? AND id=? AND named_stay_id=?`, remoteID, now, now, propertyID, codeID, stayID)
	if err != nil {
		return rollback(err)
	}
	if n, e := res.RowsAffected(); e != nil || n != 1 {
		if e != nil {
			return rollback(e)
		}
		return rollback(sql.ErrNoRows)
	}
	var managedID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM nuki_managed_credentials WHERE property_id=? AND smartlock_id=? AND remote_id=? ORDER BY id DESC LIMIT 1`, propertyID, smartlockID, remoteID).Scan(&managedID)
	if err == sql.ErrNoRows {
		res, err = tx.ExecContext(ctx, `INSERT INTO nuki_managed_credentials (property_id,named_stay_id,nuki_access_code_id,smartlock_id,remote_id,provenance,provenance_reference,desired_label,desired_valid_from,desired_valid_until,desired_enabled,operation_state,operation_revision,completion_observed_at,created_at,updated_at) VALUES (?,?,?,?,?,'evidence_backed_incident_recovery','PMS-34 Amarildo manifest',?,?,?,?,?,1,?,?,?)`, propertyID, stayID, codeID, smartlockID, remoteID, label, from.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano), 1, status, now, now, now)
		if err != nil {
			return rollback(err)
		}
		managedID, err = res.LastInsertId()
		if err != nil {
			return rollback(err)
		}
	} else if err != nil {
		return rollback(err)
	} else {
		res, err = tx.ExecContext(ctx, `UPDATE nuki_managed_credentials SET named_stay_id=?, nuki_access_code_id=?, operation_state=?, operation_revision=operation_revision+1, completion_observed_at=?, updated_at=? WHERE id=?`, stayID, codeID, status, now, now, managedID)
		if err != nil {
			return rollback(err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO nuki_event_logs (property_id,nuki_access_code_id,event_type,message,created_at) VALUES (?,?,?,?,?)`, propertyID, codeID, "incident_recovery", eventMessage, now)
	if err != nil {
		return rollback(err)
	}
	return tx.Commit()
}

// ConfirmNukiManagedCreation commits provider identity and the stay-facing
// projection together. expectedRevision is the revision claimed before the
// provider read; a stale worker cannot overwrite newer lifecycle state.
func (s *Store) ConfirmNukiManagedCreation(ctx context.Context, intentID, expectedRevision int64, remoteID string, code *NukiAccessCode, pin string, eligible bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(e error) error { _ = tx.Rollback(); return e }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	state := "active"
	if !eligible {
		state = "delete_pending"
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = ?, operation_revision = ?, remote_id = ?,
		    nuki_access_code_id = ?,
		    latest_error = NULL, next_retry_at = NULL, pending_pin = NULL,
		    request_accepted_at = COALESCE(request_accepted_at, ?),
		    completion_observed_at = ?, updated_at = ?
		WHERE id = ? AND operation_revision = ? AND operation_state = 'create_pending'`,
		state, expectedRevision+1, remoteID, codeIDValue(code), now, now, now, intentID, expectedRevision)
	if err != nil {
		return rollback(err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return rollback(err)
	}
	if n != 1 {
		return rollback(sql.ErrNoRows)
	}
	if code == nil || !code.NamedStayID.Valid {
		return rollback(errors.New("nuki projection requires named stay"))
	}
	var encrypted interface{}
	if eligible && pin != "" {
		value := sql.NullString{String: pin, Valid: true}
		if s.Crypto != nil {
			enc, err := s.Crypto.Encrypt(pin)
			if err != nil {
				return rollback(err)
			}
			value.String = enc
		}
		encrypted = value.String
	}
	masked := interface{}(nil)
	if eligible && code.AccessCodeMasked.Valid {
		masked = code.AccessCodeMasked.String
	}
	status := "revoked"
	if eligible {
		status = "generated"
	}
	validFrom := code.ValidFrom.UTC().Format(time.RFC3339Nano)
	validUntil := code.ValidUntil.UTC().Format(time.RFC3339Nano)
	if code.ID > 0 {
		res, err = tx.ExecContext(ctx, `UPDATE nuki_access_codes
			SET code_label = ?, access_code_masked = ?, generated_pin_plain = ?, external_nuki_id = ?,
			    valid_from = ?, valid_until = ?, status = ?, error_message = NULL, updated_at = ?, revoked_at = CASE WHEN ? = 'revoked' THEN ? ELSE NULL END
			WHERE property_id = ? AND id = ?`, code.CodeLabel, masked, encrypted, remoteID, validFrom, validUntil, status, now, status, now, code.PropertyID, code.ID)
	} else {
		res, err = tx.ExecContext(ctx, `INSERT INTO nuki_access_codes
			(property_id, named_stay_id, code_label, access_code_masked, generated_pin_plain, external_nuki_id,
			 valid_from, valid_until, status, error_message, created_at, updated_at, revoked_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, CASE WHEN ? = 'revoked' THEN ? ELSE NULL END)`,
			code.PropertyID, code.NamedStayID.Int64, code.CodeLabel, masked, encrypted, remoteID,
			validFrom, validUntil, status, now, now, status, now)
	}
	if err != nil {
		return rollback(err)
	}
	if affected, err := res.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return rollback(err)
		}
		return rollback(sql.ErrNoRows)
	}
	return tx.Commit()
}

// ConfirmNukiManagedDeletion atomically records authoritative absence and
// clears projection secrets while retaining the remote identity in history.
func (s *Store) ConfirmNukiManagedDeletion(ctx context.Context, intentID, expectedRevision int64, codeID sql.NullInt64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(e error) error { _ = tx.Rollback(); return e }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `UPDATE nuki_managed_credentials
		SET operation_state = 'deleted', operation_revision = ?, latest_error = NULL,
		    next_retry_at = NULL, pending_pin = NULL, completion_observed_at = ?, updated_at = ?
		WHERE id = ? AND operation_revision = ? AND operation_state = 'delete_pending'`, expectedRevision+1, now, now, intentID, expectedRevision)
	if err != nil {
		return rollback(err)
	}
	if n, e := res.RowsAffected(); e != nil || n != 1 {
		if e != nil {
			return rollback(e)
		}
		return rollback(sql.ErrNoRows)
	}
	if codeID.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE nuki_access_codes SET status='revoked', access_code_masked=NULL, generated_pin_plain=NULL, error_message=NULL, revoked_at=?, updated_at=? WHERE id=?`, now, now, codeID.Int64); err != nil {
			return rollback(err)
		}
	}
	return tx.Commit()
}

func (s *Store) MarkNukiManagedDeleted(ctx context.Context, propertyID int64, smartlockID, remoteID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = 'deleted', latest_error = NULL,
		    completion_observed_at = ?, updated_at = ?
		WHERE property_id = ? AND smartlock_id = ? AND remote_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), propertyID, smartlockID, remoteID)
	return err
}

func (s *Store) UpdateNukiManagedOperation(ctx context.Context, id int64, state string, revision int64, remoteID, errMessage string, nextRetry time.Time, accepted, completed bool) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var retry interface{}
	if !nextRetry.IsZero() {
		retry = nextRetry.UTC().Format(time.RFC3339Nano)
	}
	var acceptedAt, completedAt interface{}
	if accepted {
		acceptedAt = now
	}
	if completed {
		completedAt = now
	}
	var remote interface{}
	if remoteID != "" {
		remote = remoteID
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = ?, operation_revision = ?, remote_id = COALESCE(?, remote_id),
		    latest_error = ?, next_retry_at = ?, request_accepted_at = COALESCE(?, request_accepted_at),
		    completion_observed_at = COALESCE(?, completion_observed_at), updated_at = ?
		WHERE id = ? AND operation_revision = ?`,
		state, revision, remote, nullableStringValue(errMessage), retry, acceptedAt, completedAt, now, id, revision-1)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func managedNullableString(v sql.NullString) interface{} {
	if v.Valid {
		return v.String
	}
	return nil
}

func codeIDValue(c *NukiAccessCode) interface{} {
	if c != nil && c.ID > 0 {
		return c.ID
	}
	return nil
}

func nullableStringValue(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

func managedNullableInt(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func nullableInt(v int64) interface{} {
	if v > 0 {
		return v
	}
	return nil
}

func managedNullableTime(v sql.NullTime) interface{} {
	if v.Valid {
		return v.Time.UTC().Format(time.RFC3339Nano)
	}
	return nil
}

func managedBoolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
