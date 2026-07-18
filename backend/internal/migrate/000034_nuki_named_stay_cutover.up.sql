-- PMS_21 Stage 7: link existing Nuki data to first-class named stays.
-- Legacy occupancy_id values are intentionally preserved for historical display
-- and rollback compatibility.

UPDATE nuki_access_codes
SET named_stay_id = (
    SELECT osm.named_stay_id
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = nuki_access_codes.property_id
      AND osm.old_occupancy_id = nuki_access_codes.occupancy_id
      AND osm.named_stay_id IS NOT NULL
    LIMIT 1
)
WHERE named_stay_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM occupancy_stay_migration_map osm
      WHERE osm.property_id = nuki_access_codes.property_id
        AND osm.old_occupancy_id = nuki_access_codes.occupancy_id
        AND osm.named_stay_id IS NOT NULL
  );

UPDATE nuki_guest_daily_entries
SET named_stay_id = (
    SELECT osm.named_stay_id
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = nuki_guest_daily_entries.property_id
      AND osm.old_occupancy_id = nuki_guest_daily_entries.occupancy_id
      AND osm.named_stay_id IS NOT NULL
    LIMIT 1
)
WHERE named_stay_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM occupancy_stay_migration_map osm
      WHERE osm.property_id = nuki_guest_daily_entries.property_id
        AND osm.old_occupancy_id = nuki_guest_daily_entries.occupancy_id
        AND osm.named_stay_id IS NOT NULL
  );
