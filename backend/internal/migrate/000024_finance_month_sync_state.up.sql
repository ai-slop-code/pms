ALTER TABLE finance_month_states ADD COLUMN last_synced_at TEXT;
ALTER TABLE finance_month_states ADD COLUMN last_synced_by INTEGER REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE finance_month_states ADD COLUMN last_synced_reason TEXT;

UPDATE finance_month_states
   SET last_synced_at = opened_at,
       last_synced_by = opened_by,
       last_synced_reason = 'initial_open_legacy'
 WHERE last_synced_at IS NULL;
