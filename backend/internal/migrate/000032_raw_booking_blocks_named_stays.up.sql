-- PMS_21 Stage 1: additive raw booking block / named stay schema.
-- This migration intentionally does not remove or disable legacy occupancies.

CREATE TABLE raw_booking_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL DEFAULT 'booking_ics',
    source_event_uid TEXT NOT NULL,
    check_in_date TEXT NOT NULL,
    check_out_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'deleted_from_source', 'conflict')),
    raw_summary TEXT,
    content_hash TEXT NOT NULL,
    source_dtstamp TEXT,
    first_seen_sync_run_id INTEGER REFERENCES occupancy_sync_runs (id) ON DELETE SET NULL,
    last_sync_run_id INTEGER REFERENCES occupancy_sync_runs (id) ON DELETE SET NULL,
    imported_at TEXT NOT NULL,
    last_synced_at TEXT NOT NULL,
    deleted_from_source_at TEXT,
    conflict_reason TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, source_type, source_event_uid)
);

CREATE INDEX idx_raw_booking_blocks_property_dates
    ON raw_booking_blocks (property_id, check_in_date, check_out_date);
CREATE INDEX idx_raw_booking_blocks_property_status
    ON raw_booking_blocks (property_id, status);
CREATE INDEX idx_raw_booking_blocks_property_last_synced
    ON raw_booking_blocks (property_id, last_synced_at DESC);

CREATE TABLE raw_booking_block_nights (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    raw_booking_block_id INTEGER NOT NULL REFERENCES raw_booking_blocks (id) ON DELETE CASCADE,
    local_night_date TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, raw_booking_block_id, local_night_date)
);

CREATE INDEX idx_raw_booking_block_nights_property_date
    ON raw_booking_block_nights (property_id, local_night_date);
CREATE INDEX idx_raw_booking_block_nights_block
    ON raw_booking_block_nights (raw_booking_block_id);

CREATE TABLE named_stays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    stay_type TEXT NOT NULL CHECK (stay_type IN ('booking_com', 'external', 'maintenance', 'personal_use')),
    check_in_date TEXT NOT NULL,
    check_out_date TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled', 'archived')) DEFAULT 'active',
    cleaning_required INTEGER NOT NULL,
    cleaning_override_reason TEXT,
    source_channel TEXT,
    source_reference TEXT,
    manual_revenue_cents INTEGER CHECK (manual_revenue_cents IS NULL OR manual_revenue_cents >= 0),
    manual_revenue_currency TEXT,
    manual_revenue_note TEXT,
    review_status TEXT CHECK (review_status IN ('confirmed', 'needs_review')) DEFAULT 'confirmed',
    review_reason TEXT,
    stay_outcome TEXT CHECK (stay_outcome IN ('cancelled_non_refundable', 'no_show')),
    stay_outcome_reason TEXT,
    stay_outcome_marked_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    stay_outcome_marked_at TEXT,
    nuki_generation_status TEXT CHECK (nuki_generation_status IN ('not_applicable', 'pending', 'generated', 'error')) DEFAULT 'not_applicable',
    nuki_generation_error TEXT,
    nuki_generation_updated_at TEXT,
    created_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (manual_revenue_cents IS NULL OR manual_revenue_currency IS NOT NULL),
    CHECK (check_out_date > check_in_date)
);

CREATE INDEX idx_named_stays_property_dates
    ON named_stays (property_id, check_in_date, check_out_date);
CREATE INDEX idx_named_stays_property_status_check_in
    ON named_stays (property_id, status, check_in_date);
CREATE INDEX idx_named_stays_property_stay_type
    ON named_stays (property_id, stay_type);

CREATE TABLE named_stay_nights (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL REFERENCES named_stays (id) ON DELETE CASCADE,
    local_night_date TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, named_stay_id, local_night_date)
);

CREATE UNIQUE INDEX uq_named_stay_nights_active_property_date
    ON named_stay_nights (property_id, local_night_date)
    WHERE active = 1;
CREATE INDEX idx_named_stay_nights_stay
    ON named_stay_nights (named_stay_id);

CREATE TABLE stay_source_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL REFERENCES named_stays (id) ON DELETE CASCADE,
    raw_booking_block_id INTEGER REFERENCES raw_booking_blocks (id) ON DELETE SET NULL,
    source_type TEXT NOT NULL DEFAULT 'booking_ics',
    source_event_uid TEXT,
    linked_check_in_date TEXT NOT NULL,
    linked_check_out_date TEXT NOT NULL,
    link_status TEXT NOT NULL CHECK (link_status IN ('active', 'source_deleted', 'conflict', 'manual_unlinked')) DEFAULT 'active',
    conflict_reason TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (linked_check_out_date > linked_check_in_date)
);

CREATE INDEX idx_stay_source_links_stay
    ON stay_source_links (named_stay_id);
CREATE INDEX idx_stay_source_links_raw_block
    ON stay_source_links (raw_booking_block_id);
CREATE INDEX idx_stay_source_links_property_status
    ON stay_source_links (property_id, link_status);
CREATE INDEX idx_stay_source_links_property_source_uid
    ON stay_source_links (property_id, source_type, source_event_uid);

CREATE TABLE property_availability_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    block_type TEXT NOT NULL CHECK (block_type IN ('closed', 'off_market')),
    start_date TEXT NOT NULL,
    end_date TEXT NOT NULL,
    reason TEXT,
    source_occupancy_id INTEGER,
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')) DEFAULT 'active',
    created_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (end_date > start_date)
);

CREATE INDEX idx_property_availability_blocks_property_dates
    ON property_availability_blocks (property_id, start_date, end_date);
CREATE INDEX idx_property_availability_blocks_property_status
    ON property_availability_blocks (property_id, status);

CREATE TABLE occupancy_stay_migration_map (
    old_occupancy_id INTEGER NOT NULL,
    property_id INTEGER NOT NULL,
    raw_booking_block_id INTEGER REFERENCES raw_booking_blocks (id) ON DELETE SET NULL,
    named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL,
    availability_block_id INTEGER REFERENCES property_availability_blocks (id) ON DELETE SET NULL,
    migration_kind TEXT NOT NULL CHECK (migration_kind IN ('raw_block', 'named_stay', 'availability_block', 'closure', 'synthetic_finance', 'unmapped')),
    notes TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (old_occupancy_id)
);

CREATE INDEX idx_occupancy_stay_migration_map_property
    ON occupancy_stay_migration_map (property_id);
CREATE INDEX idx_occupancy_stay_migration_map_named_stay
    ON occupancy_stay_migration_map (named_stay_id);
CREATE INDEX idx_occupancy_stay_migration_map_raw_block
    ON occupancy_stay_migration_map (raw_booking_block_id);

ALTER TABLE cleaning_calendar_events ADD COLUMN named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE CASCADE;
ALTER TABLE cleaning_calendar_events ADD COLUMN raw_booking_block_id INTEGER REFERENCES raw_booking_blocks (id) ON DELETE CASCADE;
ALTER TABLE cleaning_calendar_events ADD COLUMN cleaning_identity TEXT;
ALTER TABLE cleaning_calendar_events ADD COLUMN desired_hash TEXT;
ALTER TABLE cleaning_calendar_events ADD COLUMN last_google_seen_at TEXT;

CREATE INDEX idx_cleaning_calendar_events_named_stay
    ON cleaning_calendar_events (named_stay_id);
CREATE INDEX idx_cleaning_calendar_events_raw_block
    ON cleaning_calendar_events (raw_booking_block_id);
CREATE UNIQUE INDEX uq_cleaning_calendar_events_identity
    ON cleaning_calendar_events (cleaning_identity)
    WHERE cleaning_identity IS NOT NULL;

ALTER TABLE nuki_access_codes ADD COLUMN named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL;
CREATE INDEX idx_nuki_access_codes_named_stay
    ON nuki_access_codes (named_stay_id);
CREATE UNIQUE INDEX uq_nuki_access_codes_property_named_stay
    ON nuki_access_codes (property_id, named_stay_id)
    WHERE named_stay_id IS NOT NULL;

ALTER TABLE nuki_guest_daily_entries ADD COLUMN named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL;
CREATE INDEX idx_nuki_guest_daily_entries_named_stay
    ON nuki_guest_daily_entries (named_stay_id);
CREATE UNIQUE INDEX uq_nuki_guest_daily_entries_property_named_stay_day
    ON nuki_guest_daily_entries (property_id, named_stay_id, day_date)
    WHERE named_stay_id IS NOT NULL;

ALTER TABLE finance_bookings ADD COLUMN named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL;
CREATE INDEX idx_finance_bookings_property_named_stay
    ON finance_bookings (property_id, named_stay_id);

ALTER TABLE invoices ADD COLUMN named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL;
CREATE INDEX idx_invoices_named_stay
    ON invoices (named_stay_id);
CREATE UNIQUE INDEX ux_invoices_property_named_stay
    ON invoices (property_id, named_stay_id)
    WHERE named_stay_id IS NOT NULL;
