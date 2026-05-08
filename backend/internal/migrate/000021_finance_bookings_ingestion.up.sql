-- PMS_12 task 4 / FEAT-04 — Booking.com Statement ingestion + merge.
-- Renames the cash-only payout table to a lifecycle-aware bookings table,
-- adds the canonical statement columns + source flags, and introduces the
-- import-audit and merge-audit tables.

-- 1. Rename payout table to canonical bookings table. SQLite >=3.25
-- automatically updates FKs from other tables that reference the old name
-- (invoices.finance_booking_payout_id keeps its column name to minimise
-- churn; only the referenced table changes).
ALTER TABLE finance_booking_payouts RENAME TO finance_bookings;

DROP INDEX IF EXISTS idx_finance_booking_payouts_property_payout_date;
DROP INDEX IF EXISTS idx_finance_booking_payouts_property_occupancy;
CREATE INDEX idx_finance_bookings_property_payout_date
    ON finance_bookings (property_id, payout_date DESC);
CREATE INDEX idx_finance_bookings_property_occupancy
    ON finance_bookings (property_id, occupancy_id);

-- 2. Statement-derived canonical columns.
ALTER TABLE finance_bookings ADD COLUMN booked_on TEXT;
ALTER TABLE finance_bookings ADD COLUMN original_amount_cents INTEGER;
ALTER TABLE finance_bookings ADD COLUMN commission_pct REAL;
ALTER TABLE finance_bookings ADD COLUMN persons INTEGER;
ALTER TABLE finance_bookings ADD COLUMN rooms INTEGER;
ALTER TABLE finance_bookings ADD COLUMN room_nights INTEGER;
ALTER TABLE finance_bookings ADD COLUMN booker_name TEXT;
ALTER TABLE finance_bookings ADD COLUMN guest_request TEXT;
ALTER TABLE finance_bookings ADD COLUMN invoice_number TEXT;
ALTER TABLE finance_bookings ADD COLUMN hotel_id TEXT;
ALTER TABLE finance_bookings ADD COLUMN property_label TEXT;
ALTER TABLE finance_bookings ADD COLUMN country TEXT;

-- 3. Source-channel + presence flags.
ALTER TABLE finance_bookings ADD COLUMN source_channel TEXT NOT NULL DEFAULT 'booking_com';
ALTER TABLE finance_bookings ADD COLUMN has_payout_data INTEGER NOT NULL DEFAULT 0;
ALTER TABLE finance_bookings ADD COLUMN has_statement_data INTEGER NOT NULL DEFAULT 0;

-- 4. Replace single raw JSON column with one per source.
ALTER TABLE finance_bookings RENAME COLUMN raw_row_json TO raw_payout_row_json;
ALTER TABLE finance_bookings ADD COLUMN raw_statement_row_json TEXT;

-- 5. Backfill source flags from existing data: any pre-existing payout row
-- has by definition come from the Payout file.
UPDATE finance_bookings
   SET has_payout_data = 1
 WHERE raw_payout_row_json IS NOT NULL OR payout_id IS NOT NULL OR net_cents IS NOT NULL;

-- 6. Wider canonical merge key. Existing UNIQUE(property_id, reference_number)
-- is retained because today's only channel is booking_com; a follow-up
-- migration will drop it once a second channel actually lands.
CREATE UNIQUE INDEX ux_finance_bookings_property_channel_reference
    ON finance_bookings (property_id, source_channel, reference_number);

-- 7. Capture the observed Hotel id on first statement upload for a property.
ALTER TABLE properties ADD COLUMN booking_hotel_id TEXT;

-- 8. Explicit FK from occupancies to the canonical bookings row.
ALTER TABLE occupancies
    ADD COLUMN finance_booking_id INTEGER
    REFERENCES finance_bookings (id) ON DELETE SET NULL;

UPDATE occupancies
   SET finance_booking_id = (
       SELECT fb.id FROM finance_bookings fb
        WHERE fb.occupancy_id = occupancies.id
        LIMIT 1
   )
 WHERE finance_booking_id IS NULL;

CREATE INDEX idx_occupancies_finance_booking_id
    ON occupancies (finance_booking_id);

-- 9. Per-upload import audit row.
CREATE TABLE finance_imports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    source_channel TEXT NOT NULL DEFAULT 'booking_com',
    hotel_id TEXT,
    invoice_number TEXT,
    period_start TEXT,
    period_end TEXT,
    uploaded_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    uploaded_at TEXT NOT NULL,
    file_sha256 TEXT,
    row_count_total INTEGER NOT NULL DEFAULT 0,
    row_count_inserted INTEGER NOT NULL DEFAULT 0,
    row_count_updated INTEGER NOT NULL DEFAULT 0,
    row_count_unchanged INTEGER NOT NULL DEFAULT 0,
    row_count_skipped_other_hotel INTEGER NOT NULL DEFAULT 0,
    row_count_rejected INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_finance_imports_property_uploaded
    ON finance_imports (property_id, uploaded_at DESC);

-- 10. Merge audit log (one row per booking row affected by a commit).
CREATE TABLE finance_booking_merges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id INTEGER NOT NULL REFERENCES finance_bookings (id) ON DELETE CASCADE,
    import_id INTEGER NOT NULL REFERENCES finance_imports (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    changed_fields_json TEXT,
    occurred_at TEXT NOT NULL
);

CREATE INDEX idx_finance_booking_merges_booking
    ON finance_booking_merges (booking_id);
CREATE INDEX idx_finance_booking_merges_import
    ON finance_booking_merges (import_id);
