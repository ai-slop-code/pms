CREATE TABLE property_google_cleaning_settings (
    property_id INTEGER PRIMARY KEY REFERENCES properties (id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    calendar_id TEXT,
    default_duration_minutes INTEGER NOT NULL DEFAULT 180,
    title_prefix TEXT NOT NULL DEFAULT 'Upratovanie:',
    same_day_label TEXT NOT NULL DEFAULT 'Pride Host',
    no_guest_label TEXT NOT NULL DEFAULT 'Bez Hosta',
    connected_account_id TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE cleaning_calendar_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER NOT NULL REFERENCES occupancies (id) ON DELETE CASCADE,
    google_calendar_id TEXT NOT NULL,
    google_event_id TEXT,
    cleaning_date TEXT NOT NULL,
    starts_at TEXT NOT NULL,
    ends_at TEXT NOT NULL,
    same_day_arrival INTEGER NOT NULL DEFAULT 0,
    next_occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'synced', 'error', 'removed')),
    warning_message TEXT,
    error_message TEXT,
    last_synced_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, occupancy_id)
);

CREATE INDEX idx_cleaning_calendar_events_property_date
ON cleaning_calendar_events (property_id, cleaning_date);

CREATE INDEX idx_cleaning_calendar_events_property_status
ON cleaning_calendar_events (property_id, status);

CREATE TABLE cleaning_calendar_sync_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL CHECK (status IN ('running', 'success', 'partial', 'failure')),
    error_message TEXT,
    events_seen INTEGER NOT NULL DEFAULT 0,
    events_upserted INTEGER NOT NULL DEFAULT 0,
    events_removed INTEGER NOT NULL DEFAULT 0,
    trigger TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_cleaning_calendar_sync_runs_property_started
ON cleaning_calendar_sync_runs (property_id, started_at DESC);

CREATE TABLE cleaning_calendar_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    cleaning_calendar_event_id INTEGER REFERENCES cleaning_calendar_events (id) ON DELETE SET NULL,
    sync_run_id INTEGER REFERENCES cleaning_calendar_sync_runs (id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    message TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_cleaning_calendar_event_logs_property_created
ON cleaning_calendar_event_logs (property_id, created_at DESC);
