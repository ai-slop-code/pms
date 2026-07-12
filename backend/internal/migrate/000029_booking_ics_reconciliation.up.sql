-- PMS_19 Booking.com ICS reconciliation.
-- Durable upstream ownership + row-identity model, per-night coverage table,
-- cleaning-event identity keys, and richer sync-run counters.

-- 6.1 Durable upstream ownership fields on occupancies.
ALTER TABLE occupancies ADD COLUMN upstream_source_type TEXT;
ALTER TABLE occupancies ADD COLUMN upstream_event_uid TEXT;
ALTER TABLE occupancies ADD COLUMN representation_kind TEXT;
ALTER TABLE occupancies ADD COLUMN representation_date TEXT;
ALTER TABLE occupancies ADD COLUMN superseded_at TEXT;
ALTER TABLE occupancies ADD COLUMN superseded_reason TEXT;

CREATE INDEX idx_occupancies_upstream_uid
    ON occupancies (property_id, upstream_source_type, upstream_event_uid);
CREATE INDEX idx_occupancies_superseded
    ON occupancies (property_id, superseded_at);

-- 6.1.1 Per-night coverage table with the hard capacity-one guarantee.
CREATE TABLE occupancy_nights (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER NOT NULL REFERENCES occupancies (id) ON DELETE CASCADE,
    local_night_date TEXT NOT NULL,
    upstream_source_type TEXT,
    upstream_event_uid TEXT,
    active INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_occupancy_nights_occupancy ON occupancy_nights (occupancy_id);
CREATE INDEX idx_occupancy_nights_property_date ON occupancy_nights (property_id, local_night_date);

-- The capacity-one invariant: at most one active representation per night.
CREATE UNIQUE INDEX uq_occupancy_nights_active
    ON occupancy_nights (property_id, local_night_date)
    WHERE active = 1;

-- 5.4 Cleaning-event identity key. Rebuild the table so occupancy_id is
-- nullable, one block can own several provisional per-night checkouts, and the
-- new identity key (property, upstream UID, checkout date, kind) is unique.
CREATE TABLE cleaning_calendar_events_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE CASCADE,
    upstream_event_uid TEXT,
    checkout_date TEXT,
    cleaning_kind TEXT NOT NULL DEFAULT 'named_stay',
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
    updated_at TEXT NOT NULL
);

INSERT INTO cleaning_calendar_events_v2 (
    id, property_id, occupancy_id, cleaning_kind, google_calendar_id, google_event_id, cleaning_date,
    starts_at, ends_at, same_day_arrival, next_occupancy_id, title, status, warning_message, error_message,
    last_synced_at, created_at, updated_at)
SELECT id, property_id, occupancy_id, 'named_stay', google_calendar_id, google_event_id, cleaning_date,
    starts_at, ends_at, same_day_arrival, next_occupancy_id, title, status, warning_message, error_message,
    last_synced_at, created_at, updated_at
FROM cleaning_calendar_events;

DROP TABLE cleaning_calendar_events;
ALTER TABLE cleaning_calendar_events_v2 RENAME TO cleaning_calendar_events;

CREATE INDEX idx_cleaning_calendar_events_property_date
    ON cleaning_calendar_events (property_id, cleaning_date);
CREATE INDEX idx_cleaning_calendar_events_property_status
    ON cleaning_calendar_events (property_id, status);
CREATE INDEX idx_cleaning_calendar_events_occupancy
    ON cleaning_calendar_events (occupancy_id);

CREATE UNIQUE INDEX uq_cleaning_calendar_identity
    ON cleaning_calendar_events (property_id, upstream_event_uid, checkout_date, cleaning_kind)
    WHERE upstream_event_uid IS NOT NULL AND checkout_date IS NOT NULL;

CREATE UNIQUE INDEX uq_cleaning_calendar_occupancy
    ON cleaning_calendar_events (property_id, occupancy_id)
    WHERE occupancy_id IS NOT NULL;

-- 12 Sync-run observability counters + a visible partial_no_mutation flag.
ALTER TABLE occupancy_sync_runs ADD COLUMN upstream_events_parsed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN parse_errors INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN representations_inserted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN representations_updated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN representations_unchanged INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN representations_superseded INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN representations_deleted_from_source INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN duplicate_nights_resolved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN legacy_generated_rows_converted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN named_stays_deleted_from_source INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN provisional_cleaning_events_created INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN provisional_cleaning_events_removed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN deletion_enabled INTEGER NOT NULL DEFAULT 1;
