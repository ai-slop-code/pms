-- PMS-22: the deployment cleanup command must complete before this schema
-- transition is applied on an existing installation.
CREATE TABLE IF NOT EXISTS cleaning_calendar_cleanup_state (
    property_id INTEGER PRIMARY KEY REFERENCES properties(id) ON DELETE CASCADE,
    cutoff_date TEXT NOT NULL,
    timezone TEXT NOT NULL,
    calendar_id TEXT,
    phase TEXT NOT NULL CHECK (phase IN ('prepared', 'discovered', 'complete')),
    last_error TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cleaning_calendar_cleanup_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES cleaning_calendar_cleanup_state(property_id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL,
    google_event_id TEXT NOT NULL CHECK (length(trim(google_event_id)) > 0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(property_id, calendar_id, google_event_id)
);

CREATE TABLE IF NOT EXISTS cleaning_calendar_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    cleaning_calendar_event_id INTEGER,
    sync_run_id INTEGER REFERENCES cleaning_calendar_sync_runs(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    message TEXT,
    created_at TEXT NOT NULL
);

DELETE FROM cleaning_calendar_event_logs
WHERE cleaning_calendar_event_id IN (
    SELECT id FROM cleaning_calendar_events WHERE named_stay_id IS NULL
);
DELETE FROM cleaning_calendar_events WHERE named_stay_id IS NULL;

ALTER TABLE cleaning_calendar_events RENAME TO cleaning_calendar_events_pms22_old;
ALTER TABLE cleaning_calendar_event_logs RENAME TO cleaning_calendar_event_logs_pms22_old;
CREATE TABLE cleaning_calendar_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    upstream_event_uid TEXT,
    checkout_date TEXT,
    cleaning_kind TEXT NOT NULL DEFAULT 'named_stay' CHECK (cleaning_kind = 'named_stay'),
    google_calendar_id TEXT NOT NULL,
    google_event_id TEXT,
    cleaning_date TEXT NOT NULL,
    starts_at TEXT NOT NULL,
    ends_at TEXT NOT NULL,
    same_day_arrival INTEGER NOT NULL DEFAULT 0,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'synced', 'error', 'removed')),
    warning_message TEXT,
    error_message TEXT,
    pending_action TEXT NOT NULL DEFAULT 'none' CHECK (pending_action IN ('none', 'upsert', 'delete')),
    schedule_hash TEXT,
    last_synced_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    cleaning_identity TEXT,
    desired_hash TEXT,
    last_google_seen_at TEXT,
    UNIQUE(property_id, id),
    FOREIGN KEY (property_id, named_stay_id) REFERENCES named_stays(property_id, id) ON DELETE RESTRICT
);
INSERT INTO cleaning_calendar_events (
    id, property_id, named_stay_id, upstream_event_uid, checkout_date, cleaning_kind,
    google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
    same_day_arrival, title, status, warning_message, error_message,
    pending_action, schedule_hash, last_synced_at, created_at, updated_at,
    cleaning_identity, desired_hash, last_google_seen_at
)
SELECT id, property_id, named_stay_id, upstream_event_uid, checkout_date, 'named_stay',
       google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
       same_day_arrival, title, status, warning_message, error_message,
       CASE
         WHEN status = 'pending' THEN 'upsert'
         WHEN status = 'removed' OR status = 'synced' THEN 'none'
         WHEN EXISTS (SELECT 1 FROM cleaning_calendar_event_logs_pms22_old l WHERE l.cleaning_calendar_event_id = e.id AND l.action = 'delete_error') THEN 'delete'
         WHEN EXISTS (SELECT 1 FROM cleaning_calendar_event_logs_pms22_old l WHERE l.cleaning_calendar_event_id = e.id AND l.action = 'upsert_error') THEN 'upsert'
         ELSE 'none'
       END,
       NULL, last_synced_at, created_at, updated_at,
       cleaning_identity, desired_hash, last_google_seen_at
FROM cleaning_calendar_events_pms22_old e;
DROP TABLE cleaning_calendar_events_pms22_old;
CREATE TABLE cleaning_calendar_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    cleaning_calendar_event_id INTEGER,
    sync_run_id INTEGER REFERENCES cleaning_calendar_sync_runs(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    message TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (property_id, cleaning_calendar_event_id) REFERENCES cleaning_calendar_events(property_id, id) ON DELETE RESTRICT
);
INSERT INTO cleaning_calendar_event_logs
SELECT l.id, l.property_id, l.cleaning_calendar_event_id, l.sync_run_id, l.action, l.message, l.created_at
FROM cleaning_calendar_event_logs_pms22_old l
JOIN cleaning_calendar_events e ON e.property_id = l.property_id AND e.id = l.cleaning_calendar_event_id;
DROP TABLE cleaning_calendar_event_logs_pms22_old;
CREATE INDEX idx_cleaning_calendar_events_property_date ON cleaning_calendar_events(property_id, cleaning_date);
CREATE INDEX idx_cleaning_calendar_events_property_status ON cleaning_calendar_events(property_id, status);
CREATE INDEX idx_cleaning_calendar_events_named_stay ON cleaning_calendar_events(named_stay_id);
CREATE UNIQUE INDEX uq_cleaning_calendar_identity ON cleaning_calendar_events(property_id, upstream_event_uid, checkout_date, cleaning_kind) WHERE upstream_event_uid IS NOT NULL AND checkout_date IS NOT NULL;
CREATE UNIQUE INDEX uq_cleaning_calendar_events_identity ON cleaning_calendar_events(cleaning_identity) WHERE cleaning_identity IS NOT NULL;
CREATE INDEX idx_cleaning_calendar_event_logs_property_created ON cleaning_calendar_event_logs(property_id, created_at DESC);
DROP TABLE cleaning_calendar_cleanup_items;
DROP TABLE cleaning_calendar_cleanup_state;
