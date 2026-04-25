-- Force first-login password change for the bootstrap super-admin (and any
-- account an operator wants to mark for rotation). When set to 1 the API
-- gate refuses every state-changing route except the password-change call,
-- so the user is funnelled into rotating credentials before doing anything
-- else. Cleared automatically when the user updates their own password.
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
