-- T3.1: TOTP-based 2FA for users.
--
-- Adds opt-in TOTP enrolment columns to `users`, a per-session
-- `mfa_verified` flag so password-only sessions for enrolled users
-- are quarantined until the challenge is completed, and a table
-- holding single-use recovery codes.

ALTER TABLE users ADD COLUMN totp_secret TEXT;
ALTER TABLE users ADD COLUMN totp_enrolled_at TEXT;

-- Existing sessions default to verified (1) so deploying the migration
-- does not invalidate logged-in users. New sessions for enrolled users
-- will be created with mfa_verified=0 until they pass the challenge.
ALTER TABLE auth_sessions ADD COLUMN mfa_verified INTEGER NOT NULL DEFAULT 1;

CREATE TABLE user_recovery_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL UNIQUE,
    used_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_user_recovery_codes_user ON user_recovery_codes (user_id);
