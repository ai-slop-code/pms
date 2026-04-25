DROP INDEX IF EXISTS idx_user_recovery_codes_user;
DROP TABLE IF EXISTS user_recovery_codes;

-- SQLite does not support DROP COLUMN portably until 3.35; modernc.org/sqlite
-- ships 3.45 so this works on our runtime. We tolerate failure for robustness.
ALTER TABLE auth_sessions DROP COLUMN mfa_verified;
ALTER TABLE users DROP COLUMN totp_enrolled_at;
ALTER TABLE users DROP COLUMN totp_secret;
