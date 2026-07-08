DROP INDEX IF EXISTS idx_finance_bookings_property_outcome_override;
ALTER TABLE finance_bookings DROP COLUMN outcome_override_marked_at;
ALTER TABLE finance_bookings DROP COLUMN outcome_override;

DROP INDEX IF EXISTS idx_occupancies_property_stay_outcome;
ALTER TABLE occupancies DROP COLUMN stay_outcome_marked_at;
ALTER TABLE occupancies DROP COLUMN stay_outcome_marked_by_user_id;
ALTER TABLE occupancies DROP COLUMN stay_outcome_reason;
ALTER TABLE occupancies DROP COLUMN stay_outcome;
