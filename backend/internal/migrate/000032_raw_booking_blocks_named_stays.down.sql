DROP INDEX IF EXISTS ux_invoices_property_named_stay;
DROP INDEX IF EXISTS idx_invoices_named_stay;
ALTER TABLE invoices DROP COLUMN named_stay_id;

DROP INDEX IF EXISTS idx_finance_bookings_property_named_stay;
ALTER TABLE finance_bookings DROP COLUMN named_stay_id;

DROP INDEX IF EXISTS uq_nuki_guest_daily_entries_property_named_stay_day;
DROP INDEX IF EXISTS idx_nuki_guest_daily_entries_named_stay;
ALTER TABLE nuki_guest_daily_entries DROP COLUMN named_stay_id;

DROP INDEX IF EXISTS uq_nuki_access_codes_property_named_stay;
DROP INDEX IF EXISTS idx_nuki_access_codes_named_stay;
ALTER TABLE nuki_access_codes DROP COLUMN named_stay_id;

DROP INDEX IF EXISTS uq_cleaning_calendar_events_identity;
DROP INDEX IF EXISTS idx_cleaning_calendar_events_raw_block;
DROP INDEX IF EXISTS idx_cleaning_calendar_events_named_stay;
ALTER TABLE cleaning_calendar_events DROP COLUMN last_google_seen_at;
ALTER TABLE cleaning_calendar_events DROP COLUMN desired_hash;
ALTER TABLE cleaning_calendar_events DROP COLUMN cleaning_identity;
ALTER TABLE cleaning_calendar_events DROP COLUMN raw_booking_block_id;
ALTER TABLE cleaning_calendar_events DROP COLUMN named_stay_id;

DROP TABLE IF EXISTS occupancy_stay_migration_map;
DROP TABLE IF EXISTS property_availability_blocks;
DROP TABLE IF EXISTS stay_source_links;
DROP TABLE IF EXISTS named_stay_nights;
DROP TABLE IF EXISTS named_stays;
DROP TABLE IF EXISTS raw_booking_block_nights;
DROP TABLE IF EXISTS raw_booking_blocks;
