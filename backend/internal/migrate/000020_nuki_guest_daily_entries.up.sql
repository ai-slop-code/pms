-- PMS v1.1 task #3 — guest check-in heatmap.
-- Mirrors cleaning_daily_logs but is keyed by (property_id, occupancy_id, day_date)
-- so that multiple stays staying in the same property on the same day each
-- contribute their first guest unlock independently. The hour-of-day analytics
-- aggregate is built on top of these rows.
CREATE TABLE nuki_guest_daily_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER NOT NULL REFERENCES occupancies (id) ON DELETE CASCADE,
    day_date TEXT NOT NULL,
    first_entry_at TEXT NOT NULL,
    nuki_event_reference TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, occupancy_id, day_date)
);

CREATE INDEX idx_nuki_guest_daily_entries_property_day
    ON nuki_guest_daily_entries (property_id, day_date);
