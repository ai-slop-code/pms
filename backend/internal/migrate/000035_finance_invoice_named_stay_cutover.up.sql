-- PMS_21 Stage 8: finance, payout, and invoice named-stay cutover backfill.
-- Legacy occupancy_id columns remain for rollback and compatibility.

UPDATE finance_bookings
SET named_stay_id = (
    SELECT osm.named_stay_id
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = finance_bookings.property_id
      AND osm.old_occupancy_id = finance_bookings.occupancy_id
      AND osm.named_stay_id IS NOT NULL
    LIMIT 1
)
WHERE named_stay_id IS NULL
  AND occupancy_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = finance_bookings.property_id
      AND osm.old_occupancy_id = finance_bookings.occupancy_id
      AND osm.named_stay_id IS NOT NULL
  );

UPDATE invoices
SET named_stay_id = (
    SELECT osm.named_stay_id
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = invoices.property_id
      AND osm.old_occupancy_id = invoices.occupancy_id
      AND osm.named_stay_id IS NOT NULL
    LIMIT 1
)
WHERE named_stay_id IS NULL
  AND occupancy_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM occupancy_stay_migration_map osm
    WHERE osm.property_id = invoices.property_id
      AND osm.old_occupancy_id = invoices.occupancy_id
      AND osm.named_stay_id IS NOT NULL
  );

UPDATE invoices
SET named_stay_id = (
    SELECT fb.named_stay_id
    FROM finance_bookings fb
    WHERE fb.property_id = invoices.property_id
      AND fb.id = invoices.finance_booking_payout_id
      AND fb.named_stay_id IS NOT NULL
    LIMIT 1
)
WHERE named_stay_id IS NULL
  AND finance_booking_payout_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM finance_bookings fb
    WHERE fb.property_id = invoices.property_id
      AND fb.id = invoices.finance_booking_payout_id
      AND fb.named_stay_id IS NOT NULL
  );
