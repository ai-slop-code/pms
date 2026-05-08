-- PMS_12 task 4 / FEAT-05 — persist canonical reservation status.
--
-- The FEAT-04 schema (000021) added the statement-derived columns but
-- forgot the canonical `status` field that the Booking.com Statement
-- carries verbatim ("OK", "Cancelled by guest", "Modified", ...). The
-- merge code computes a normalised value in memory but had nowhere to
-- write it, so the FEAT-05 cancellation/lead-time analytics could not
-- distinguish active stays from cancellations.
--
-- We add the column and backfill from `reservation_status` (which is
-- where the parser already stores the raw "Status" CSV value for
-- statement rows). The value is stored upper-cased so callers can
-- compare without re-normalising.

ALTER TABLE finance_bookings ADD COLUMN status TEXT;

UPDATE finance_bookings
   SET status = UPPER(TRIM(reservation_status))
 WHERE reservation_status IS NOT NULL
   AND TRIM(reservation_status) <> '';

CREATE INDEX idx_finance_bookings_property_status
    ON finance_bookings (property_id, status);
