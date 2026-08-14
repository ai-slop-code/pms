-- PMS 21 Release C: remove the legacy occupancy compatibility schema.
-- The migration runner wraps this file in a transaction. Foreign keys remain
-- enabled throughout; retained children are staged before their parents move.

CREATE TEMP TABLE pms21_guard (ok INTEGER NOT NULL);

-- The database must be structurally sound before any destructive operation.
INSERT INTO pms21_guard SELECT NULL FROM pragma_foreign_key_check LIMIT 1;
INSERT INTO pms21_guard
SELECT NULL FROM pragma_integrity_check WHERE integrity_check <> 'ok' LIMIT 1;

-- Every retained business owner must exist, belong to the same property, and
-- already satisfy the final mandatory-owner rules.
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM nuki_access_codes c
    LEFT JOIN named_stays s ON s.id = c.named_stay_id
    WHERE c.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> c.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM nuki_guest_daily_entries e
    LEFT JOIN named_stays s ON s.id = e.named_stay_id
    WHERE e.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> e.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM finance_bookings b
    LEFT JOIN named_stays s ON s.id = b.named_stay_id
    WHERE b.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> b.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM invoices i
    LEFT JOIN named_stays s ON s.id = i.named_stay_id
    WHERE i.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> i.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM invoices i
    JOIN finance_bookings b ON b.id = i.finance_booking_payout_id
    WHERE b.property_id <> i.property_id OR b.named_stay_id <> i.named_stay_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM cleaning_calendar_events e
    LEFT JOIN named_stays s ON s.id = e.named_stay_id
    LEFT JOIN raw_booking_blocks b ON b.id = e.raw_booking_block_id
    WHERE (e.named_stay_id IS NULL) = (e.raw_booking_block_id IS NULL)
       OR (e.named_stay_id IS NOT NULL AND (s.id IS NULL OR s.property_id <> e.property_id))
       OR (e.raw_booking_block_id IS NOT NULL AND (b.id IS NULL OR b.property_id <> e.property_id))
       OR (e.cleaning_kind = 'named_stay' AND e.named_stay_id IS NULL)
       OR (e.cleaning_kind = 'provisional_block' AND e.raw_booking_block_id IS NULL)
       OR e.cleaning_kind NOT IN ('named_stay', 'provisional_block')
       OR e.next_occupancy_id IS NOT NULL
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM nuki_event_logs l
    JOIN nuki_access_codes c ON c.id = l.nuki_access_code_id
    WHERE c.property_id <> l.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM cleaning_calendar_event_logs l
    JOIN cleaning_calendar_events e ON e.id = l.cleaning_calendar_event_id
    WHERE e.property_id <> l.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM finance_booking_merges m
    JOIN finance_bookings b ON b.id = m.booking_id
    JOIN finance_imports i ON i.id = m.import_id
    WHERE b.property_id <> i.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM named_stays
    WHERE review_resolution IS NULL
       OR first_known_at IS NULL OR trim(first_known_at) = '' OR julianday(first_known_at) IS NULL
       OR (status = 'cancelled' AND
           (cancellation_effective_at IS NULL OR trim(cancellation_effective_at) = ''
            OR julianday(cancellation_effective_at) IS NULL))
);

-- New-model parent/child references and source links must agree before stronger
-- composite foreign keys are installed.
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM named_stay_nights n
    LEFT JOIN named_stays s ON s.id = n.named_stay_id
    WHERE s.id IS NULL OR s.property_id <> n.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM raw_booking_block_nights n
    LEFT JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id
    WHERE b.id IS NULL OR b.property_id <> n.property_id
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    WITH health AS (
        SELECT ns.id AS stay_id, ns.property_id,
               CASE
                   WHEN COUNT(DISTINCT rb.id) = 0
                     OR COUNT(DISTINCT CASE
                         WHEN rbn.active = 1
                          AND rbn.local_night_date >= ns.check_in_date
                          AND rbn.local_night_date < ns.check_out_date
                         THEN rbn.local_night_date END) = 0
                       THEN 'source_deleted'
                   WHEN COUNT(DISTINCT CASE
                         WHEN rbn.active = 1
                          AND rbn.local_night_date >= ns.check_in_date
                          AND rbn.local_night_date < ns.check_out_date
                         THEN rbn.local_night_date END)
                        <> CAST(julianday(ns.check_out_date) - julianday(ns.check_in_date) AS INTEGER)
                       THEN 'conflict'
                   ELSE 'active'
               END AS expected_status
        FROM named_stays ns
        JOIN stay_source_links l
          ON l.named_stay_id = ns.id
         AND l.property_id = ns.property_id
         AND l.link_status <> 'manual_unlinked'
        LEFT JOIN raw_booking_blocks rb
          ON rb.id = l.raw_booking_block_id
         AND rb.property_id = l.property_id
         AND rb.status = 'active'
        LEFT JOIN raw_booking_block_nights rbn
          ON rbn.raw_booking_block_id = rb.id
         AND rbn.property_id = l.property_id
        WHERE ns.status = 'active'
        GROUP BY ns.id, ns.property_id
    )
    SELECT 1
    FROM stay_source_links l
    LEFT JOIN named_stays s ON s.id = l.named_stay_id
    LEFT JOIN raw_booking_blocks b ON b.id = l.raw_booking_block_id
    LEFT JOIN health h
      ON h.stay_id = l.named_stay_id
     AND h.property_id = l.property_id
    WHERE s.id IS NULL OR s.property_id <> l.property_id
       OR date(l.linked_check_in_date) <> l.linked_check_in_date
       OR date(l.linked_check_out_date) <> l.linked_check_out_date
       OR l.linked_check_out_date <= l.linked_check_in_date
       OR l.linked_check_in_date <> s.check_in_date
       OR l.linked_check_out_date <> s.check_out_date
       OR (l.raw_booking_block_id IS NOT NULL AND
           (b.id IS NULL OR b.property_id <> l.property_id
            OR l.source_type IS NOT b.source_type
            OR l.source_event_uid IS NOT b.source_event_uid))
       OR (l.link_status = 'manual_unlinked' AND l.raw_booking_block_id IS NOT NULL)
       OR (l.link_status IN ('active', 'conflict') AND l.raw_booking_block_id IS NULL)
       OR l.link_status NOT IN ('active', 'source_deleted', 'conflict', 'manual_unlinked')
       OR (h.expected_status IS NOT NULL AND l.link_status <> h.expected_status)
);

-- Active owners have exactly their half-open range of active derived nights;
-- inactive owners have none. Date normalization also makes malformed ranges a
-- hard precondition failure instead of allowing a misleading count match.
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM named_stays
    WHERE date(check_in_date) <> check_in_date
       OR date(check_out_date) <> check_out_date
       OR check_out_date <= check_in_date
);
WITH RECURSIVE expected(stay_id, property_id, night_date, end_date) AS (
    SELECT id, property_id, check_in_date, check_out_date
    FROM named_stays WHERE status = 'active'
    UNION ALL
    SELECT stay_id, property_id, date(night_date, '+1 day'), end_date
    FROM expected WHERE date(night_date, '+1 day') < end_date
)
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM expected x
    WHERE NOT EXISTS (
        SELECT 1 FROM named_stay_nights n
        WHERE n.property_id = x.property_id AND n.named_stay_id = x.stay_id
          AND n.local_night_date = x.night_date AND n.active = 1
    )
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM named_stay_nights n
    JOIN named_stays s ON s.id = n.named_stay_id
    WHERE n.active = 1
      AND (s.status <> 'active' OR n.local_night_date < s.check_in_date
           OR n.local_night_date >= s.check_out_date)
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM raw_booking_blocks
    WHERE date(check_in_date) <> check_in_date
       OR date(check_out_date) <> check_out_date
       OR check_out_date <= check_in_date
);
WITH RECURSIVE expected(block_id, property_id, night_date, end_date) AS (
    SELECT id, property_id, check_in_date, check_out_date
    FROM raw_booking_blocks WHERE status = 'active'
    UNION ALL
    SELECT block_id, property_id, date(night_date, '+1 day'), end_date
    FROM expected WHERE date(night_date, '+1 day') < end_date
)
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM expected x
    WHERE NOT EXISTS (
        SELECT 1 FROM raw_booking_block_nights n
        WHERE n.property_id = x.property_id AND n.raw_booking_block_id = x.block_id
          AND n.local_night_date = x.night_date AND n.active = 1
    )
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM raw_booking_block_nights n
    JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id
    WHERE n.active = 1
      AND (b.status <> 'active' OR n.local_night_date < b.check_in_date
           OR n.local_night_date >= b.check_out_date)
);

-- Migration-map rows are accepted only when their legacy row and exactly one
-- kind-appropriate, same-property target still exist. Every legacy row must be
-- accounted for before the attribution map is intentionally discarded.
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM occupancy_stay_migration_map m
    LEFT JOIN occupancies o ON o.id = m.old_occupancy_id
    LEFT JOIN raw_booking_blocks r ON r.id = m.raw_booking_block_id
    LEFT JOIN named_stays s ON s.id = m.named_stay_id
    LEFT JOIN property_availability_blocks a ON a.id = m.availability_block_id
    WHERE m.migration_kind = 'unmapped'
       OR o.id IS NULL OR o.property_id <> m.property_id
       OR (m.raw_booking_block_id IS NOT NULL AND (r.id IS NULL OR r.property_id <> m.property_id))
       OR (m.named_stay_id IS NOT NULL AND (s.id IS NULL OR s.property_id <> m.property_id))
       OR (m.availability_block_id IS NOT NULL AND (a.id IS NULL OR a.property_id <> m.property_id))
       OR ((m.raw_booking_block_id IS NOT NULL) + (m.named_stay_id IS NOT NULL)
           + (m.availability_block_id IS NOT NULL)) <> 1
       OR (m.migration_kind = 'raw_block' AND m.raw_booking_block_id IS NULL)
       OR (m.migration_kind IN ('named_stay', 'synthetic_finance') AND m.named_stay_id IS NULL)
       OR (m.migration_kind IN ('availability_block', 'closure') AND m.availability_block_id IS NULL)
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM occupancies o
    LEFT JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = o.id
    WHERE m.old_occupancy_id IS NULL
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM occupancies o
    JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = o.id
    WHERE (o.closure_state = 'closed' AND m.availability_block_id IS NULL)
       OR (o.closure_state = 'external_sale' AND m.named_stay_id IS NULL)
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM occupancy_stay_migration_map m
    JOIN occupancies o ON o.id = m.old_occupancy_id
    JOIN property_availability_blocks a ON a.id = m.availability_block_id
    WHERE a.source_occupancy_id IS NOT NULL AND a.source_occupancy_id <> o.id
       OR a.property_id <> o.property_id
       OR a.start_date <> substr(o.start_at, 1, 10)
       OR a.end_date <> substr(o.end_at, 1, 10)
);

-- Explicit duplicate guards produce an atomic eligibility failure before any
-- CREATE UNIQUE INDEX or copy can fail part-way through the rebuild plan.
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM nuki_access_codes GROUP BY property_id, named_stay_id
    HAVING named_stay_id IS NOT NULL AND count(*) > 1
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM nuki_guest_daily_entries GROUP BY property_id, named_stay_id, day_date
    HAVING named_stay_id IS NOT NULL AND count(*) > 1
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM invoices GROUP BY property_id, named_stay_id
    HAVING named_stay_id IS NOT NULL AND count(*) > 1
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM invoices GROUP BY property_id, finance_booking_payout_id
    HAVING finance_booking_payout_id IS NOT NULL AND count(*) > 1
);

-- Preserve high-water marks, including deliberately advanced empty-table
-- sequences. Explicit ID copies alone would only restore MAX(id).
CREATE TEMP TABLE pms21_sequences AS
SELECT name, seq FROM sqlite_sequence
WHERE name IN (
    'nuki_access_codes', 'nuki_event_logs', 'nuki_guest_daily_entries',
    'cleaning_calendar_events', 'cleaning_calendar_event_logs',
    'finance_bookings', 'finance_booking_merges', 'invoices', 'invoice_files',
    'property_availability_blocks', 'named_stay_nights',
    'raw_booking_block_nights', 'stay_source_links'
);

-- Composite owner keys required by SQLite parent-key rules.
CREATE UNIQUE INDEX uq_named_stays_property_id ON named_stays (property_id, id);
CREATE UNIQUE INDEX uq_raw_booking_blocks_property_id ON raw_booking_blocks (property_id, id);

CREATE TABLE named_stay_nights_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    local_night_date TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, named_stay_id, local_night_date),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE CASCADE
);
INSERT INTO named_stay_nights_v2
SELECT id, property_id, named_stay_id, local_night_date, active, created_at
FROM named_stay_nights;
DROP TABLE named_stay_nights;
ALTER TABLE named_stay_nights_v2 RENAME TO named_stay_nights;
CREATE UNIQUE INDEX uq_named_stay_nights_active_property_date
    ON named_stay_nights (property_id, local_night_date) WHERE active = 1;
CREATE INDEX idx_named_stay_nights_stay ON named_stay_nights (named_stay_id);

CREATE TABLE raw_booking_block_nights_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    raw_booking_block_id INTEGER NOT NULL,
    local_night_date TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, raw_booking_block_id, local_night_date),
    FOREIGN KEY (property_id, raw_booking_block_id)
        REFERENCES raw_booking_blocks (property_id, id) ON DELETE CASCADE
);
INSERT INTO raw_booking_block_nights_v2
SELECT id, property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at
FROM raw_booking_block_nights;
DROP TABLE raw_booking_block_nights;
ALTER TABLE raw_booking_block_nights_v2 RENAME TO raw_booking_block_nights;
CREATE INDEX idx_raw_booking_block_nights_property_date
    ON raw_booking_block_nights (property_id, local_night_date);
CREATE INDEX idx_raw_booking_block_nights_block
    ON raw_booking_block_nights (raw_booking_block_id);

CREATE TABLE stay_source_links_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    raw_booking_block_id INTEGER,
    source_type TEXT NOT NULL DEFAULT 'booking_ics',
    source_event_uid TEXT,
    linked_check_in_date TEXT NOT NULL,
    linked_check_out_date TEXT NOT NULL,
    link_status TEXT NOT NULL CHECK (link_status IN ('active', 'source_deleted', 'conflict', 'manual_unlinked')) DEFAULT 'active',
    conflict_reason TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (linked_check_out_date > linked_check_in_date),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (property_id, raw_booking_block_id)
        REFERENCES raw_booking_blocks (property_id, id) ON DELETE RESTRICT
);
INSERT INTO stay_source_links_v2
SELECT id, property_id, named_stay_id, raw_booking_block_id, source_type,
       source_event_uid, linked_check_in_date, linked_check_out_date, link_status,
       conflict_reason, created_at, updated_at
FROM stay_source_links;
DROP TABLE stay_source_links;
ALTER TABLE stay_source_links_v2 RENAME TO stay_source_links;
CREATE INDEX idx_stay_source_links_stay ON stay_source_links (named_stay_id);
CREATE INDEX idx_stay_source_links_raw_block ON stay_source_links (raw_booking_block_id);
CREATE INDEX idx_stay_source_links_property_status ON stay_source_links (property_id, link_status);
CREATE INDEX idx_stay_source_links_property_source_uid
    ON stay_source_links (property_id, source_type, source_event_uid);

CREATE TABLE property_availability_blocks_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    block_type TEXT NOT NULL CHECK (block_type IN ('closed', 'off_market')),
    start_date TEXT NOT NULL,
    end_date TEXT NOT NULL,
    reason TEXT,
    status TEXT NOT NULL CHECK (status IN ('active', 'archived')) DEFAULT 'active',
    created_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    updated_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (end_date > start_date)
);
INSERT INTO property_availability_blocks_v2
SELECT id, property_id, block_type, start_date, end_date, reason, status,
       created_by_user_id, updated_by_user_id, created_at, updated_at
FROM property_availability_blocks;
DROP TABLE property_availability_blocks;
ALTER TABLE property_availability_blocks_v2 RENAME TO property_availability_blocks;
CREATE INDEX idx_property_availability_blocks_property_dates
    ON property_availability_blocks (property_id, start_date, end_date);
CREATE INDEX idx_property_availability_blocks_property_status
    ON property_availability_blocks (property_id, status);

-- Nuki code/log parent-child rebuild.
CREATE TEMP TABLE pms21_nuki_event_logs AS SELECT * FROM nuki_event_logs;
DROP TABLE nuki_event_logs;
CREATE TABLE nuki_access_codes_v3 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
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
    UNIQUE (property_id, id),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT
);
INSERT INTO nuki_access_codes_v3
SELECT id, property_id, named_stay_id, code_label, access_code_masked,
       generated_pin_plain, external_nuki_id, valid_from, valid_until, status,
       error_message, last_sync_run_id, created_at, updated_at, revoked_at
FROM nuki_access_codes;
DROP TABLE nuki_access_codes;
ALTER TABLE nuki_access_codes_v3 RENAME TO nuki_access_codes;
CREATE UNIQUE INDEX uq_nuki_access_codes_property_named_stay
    ON nuki_access_codes (property_id, named_stay_id);
CREATE INDEX idx_nuki_access_codes_named_stay ON nuki_access_codes (named_stay_id);
CREATE INDEX idx_nuki_access_codes_property_status ON nuki_access_codes (property_id, status);
CREATE INDEX idx_nuki_access_codes_valid_until ON nuki_access_codes (valid_until);
CREATE TABLE nuki_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    nuki_access_code_id INTEGER,
    sync_run_id INTEGER REFERENCES nuki_sync_runs (id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    message TEXT,
    payload_json TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (property_id, nuki_access_code_id)
        REFERENCES nuki_access_codes (property_id, id) ON DELETE RESTRICT
);
INSERT INTO nuki_event_logs
SELECT id, property_id, nuki_access_code_id, sync_run_id, event_type, message,
       payload_json, created_at
FROM pms21_nuki_event_logs;
CREATE INDEX idx_nuki_event_logs_property_created
    ON nuki_event_logs (property_id, created_at DESC);
DROP TABLE pms21_nuki_event_logs;

CREATE TABLE nuki_guest_daily_entries_v3 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    day_date TEXT NOT NULL,
    first_entry_at TEXT NOT NULL,
    nuki_event_reference TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT
);
INSERT INTO nuki_guest_daily_entries_v3
SELECT id, property_id, named_stay_id, day_date, first_entry_at,
       nuki_event_reference, created_at
FROM nuki_guest_daily_entries;
DROP TABLE nuki_guest_daily_entries;
ALTER TABLE nuki_guest_daily_entries_v3 RENAME TO nuki_guest_daily_entries;
CREATE UNIQUE INDEX uq_nuki_guest_daily_entries_property_named_stay_day
    ON nuki_guest_daily_entries (property_id, named_stay_id, day_date);
CREATE INDEX idx_nuki_guest_daily_entries_named_stay
    ON nuki_guest_daily_entries (named_stay_id);
CREATE INDEX idx_nuki_guest_daily_entries_property_day
    ON nuki_guest_daily_entries (property_id, day_date);

-- Cleaning event/log parent-child rebuild.
CREATE TEMP TABLE pms21_cleaning_event_logs AS SELECT * FROM cleaning_calendar_event_logs;
DROP TABLE cleaning_calendar_event_logs;
CREATE TABLE cleaning_calendar_events_v3 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    upstream_event_uid TEXT,
    checkout_date TEXT,
    cleaning_kind TEXT NOT NULL DEFAULT 'named_stay'
        CHECK (cleaning_kind IN ('named_stay', 'provisional_block')),
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
    last_synced_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    named_stay_id INTEGER,
    raw_booking_block_id INTEGER,
    cleaning_identity TEXT,
    desired_hash TEXT,
    last_google_seen_at TEXT,
    UNIQUE (property_id, id),
    CHECK ((named_stay_id IS NOT NULL) <> (raw_booking_block_id IS NOT NULL)),
    CHECK ((cleaning_kind = 'named_stay' AND named_stay_id IS NOT NULL) OR
           (cleaning_kind = 'provisional_block' AND raw_booking_block_id IS NOT NULL)),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (property_id, raw_booking_block_id)
        REFERENCES raw_booking_blocks (property_id, id) ON DELETE RESTRICT
);
INSERT INTO cleaning_calendar_events_v3
SELECT id, property_id, upstream_event_uid, checkout_date, cleaning_kind,
       google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
       same_day_arrival, title, status, warning_message, error_message,
       last_synced_at, created_at, updated_at, named_stay_id,
       raw_booking_block_id, cleaning_identity, desired_hash, last_google_seen_at
FROM cleaning_calendar_events;
DROP TABLE cleaning_calendar_events;
ALTER TABLE cleaning_calendar_events_v3 RENAME TO cleaning_calendar_events;
CREATE INDEX idx_cleaning_calendar_events_property_date
    ON cleaning_calendar_events (property_id, cleaning_date);
CREATE INDEX idx_cleaning_calendar_events_property_status
    ON cleaning_calendar_events (property_id, status);
CREATE UNIQUE INDEX uq_cleaning_calendar_identity
    ON cleaning_calendar_events (property_id, upstream_event_uid, checkout_date, cleaning_kind)
    WHERE upstream_event_uid IS NOT NULL AND checkout_date IS NOT NULL;
CREATE INDEX idx_cleaning_calendar_events_named_stay
    ON cleaning_calendar_events (named_stay_id);
CREATE INDEX idx_cleaning_calendar_events_raw_block
    ON cleaning_calendar_events (raw_booking_block_id);
CREATE UNIQUE INDEX uq_cleaning_calendar_events_identity
    ON cleaning_calendar_events (cleaning_identity) WHERE cleaning_identity IS NOT NULL;
CREATE TABLE cleaning_calendar_event_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    cleaning_calendar_event_id INTEGER,
    sync_run_id INTEGER REFERENCES cleaning_calendar_sync_runs (id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    message TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (property_id, cleaning_calendar_event_id)
        REFERENCES cleaning_calendar_events (property_id, id) ON DELETE RESTRICT
);
INSERT INTO cleaning_calendar_event_logs
SELECT id, property_id, cleaning_calendar_event_id, sync_run_id, action, message, created_at
FROM pms21_cleaning_event_logs;
CREATE INDEX idx_cleaning_calendar_event_logs_property_created
    ON cleaning_calendar_event_logs (property_id, created_at DESC);
DROP TABLE pms21_cleaning_event_logs;

-- Finance and invoice dependency chain: stage every child before replacing the
-- booking parent, then restore merges, invoices, and files with unchanged IDs.
CREATE TEMP TABLE pms21_finance_booking_merges AS SELECT * FROM finance_booking_merges;
CREATE TEMP TABLE pms21_invoices AS SELECT * FROM invoices;
CREATE TEMP TABLE pms21_invoice_files AS SELECT * FROM invoice_files;
DROP TABLE invoice_files;
DROP TABLE invoices;
DROP TABLE finance_booking_merges;
CREATE TABLE finance_bookings_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    reference_number TEXT NOT NULL,
    payout_id TEXT,
    row_type TEXT,
    check_in_date TEXT,
    check_out_date TEXT,
    guest_name TEXT,
    reservation_status TEXT,
    currency TEXT,
    payment_status TEXT,
    amount_cents INTEGER,
    commission_cents INTEGER,
    payment_service_fee_cents INTEGER,
    net_cents INTEGER NOT NULL,
    payout_date TEXT NOT NULL,
    transaction_id INTEGER REFERENCES finance_transactions (id) ON DELETE SET NULL,
    raw_payout_row_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    booked_on TEXT,
    original_amount_cents INTEGER,
    commission_pct REAL,
    persons INTEGER,
    rooms INTEGER,
    room_nights INTEGER,
    booker_name TEXT,
    guest_request TEXT,
    invoice_number TEXT,
    hotel_id TEXT,
    property_label TEXT,
    country TEXT,
    source_channel TEXT NOT NULL DEFAULT 'booking_com',
    has_payout_data INTEGER NOT NULL DEFAULT 0,
    has_statement_data INTEGER NOT NULL DEFAULT 0,
    raw_statement_row_json TEXT,
    status TEXT,
    outcome_override TEXT,
    outcome_override_marked_at TEXT,
    named_stay_id INTEGER NOT NULL,
    UNIQUE (property_id, reference_number),
    UNIQUE (property_id, id, named_stay_id),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT
);
INSERT INTO finance_bookings_v2
SELECT id, property_id, reference_number, payout_id, row_type, check_in_date,
       check_out_date, guest_name, reservation_status, currency, payment_status,
       amount_cents, commission_cents, payment_service_fee_cents, net_cents,
       payout_date, transaction_id, raw_payout_row_json, created_at, updated_at,
       booked_on, original_amount_cents, commission_pct, persons, rooms,
       room_nights, booker_name, guest_request, invoice_number, hotel_id,
       property_label, country, source_channel, has_payout_data,
       has_statement_data, raw_statement_row_json, status, outcome_override,
       outcome_override_marked_at, named_stay_id
FROM finance_bookings;
DROP TABLE finance_bookings;
ALTER TABLE finance_bookings_v2 RENAME TO finance_bookings;
CREATE INDEX idx_finance_bookings_property_payout_date
    ON finance_bookings (property_id, payout_date DESC);
CREATE UNIQUE INDEX ux_finance_bookings_property_channel_reference
    ON finance_bookings (property_id, source_channel, reference_number);
CREATE INDEX idx_finance_bookings_property_status
    ON finance_bookings (property_id, status);
CREATE INDEX idx_finance_bookings_property_outcome_override
    ON finance_bookings (property_id, outcome_override);
CREATE INDEX idx_finance_bookings_property_named_stay
    ON finance_bookings (property_id, named_stay_id);
CREATE TABLE finance_booking_merges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id INTEGER NOT NULL REFERENCES finance_bookings (id) ON DELETE RESTRICT,
    import_id INTEGER NOT NULL REFERENCES finance_imports (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    changed_fields_json TEXT,
    occurred_at TEXT NOT NULL
);
INSERT INTO finance_booking_merges
SELECT id, booking_id, import_id, source_type, changed_fields_json, occurred_at
FROM pms21_finance_booking_merges;
CREATE INDEX idx_finance_booking_merges_booking ON finance_booking_merges (booking_id);
CREATE INDEX idx_finance_booking_merges_import ON finance_booking_merges (import_id);
DROP TABLE pms21_finance_booking_merges;

CREATE TABLE invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    invoice_number TEXT NOT NULL,
    sequence_year INTEGER NOT NULL,
    sequence_value INTEGER NOT NULL CHECK (sequence_value > 0),
    language TEXT NOT NULL CHECK (language IN ('sk', 'en')),
    issue_date TEXT NOT NULL,
    taxable_supply_date TEXT NOT NULL,
    due_date TEXT NOT NULL,
    stay_start_date TEXT NOT NULL,
    stay_end_date TEXT NOT NULL,
    supplier_snapshot_json TEXT NOT NULL,
    customer_snapshot_json TEXT NOT NULL,
    amount_total_cents INTEGER NOT NULL CHECK (amount_total_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'EUR',
    payment_status TEXT NOT NULL DEFAULT 'paid',
    payment_note TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finance_booking_payout_id INTEGER,
    named_stay_id INTEGER NOT NULL,
    UNIQUE (property_id, invoice_number),
    UNIQUE (property_id, sequence_year, sequence_value),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (property_id, finance_booking_payout_id, named_stay_id)
        REFERENCES finance_bookings (property_id, id, named_stay_id) ON DELETE RESTRICT
);
INSERT INTO invoices
SELECT id, property_id, invoice_number, sequence_year, sequence_value, language,
       issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date,
       supplier_snapshot_json, customer_snapshot_json, amount_total_cents,
       currency, payment_status, payment_note, version, created_by, created_at,
       updated_at, finance_booking_payout_id, named_stay_id
FROM pms21_invoices;
CREATE INDEX idx_invoices_property_issue_date
    ON invoices (property_id, issue_date DESC, id DESC);
CREATE UNIQUE INDEX ux_invoices_property_booking_payout
    ON invoices (property_id, finance_booking_payout_id)
    WHERE finance_booking_payout_id IS NOT NULL;
CREATE INDEX idx_invoices_named_stay ON invoices (named_stay_id);
CREATE UNIQUE INDEX ux_invoices_property_named_stay
    ON invoices (property_id, named_stay_id);
DROP TABLE pms21_invoices;
CREATE TABLE invoice_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL REFERENCES invoices (id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version >= 1),
    file_path TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (invoice_id, version)
);
INSERT INTO invoice_files
SELECT id, invoice_id, version, file_path, file_size_bytes, created_at
FROM pms21_invoice_files;
CREATE INDEX idx_invoice_files_invoice_version ON invoice_files (invoice_id, version DESC);
DROP TABLE pms21_invoice_files;

-- No retained table now references the legacy occupancy model.
DROP TABLE occupancy_nights;
DROP TABLE occupancy_api_tokens;
DROP TABLE occupancies;
DROP TABLE occupancy_stay_migration_map;

DELETE FROM sqlite_sequence
WHERE name IN (SELECT name FROM pms21_sequences);
INSERT INTO sqlite_sequence (name, seq)
SELECT name, seq FROM pms21_sequences;
DROP TABLE pms21_sequences;

-- SQL-verifiable postconditions fail the containing migration transaction.
INSERT INTO pms21_guard SELECT NULL FROM pragma_foreign_key_check LIMIT 1;
INSERT INTO pms21_guard
SELECT NULL FROM pragma_integrity_check WHERE integrity_check <> 'ok' LIMIT 1;
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM sqlite_schema
    WHERE type = 'table'
      AND name IN ('occupancies', 'occupancy_nights', 'occupancy_api_tokens',
                   'occupancy_stay_migration_map')
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1 FROM sqlite_schema
    WHERE lower(coalesce(sql, '')) LIKE '%references occupancies%'
       OR lower(coalesce(sql, '')) LIKE '% next_occupancy_id%'
       OR lower(coalesce(sql, '')) LIKE '% source_occupancy_id%'
       OR lower(coalesce(sql, '')) LIKE '% old_occupancy_id%'
);
INSERT INTO pms21_guard
SELECT NULL WHERE EXISTS (
    SELECT 1
    FROM sqlite_schema s, pragma_table_info(s.name) c
    WHERE s.type = 'table'
      AND c.name IN ('occupancy_id', 'next_occupancy_id', 'source_occupancy_id',
                     'old_occupancy_id')
);
DROP TABLE pms21_guard;
