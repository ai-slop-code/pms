-- PMS_21 remediation: make named stays the primary Nuki identity while
-- preserving every access-code ID and all historical legacy attribution.

CREATE TEMP TABLE pms21_nuki_event_logs AS
SELECT id, property_id, nuki_access_code_id, sync_run_id, event_type, message,
       payload_json, created_at
FROM nuki_event_logs;

-- Migration 000007 renamed the access-code table while foreign keys were
-- disabled, so older SQLite databases may still name nuki_access_codes_old as
-- this child's parent. Rebuilding the child fixes that latent reference.
DROP TABLE nuki_event_logs;

CREATE TABLE nuki_access_codes_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE SET NULL,
    named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL,
    code_label TEXT NOT NULL,
    access_code_masked TEXT,
    generated_pin_plain TEXT,
    external_nuki_id TEXT,
    valid_from TEXT NOT NULL,
    valid_until TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('not_generated', 'generated', 'revoked')),
    error_message TEXT,
    last_sync_run_id INTEGER REFERENCES nuki_sync_runs (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revoked_at TEXT,
    CHECK (occupancy_id IS NOT NULL OR named_stay_id IS NOT NULL)
);

INSERT INTO nuki_access_codes_v2 (
    id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked,
    generated_pin_plain, external_nuki_id, valid_from, valid_until, status,
    error_message, last_sync_run_id, created_at, updated_at, revoked_at
)
SELECT
    id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked,
    generated_pin_plain, external_nuki_id, valid_from, valid_until, status,
    error_message, last_sync_run_id, created_at, updated_at, revoked_at
FROM nuki_access_codes;

DROP TABLE nuki_access_codes;
ALTER TABLE nuki_access_codes_v2 RENAME TO nuki_access_codes;

CREATE UNIQUE INDEX uq_nuki_access_codes_property_named_stay
    ON nuki_access_codes (property_id, named_stay_id)
    WHERE named_stay_id IS NOT NULL;
CREATE UNIQUE INDEX uq_nuki_access_codes_property_legacy_occupancy
    ON nuki_access_codes (property_id, occupancy_id)
    WHERE named_stay_id IS NULL AND occupancy_id IS NOT NULL;
CREATE INDEX idx_nuki_access_codes_named_stay
    ON nuki_access_codes (named_stay_id);
CREATE INDEX idx_nuki_access_codes_property_status
    ON nuki_access_codes (property_id, status);
CREATE INDEX idx_nuki_access_codes_valid_until
    ON nuki_access_codes (valid_until);

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

INSERT INTO nuki_event_logs (
    id, property_id, nuki_access_code_id, sync_run_id, event_type, message,
    payload_json, created_at
)
SELECT id, property_id, nuki_access_code_id, sync_run_id, event_type, message,
       payload_json, created_at
FROM pms21_nuki_event_logs;

CREATE INDEX idx_nuki_event_logs_property_created
    ON nuki_event_logs (property_id, created_at DESC);

DROP TABLE pms21_nuki_event_logs;

CREATE TABLE nuki_guest_daily_entries_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE SET NULL,
    named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL,
    day_date TEXT NOT NULL,
    first_entry_at TEXT NOT NULL,
    nuki_event_reference TEXT,
    created_at TEXT NOT NULL,
    CHECK (occupancy_id IS NOT NULL OR named_stay_id IS NOT NULL)
);

INSERT INTO nuki_guest_daily_entries_v2 (
    id, property_id, occupancy_id, named_stay_id, day_date, first_entry_at,
    nuki_event_reference, created_at
)
SELECT
    id, property_id, occupancy_id, named_stay_id, day_date, first_entry_at,
    nuki_event_reference, created_at
FROM nuki_guest_daily_entries;

DROP TABLE nuki_guest_daily_entries;
ALTER TABLE nuki_guest_daily_entries_v2 RENAME TO nuki_guest_daily_entries;

CREATE UNIQUE INDEX uq_nuki_guest_daily_entries_property_named_stay_day
    ON nuki_guest_daily_entries (property_id, named_stay_id, day_date)
    WHERE named_stay_id IS NOT NULL;
CREATE UNIQUE INDEX uq_nuki_guest_daily_entries_property_legacy_occupancy_day
    ON nuki_guest_daily_entries (property_id, occupancy_id, day_date)
    WHERE named_stay_id IS NULL AND occupancy_id IS NOT NULL;
CREATE INDEX idx_nuki_guest_daily_entries_named_stay
    ON nuki_guest_daily_entries (named_stay_id);
CREATE INDEX idx_nuki_guest_daily_entries_property_day
    ON nuki_guest_daily_entries (property_id, day_date);
