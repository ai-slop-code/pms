CREATE TABLE nuki_keypad_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    external_nuki_id TEXT NOT NULL,
    name TEXT,
    access_code_masked TEXT,
    valid_from TEXT,
    valid_until TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    raw_json TEXT,
    last_seen_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, external_nuki_id)
);

CREATE INDEX idx_nuki_keypad_codes_property ON nuki_keypad_codes (property_id, enabled, valid_until);
