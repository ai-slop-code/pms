-- PMS_14 / PMS_12 §2 — let operators classify a Booking.com block as either
-- "closed" (off the market, drops out of occupancy/ADR/RevPAR) or
-- "external_sale" (sold via a different channel, with an operator-entered
-- net amount that feeds gross revenue while keeping the night sold +
-- available). The two labels share a single `closure_state` column so
-- analytics queries stay a clean two-way split. Only one label is ever
-- set on a given row; clearing it ("reopen") nulls everything in this
-- migration.
ALTER TABLE occupancies ADD COLUMN closure_state TEXT;
ALTER TABLE occupancies ADD COLUMN closure_reason TEXT;
ALTER TABLE occupancies ADD COLUMN closure_category TEXT;
ALTER TABLE occupancies ADD COLUMN closed_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE occupancies ADD COLUMN closed_at TEXT;

-- External-sale columns. NULL unless `closure_state = 'external_sale'`.
-- Money is INTEGER cents per PMS_13 §5.
ALTER TABLE occupancies ADD COLUMN external_net_amount_cents INTEGER;
ALTER TABLE occupancies ADD COLUMN external_currency TEXT;
ALTER TABLE occupancies ADD COLUMN external_channel TEXT;

-- Index keeps closure-aware analytics scans cheap; the column is NULL for
-- the vast majority of rows so the index stays selective.
CREATE INDEX idx_occupancies_property_closure ON occupancies (property_id, closure_state);
