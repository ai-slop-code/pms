DROP INDEX IF EXISTS uq_cleaning_calendar_identity;
DROP INDEX IF EXISTS idx_cleaning_calendar_events_occupancy;

DROP INDEX IF EXISTS uq_occupancy_nights_active;
DROP INDEX IF EXISTS idx_occupancy_nights_property_date;
DROP INDEX IF EXISTS idx_occupancy_nights_occupancy;
DROP TABLE IF EXISTS occupancy_nights;

DROP INDEX IF EXISTS idx_occupancies_superseded;
DROP INDEX IF EXISTS idx_occupancies_upstream_uid;
ALTER TABLE occupancies DROP COLUMN superseded_reason;
ALTER TABLE occupancies DROP COLUMN superseded_at;
ALTER TABLE occupancies DROP COLUMN representation_date;
ALTER TABLE occupancies DROP COLUMN representation_kind;
ALTER TABLE occupancies DROP COLUMN upstream_event_uid;
ALTER TABLE occupancies DROP COLUMN upstream_source_type;

ALTER TABLE occupancy_sync_runs DROP COLUMN deletion_enabled;
ALTER TABLE occupancy_sync_runs DROP COLUMN provisional_cleaning_events_removed;
ALTER TABLE occupancy_sync_runs DROP COLUMN provisional_cleaning_events_created;
ALTER TABLE occupancy_sync_runs DROP COLUMN named_stays_deleted_from_source;
ALTER TABLE occupancy_sync_runs DROP COLUMN legacy_generated_rows_converted;
ALTER TABLE occupancy_sync_runs DROP COLUMN duplicate_nights_resolved;
ALTER TABLE occupancy_sync_runs DROP COLUMN representations_deleted_from_source;
ALTER TABLE occupancy_sync_runs DROP COLUMN representations_superseded;
ALTER TABLE occupancy_sync_runs DROP COLUMN representations_unchanged;
ALTER TABLE occupancy_sync_runs DROP COLUMN representations_updated;
ALTER TABLE occupancy_sync_runs DROP COLUMN representations_inserted;
ALTER TABLE occupancy_sync_runs DROP COLUMN parse_errors;
ALTER TABLE occupancy_sync_runs DROP COLUMN upstream_events_parsed;
