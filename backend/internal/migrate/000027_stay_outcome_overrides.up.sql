-- PMS_17 — manual stay outcomes for Booking.com reservations that remain
-- blocked/sold but should not create physical guest operations.
ALTER TABLE occupancies ADD COLUMN stay_outcome TEXT;
ALTER TABLE occupancies ADD COLUMN stay_outcome_reason TEXT;
ALTER TABLE occupancies ADD COLUMN stay_outcome_marked_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE occupancies ADD COLUMN stay_outcome_marked_at TEXT;

CREATE INDEX idx_occupancies_property_stay_outcome
    ON occupancies (property_id, stay_outcome);

-- Optional denormalized visibility for finance views/analytics that read
-- finance_bookings directly. Imported Booking money/status fields stay intact.
ALTER TABLE finance_bookings ADD COLUMN outcome_override TEXT;
ALTER TABLE finance_bookings ADD COLUMN outcome_override_marked_at TEXT;

CREATE INDEX idx_finance_bookings_property_outcome_override
    ON finance_bookings (property_id, outcome_override);
