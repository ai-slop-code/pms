package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// --- TOTP secret (per-user) ---------------------------------------------

// SetUserTOTP persists an encrypted TOTP secret and marks the user as
// enrolled. The raw base32 secret is encrypted in place using Store.Crypto
// if configured; otherwise it is stored as plaintext (dev/test convenience).
func (s *Store) SetUserTOTP(ctx context.Context, userID int64, rawSecret string) error {
	enc := sql.NullString{String: rawSecret, Valid: true}
	enc, err := s.encryptNS(enc)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.DB.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, totp_enrolled_at = ?, updated_at = ? WHERE id = ?`,
		enc.String, now, now, userID)
	return err
}

// ClearUserTOTP removes the enrolment and any remaining recovery codes.
func (s *Store) ClearUserTOTP(ctx context.Context, userID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx,
		`UPDATE users SET totp_secret = NULL, totp_enrolled_at = NULL, updated_at = ? WHERE id = ?`,
		now, userID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID)
	return err
}

// GetUserTOTPSecret returns the decrypted secret and enrolled flag for a
// user. Returns ok=false when the user has not enrolled.
func (s *Store) GetUserTOTPSecret(ctx context.Context, userID int64) (secret string, enrolled bool, err error) {
	var sec sql.NullString
	var enrolledAt sql.NullString
	err = s.DB.QueryRowContext(ctx,
		`SELECT totp_secret, totp_enrolled_at FROM users WHERE id = ?`, userID).
		Scan(&sec, &enrolledAt)
	if err != nil {
		return "", false, err
	}
	if !sec.Valid || sec.String == "" {
		return "", false, nil
	}
	if err := s.decryptNS(&sec); err != nil {
		return "", false, err
	}
	return sec.String, enrolledAt.Valid, nil
}

// --- Session MFA flag ----------------------------------------------------

// CreateSessionWithMFA inserts a new session row. When mfaVerified is false
// the session is quarantined — only the 2FA verification endpoint will
// honour it until the flag is flipped via SetSessionMFAVerified.
func (s *Store) CreateSessionWithMFA(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, mfaVerified bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	flag := 0
	if mfaVerified {
		flag = 1
	}
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO auth_sessions (user_id, token_hash, expires_at, created_at, mfa_verified) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt.UTC().Format(time.RFC3339), now, flag)
	return err
}

// SessionLookup is what middleware.Auth reads from an incoming cookie.
type SessionLookup struct {
	UserID      int64
	MFAVerified bool
}

// LookupSessionByHash returns the session metadata for a cookie hash. A row
// whose expires_at has passed is deleted and sql.ErrNoRows is returned.
func (s *Store) LookupSessionByHash(ctx context.Context, tokenHash string) (SessionLookup, error) {
	var uid int64
	var exp string
	var mfa int
	err := s.DB.QueryRowContext(ctx,
		`SELECT user_id, expires_at, mfa_verified FROM auth_sessions WHERE token_hash = ?`, tokenHash).
		Scan(&uid, &exp, &mfa)
	if err != nil {
		return SessionLookup{}, err
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil || time.Now().UTC().After(t) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, tokenHash)
		return SessionLookup{}, sql.ErrNoRows
	}
	return SessionLookup{UserID: uid, MFAVerified: mfa == 1}, nil
}

// SetSessionMFAVerified flips the mfa_verified flag to 1 on an existing
// session. Returns sql.ErrNoRows if the cookie has no matching row.
func (s *Store) SetSessionMFAVerified(ctx context.Context, tokenHash string) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE auth_sessions SET mfa_verified = 1 WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- Recovery codes ------------------------------------------------------

// ReplaceRecoveryCodes deletes any existing recovery codes for the user and
// inserts the provided hashes. Intended to be called at enrolment time.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID int64, codeHashes []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, h := range codeHashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_recovery_codes (user_id, code_hash, created_at) VALUES (?, ?, ?)`,
			userID, h, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ConsumeRecoveryCode marks a recovery code as used if it exists for the
// user and has not been used before. Returns true when a code was consumed.
// The check-and-flip is atomic (UPDATE WHERE used_at IS NULL).
func (s *Store) ConsumeRecoveryCode(ctx context.Context, userID int64, codeHash string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx,
		`UPDATE user_recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		now, userID, codeHash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountRecoveryCodesRemaining returns the number of unused codes.
func (s *Store) CountRecoveryCodesRemaining(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		userID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
