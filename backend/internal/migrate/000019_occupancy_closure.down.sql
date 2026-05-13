DROP INDEX IF EXISTS idx_occupancies_property_closure;

-- modernc.org/sqlite ships SQLite ≥3.45, so DROP COLUMN works.
ALTER TABLE occupancies DROP COLUMN external_channel;
ALTER TABLE occupancies DROP COLUMN external_currency;
ALTER TABLE occupancies DROP COLUMN external_net_amount_cents;
ALTER TABLE occupancies DROP COLUMN closed_at;
ALTER TABLE occupancies DROP COLUMN closed_by_user_id;
ALTER TABLE occupancies DROP COLUMN closure_category;
ALTER TABLE occupancies DROP COLUMN closure_reason;
ALTER TABLE occupancies DROP COLUMN closure_state;
