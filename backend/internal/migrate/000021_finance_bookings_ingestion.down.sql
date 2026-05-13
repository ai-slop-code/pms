-- Reverse PMS_12 task 4 / FEAT-04 migration. Down path is best-effort:
-- it reverts schema names but cannot recover data dropped from the new
-- columns or audit tables.

DROP INDEX IF EXISTS idx_finance_booking_merges_import;
DROP INDEX IF EXISTS idx_finance_booking_merges_booking;
DROP TABLE IF EXISTS finance_booking_merges;

DROP INDEX IF EXISTS idx_finance_imports_property_uploaded;
DROP TABLE IF EXISTS finance_imports;

DROP INDEX IF EXISTS idx_occupancies_finance_booking_id;
ALTER TABLE occupancies DROP COLUMN finance_booking_id;

ALTER TABLE properties DROP COLUMN booking_hotel_id;

DROP INDEX IF EXISTS ux_finance_bookings_property_channel_reference;

ALTER TABLE finance_bookings DROP COLUMN raw_statement_row_json;
ALTER TABLE finance_bookings RENAME COLUMN raw_payout_row_json TO raw_row_json;

ALTER TABLE finance_bookings DROP COLUMN has_statement_data;
ALTER TABLE finance_bookings DROP COLUMN has_payout_data;
ALTER TABLE finance_bookings DROP COLUMN source_channel;

ALTER TABLE finance_bookings DROP COLUMN country;
ALTER TABLE finance_bookings DROP COLUMN property_label;
ALTER TABLE finance_bookings DROP COLUMN hotel_id;
ALTER TABLE finance_bookings DROP COLUMN invoice_number;
ALTER TABLE finance_bookings DROP COLUMN guest_request;
ALTER TABLE finance_bookings DROP COLUMN booker_name;
ALTER TABLE finance_bookings DROP COLUMN room_nights;
ALTER TABLE finance_bookings DROP COLUMN rooms;
ALTER TABLE finance_bookings DROP COLUMN persons;
ALTER TABLE finance_bookings DROP COLUMN commission_pct;
ALTER TABLE finance_bookings DROP COLUMN original_amount_cents;
ALTER TABLE finance_bookings DROP COLUMN booked_on;

DROP INDEX IF EXISTS idx_finance_bookings_property_occupancy;
DROP INDEX IF EXISTS idx_finance_bookings_property_payout_date;

ALTER TABLE finance_bookings RENAME TO finance_booking_payouts;

CREATE INDEX idx_finance_booking_payouts_property_payout_date
    ON finance_booking_payouts (property_id, payout_date DESC);
CREATE INDEX idx_finance_booking_payouts_property_occupancy
    ON finance_booking_payouts (property_id, occupancy_id);
