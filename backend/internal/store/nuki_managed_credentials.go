package store

import (
	"context"
	"database/sql"
	"time"
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

func (s *Store) StartNukiManagedDeletion(ctx context.Context, c *NukiManagedCredential) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = 'delete_pending', operation_revision = operation_revision + 1,
		    latest_error = NULL, last_attempt_at = ?, updated_at = ?
		WHERE property_id = ? AND smartlock_id = ? AND remote_id = ?`,
		now, now, c.PropertyID, c.SmartlockID, c.RemoteID.String)
	if err != nil {
		return err
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT changes()`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	c.OperationState = "delete_pending"
	c.OperationRevision = 1
	_, err = s.CreateNukiManagedCredential(ctx, c)
	return err
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
	_, err := s.DB.ExecContext(ctx, `
		UPDATE nuki_managed_credentials
		SET operation_state = ?, operation_revision = ?, remote_id = COALESCE(?, remote_id),
		    latest_error = ?, next_retry_at = ?, request_accepted_at = COALESCE(?, request_accepted_at),
		    completion_observed_at = COALESCE(?, completion_observed_at), updated_at = ?
		WHERE id = ? AND operation_revision <= ?`,
		state, revision, remote, nullableStringValue(errMessage), retry, acceptedAt, completedAt, now, id, revision)
	return err
}

func managedNullableString(v sql.NullString) interface{} {
	if v.Valid {
		return v.String
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
