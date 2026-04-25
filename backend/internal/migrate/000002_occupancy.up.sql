CREATE TABLE occupancy_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL UNIQUE REFERENCES properties (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL DEFAULT 'booking_ics',
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_occupancy_sources_property ON occupancy_sources (property_id);

CREATE TABLE occupancy_sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL,
    error_message TEXT,
    events_seen INTEGER NOT NULL DEFAULT 0,
    occupancies_upserted INTEGER NOT NULL DEFAULT 0,
    http_status INTEGER,
    trigger TEXT NOT NULL DEFAULT 'scheduled',
    created_at TEXT NOT NULL
);

CREATE INDEX idx_occupancy_sync_runs_property ON occupancy_sync_runs (property_id);
CREATE INDEX idx_occupancy_sync_runs_started ON occupancy_sync_runs (started_at DESC);

CREATE TABLE occupancy_raw_events (
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

CREATE INDEX idx_occupancy_raw_events_property ON occupancy_raw_events (property_id);

CREATE TABLE occupancies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_event_uid TEXT NOT NULL,
    start_at TEXT NOT NULL,
    end_at TEXT NOT NULL,
    status TEXT NOT NULL,
    raw_summary TEXT,
    guest_display_name TEXT,
    content_hash TEXT NOT NULL,
    imported_at TEXT NOT NULL,
    last_synced_at TEXT NOT NULL,
    last_sync_run_id INTEGER REFERENCES occupancy_sync_runs (id) ON DELETE SET NULL,
    UNIQUE (property_id, source_event_uid)
);

CREATE INDEX idx_occupancies_property ON occupancies (property_id);
CREATE INDEX idx_occupancies_property_start ON occupancies (property_id, start_at);

CREATE TABLE occupancy_api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    label TEXT,
    created_at TEXT NOT NULL,
    last_used_at TEXT,
    UNIQUE (property_id, token_hash)
);

CREATE INDEX idx_occupancy_api_tokens_property ON occupancy_api_tokens (property_id);

INSERT INTO occupancy_sources (property_id, source_type, active, created_at, updated_at)
SELECT
    id,
    'booking_ics',
    1,
    strftime ('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime ('%Y-%m-%dT%H:%M:%SZ', 'now')
FROM properties
WHERE
    id NOT IN (
        SELECT property_id
        FROM occupancy_sources
    );
