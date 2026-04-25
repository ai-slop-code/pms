PRAGMA foreign_keys=off;

ALTER TABLE nuki_access_codes RENAME TO nuki_access_codes_old;

CREATE TABLE nuki_access_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER NOT NULL REFERENCES occupancies (id) ON DELETE CASCADE,
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
    UNIQUE (property_id, occupancy_id)
);

INSERT INTO nuki_access_codes (
    id,
    property_id,
    occupancy_id,
    code_label,
    access_code_masked,
    generated_pin_plain,
    external_nuki_id,
    valid_from,
    valid_until,
    status,
    error_message,
    last_sync_run_id,
    created_at,
    updated_at,
    revoked_at
)
SELECT
    id,
    property_id,
    occupancy_id,
    code_label,
    access_code_masked,
    generated_pin_plain,
    external_nuki_id,
    valid_from,
    valid_until,
    CASE
        WHEN status = 'revoked' THEN 'revoked'
        WHEN COALESCE(TRIM(external_nuki_id), '') != '' THEN 'generated'
        ELSE 'not_generated'
    END AS status,
    error_message,
    last_sync_run_id,
    created_at,
    updated_at,
    revoked_at
FROM nuki_access_codes_old;

DROP TABLE nuki_access_codes_old;

CREATE INDEX idx_nuki_access_codes_property_status ON nuki_access_codes (property_id, status);
CREATE INDEX idx_nuki_access_codes_valid_until ON nuki_access_codes (valid_until);

PRAGMA foreign_keys=on;
