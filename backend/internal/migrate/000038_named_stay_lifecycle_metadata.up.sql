-- PMS 21 Release A/B: canonical named-stay lifecycle attribution.
ALTER TABLE named_stays ADD COLUMN first_known_at TEXT;
ALTER TABLE named_stays ADD COLUMN cancellation_effective_at TEXT;
ALTER TABLE named_stays ADD COLUMN stay_outcome_actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE named_stays ADD COLUMN review_actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE named_stays ADD COLUMN reviewed_at TEXT;
ALTER TABLE named_stays ADD COLUMN review_resolution TEXT CHECK (review_resolution IN ('confirmed', 'rejected'));

-- stay_outcome_reason was introduced with named_stays in 000032. Keep that
-- column in place and backfill the new canonical actor from its legacy name.
UPDATE named_stays
SET stay_outcome_actor_user_id = stay_outcome_marked_by_user_id
WHERE stay_outcome_actor_user_id IS NULL
  AND stay_outcome_marked_by_user_id IS NOT NULL;

ALTER TABLE named_stays DROP COLUMN stay_outcome_marked_by_user_id;

-- A confirmed legacy review state is already resolved. needs_review remains
-- unresolved until an explicit confirmed/rejected decision is recorded.
UPDATE named_stays
SET review_resolution = 'confirmed'
WHERE review_status = 'confirmed';

-- Earliest reliable knowledge wins across every source; source priority must
-- not hide an older timestamp from another source.
UPDATE named_stays
SET first_known_at = (
    SELECT evidence_at
    FROM (
        SELECT fb.booked_on AS evidence_at
        FROM finance_bookings fb
        WHERE fb.named_stay_id = named_stays.id
          AND fb.property_id = named_stays.property_id
          AND fb.booked_on IS NOT NULL AND trim(fb.booked_on) <> ''
          AND julianday(fb.booked_on) IS NOT NULL
        UNION ALL
        SELECT fb.booked_on
        FROM occupancy_stay_migration_map osm
        JOIN finance_bookings fb
          ON fb.property_id = osm.property_id
         AND fb.occupancy_id = osm.old_occupancy_id
        WHERE osm.named_stay_id = named_stays.id
          AND osm.property_id = named_stays.property_id
          AND fb.booked_on IS NOT NULL AND trim(fb.booked_on) <> ''
          AND julianday(fb.booked_on) IS NOT NULL
        UNION ALL
        SELECT o.imported_at
        FROM occupancy_stay_migration_map osm
        JOIN occupancies o ON o.id = osm.old_occupancy_id
        WHERE osm.named_stay_id = named_stays.id
          AND osm.property_id = named_stays.property_id
          AND o.property_id = named_stays.property_id
          AND o.imported_at IS NOT NULL AND trim(o.imported_at) <> ''
          AND julianday(o.imported_at) IS NOT NULL
        UNION ALL
        SELECT rb.imported_at
        FROM stay_source_links ssl
        JOIN raw_booking_blocks rb ON rb.id = ssl.raw_booking_block_id
        WHERE ssl.named_stay_id = named_stays.id
          AND ssl.property_id = named_stays.property_id
          AND rb.property_id = named_stays.property_id
          AND rb.imported_at IS NOT NULL AND trim(rb.imported_at) <> ''
          AND julianday(rb.imported_at) IS NOT NULL
        UNION ALL
        SELECT named_stays.created_at
        WHERE (named_stays.created_by_user_id IS NOT NULL
               OR (lower(trim(COALESCE(named_stays.source_channel, ''))) = 'manual'
                   AND NOT EXISTS (
                       SELECT 1
                       FROM finance_bookings fb
                       WHERE fb.named_stay_id = named_stays.id
                         AND fb.property_id = named_stays.property_id
                   )))
          AND NOT EXISTS (
              SELECT 1
              FROM occupancy_stay_migration_map osm
              WHERE osm.named_stay_id = named_stays.id
          )
          AND named_stays.created_at IS NOT NULL AND trim(named_stays.created_at) <> ''
          AND julianday(named_stays.created_at) IS NOT NULL
    ) evidence
    ORDER BY julianday(evidence_at) IS NULL, julianday(evidence_at), evidence_at
    LIMIT 1
)
WHERE first_known_at IS NULL OR trim(first_known_at) = '';

UPDATE named_stays
SET cancellation_effective_at = (
    SELECT evidence_at
    FROM (
            SELECT named_stays.stay_outcome_marked_at AS evidence_at
            WHERE named_stays.stay_outcome IS NOT NULL
              AND named_stays.stay_outcome_marked_at IS NOT NULL
               AND trim(named_stays.stay_outcome_marked_at) <> ''
               AND julianday(named_stays.stay_outcome_marked_at) IS NOT NULL
            UNION ALL
            SELECT o.stay_outcome_marked_at
            FROM occupancy_stay_migration_map osm
            JOIN occupancies o ON o.id = osm.old_occupancy_id
            WHERE osm.named_stay_id = named_stays.id
              AND osm.property_id = named_stays.property_id
              AND o.property_id = named_stays.property_id
              AND o.stay_outcome IS NOT NULL
              AND o.stay_outcome_marked_at IS NOT NULL
               AND trim(o.stay_outcome_marked_at) <> ''
               AND julianday(o.stay_outcome_marked_at) IS NOT NULL
            UNION ALL
            SELECT fb.outcome_override_marked_at
            FROM finance_bookings fb
            WHERE fb.named_stay_id = named_stays.id
              AND fb.property_id = named_stays.property_id
              AND fb.outcome_override IS NOT NULL
              AND fb.outcome_override_marked_at IS NOT NULL
               AND trim(fb.outcome_override_marked_at) <> ''
               AND julianday(fb.outcome_override_marked_at) IS NOT NULL
            UNION ALL
            SELECT fb.outcome_override_marked_at
            FROM occupancy_stay_migration_map osm
            JOIN finance_bookings fb
              ON fb.property_id = osm.property_id
             AND fb.occupancy_id = osm.old_occupancy_id
            WHERE osm.named_stay_id = named_stays.id
              AND osm.property_id = named_stays.property_id
              AND fb.outcome_override IS NOT NULL
              AND fb.outcome_override_marked_at IS NOT NULL
               AND trim(fb.outcome_override_marked_at) <> ''
               AND julianday(fb.outcome_override_marked_at) IS NOT NULL
            UNION ALL
            SELECT o.last_synced_at
            FROM occupancy_stay_migration_map osm
            JOIN occupancies o ON o.id = osm.old_occupancy_id
            WHERE osm.named_stay_id = named_stays.id
              AND osm.property_id = named_stays.property_id
              AND o.property_id = named_stays.property_id
              AND lower(trim(o.status)) = 'cancelled'
              AND o.last_synced_at IS NOT NULL
               AND trim(o.last_synced_at) <> ''
               AND julianday(o.last_synced_at) IS NOT NULL
            UNION ALL
            SELECT rb.deleted_from_source_at
            FROM stay_source_links ssl
            JOIN raw_booking_blocks rb ON rb.id = ssl.raw_booking_block_id
            WHERE ssl.named_stay_id = named_stays.id
              AND ssl.property_id = named_stays.property_id
              AND rb.property_id = named_stays.property_id
              AND ssl.link_status = 'source_deleted'
              AND rb.status = 'deleted_from_source'
              AND rb.deleted_from_source_at IS NOT NULL
               AND trim(rb.deleted_from_source_at) <> ''
               AND julianday(rb.deleted_from_source_at) IS NOT NULL
    ) evidence
    ORDER BY julianday(evidence_at) IS NULL, julianday(evidence_at), evidence_at
    LIMIT 1
)
WHERE status = 'cancelled'
  AND cancellation_effective_at IS NULL;
