CREATE TABLE nuki_sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL,
    trigger TEXT NOT NULL DEFAULT 'manual',
    error_message TEXT,
    processed_count INTEGER NOT NULL DEFAULT 0,
    created_count INTEGER NOT NULL DEFAULT 0,
    updated_count INTEGER NOT NULL DEFAULT 0,
    revoked_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_nuki_sync_runs_property ON nuki_sync_runs (property_id, started_at DESC);

CREATE TABLE nuki_access_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER NOT NULL REFERENCES occupancies (id) ON DELETE CASCADE,
    code_label TEXT NOT NULL,
    access_code_masked TEXT,
    external_nuki_id TEXT,
    valid_from TEXT NOT NULL,
    valid_until TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('not_generated', 'generated', 'revoked')),
    error_message TEXT,
    last_sync_run_id INTEGER REFERENCES nuki_sync_runs (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revoked_at TEXT,
    UNIQUE (property_id, occupancy_id)
);

CREATE INDEX idx_nuki_access_codes_property_status ON nuki_access_codes (property_id, status);
CREATE INDEX idx_nuki_access_codes_valid_until ON nuki_access_codes (valid_until);

CREATE TABLE nuki_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    nuki_access_code_id INTEGER REFERENCES nuki_access_codes (id) ON DELETE SET NULL,
    sync_run_id INTEGER REFERENCES nuki_sync_runs (id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    message TEXT,
    payload_json TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_nuki_event_logs_property_created ON nuki_event_logs (property_id, created_at DESC);
