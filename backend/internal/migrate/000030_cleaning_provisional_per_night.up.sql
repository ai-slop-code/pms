-- PMS_19 §5.4 full provisional cleaning: an unnamed Booking block produces one
-- provisional cleaning checkout per blocked night, so a single occupancy can own
-- several cleaning events. Drop the one-event-per-occupancy unique index; the
-- identity key (property, upstream UID, checkout date, cleaning kind) is now the
-- idempotency guarantee.
DROP INDEX IF EXISTS uq_cleaning_calendar_occupancy;
