DROP INDEX IF EXISTS idx_occupancy_raw_events_property;

CREATE TABLE occupancy_raw_events_v1 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    sync_run_id INTEGER NOT NULL REFERENCES occupancy_sync_runs (id) ON DELETE CASCADE,
    source_event_uid TEXT NOT NULL,
    raw_component TEXT NOT NULL,
    summary TEXT,
    event_start TEXT NOT NULL,
    event_end TEXT NOT NULL,
    sequence_num INTEGER NOT NULL DEFAULT 0,
    ical_status TEXT,
    content_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, sync_run_id, source_event_uid)
);

INSERT INTO occupancy_raw_events_v1 (
    id, property_id, sync_run_id, source_event_uid, raw_component, summary,
    event_start, event_end, sequence_num, ical_status, content_hash, created_at)
SELECT id, property_id, sync_run_id, source_event_uid, raw_component, summary,
    event_start, event_end, sequence_num, ical_status, content_hash, created_at
FROM occupancy_raw_events;

DROP TABLE occupancy_raw_events;
ALTER TABLE occupancy_raw_events_v1 RENAME TO occupancy_raw_events;

CREATE INDEX idx_occupancy_raw_events_property ON occupancy_raw_events (property_id);

ALTER TABLE occupancies DROP COLUMN source_dtstamp;
