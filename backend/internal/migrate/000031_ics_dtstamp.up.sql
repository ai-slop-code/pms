-- PMS_19 §6.2 / §7.6: store the upstream DTSTAMP so duplicate resolution can
-- prefer the newest source revision, add a source_type namespace + dtstamp to
-- the raw snapshot table, and tighten its uniqueness to include source_type.

ALTER TABLE occupancies ADD COLUMN source_dtstamp TEXT;

-- Rebuild occupancy_raw_events with source_type + dtstamp columns and the
-- spec §6.2 uniqueness (property_id, sync_run_id, source_type, source_event_uid).
CREATE TABLE occupancy_raw_events_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    sync_run_id INTEGER NOT NULL REFERENCES occupancy_sync_runs (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL DEFAULT 'booking_ics',
    source_event_uid TEXT NOT NULL,
    raw_component TEXT NOT NULL,
    summary TEXT,
    event_start TEXT NOT NULL,
    event_end TEXT NOT NULL,
    sequence_num INTEGER NOT NULL DEFAULT 0,
    ical_status TEXT,
    dtstamp TEXT,
    content_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, sync_run_id, source_type, source_event_uid)
);

INSERT INTO occupancy_raw_events_v2 (
    id, property_id, sync_run_id, source_type, source_event_uid, raw_component, summary,
    event_start, event_end, sequence_num, ical_status, dtstamp, content_hash, created_at)
SELECT id, property_id, sync_run_id, 'booking_ics', source_event_uid, raw_component, summary,
    event_start, event_end, sequence_num, ical_status, NULL, content_hash, created_at
FROM occupancy_raw_events;

DROP TABLE occupancy_raw_events;
ALTER TABLE occupancy_raw_events_v2 RENAME TO occupancy_raw_events;

CREATE INDEX idx_occupancy_raw_events_property ON occupancy_raw_events (property_id);
