-- PMS 38: allow payout-only adjustments without a stay association.
-- Existing stay-backed bookings and invoice relationships are preserved.

DROP INDEX IF EXISTS idx_finance_booking_merges_booking;
DROP INDEX IF EXISTS idx_finance_booking_merges_import;
DROP INDEX IF EXISTS idx_invoices_property_issue_date;
DROP INDEX IF EXISTS idx_invoices_named_stay;
DROP INDEX IF EXISTS ux_invoices_property_booking_payout;
DROP INDEX IF EXISTS idx_invoice_files_invoice_version;
DROP INDEX IF EXISTS idx_finance_bookings_property_payout_date;
DROP INDEX IF EXISTS ux_finance_bookings_property_channel_reference;
DROP INDEX IF EXISTS idx_finance_bookings_property_status;
DROP INDEX IF EXISTS idx_finance_bookings_property_outcome_override;
DROP INDEX IF EXISTS idx_finance_bookings_property_named_stay;

ALTER TABLE invoice_files RENAME TO pms47_invoice_files_old;
ALTER TABLE invoices RENAME TO pms47_invoices_old;
ALTER TABLE finance_booking_merges RENAME TO pms47_finance_booking_merges_old;
ALTER TABLE finance_bookings RENAME TO pms47_finance_bookings_old;

CREATE TABLE finance_bookings (
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
    named_stay_id INTEGER,
    UNIQUE (property_id, reference_number),
    UNIQUE (property_id, id, named_stay_id),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT
);
INSERT INTO finance_bookings SELECT * FROM pms47_finance_bookings_old;

CREATE TABLE finance_booking_merges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    booking_id INTEGER NOT NULL REFERENCES finance_bookings (id) ON DELETE RESTRICT,
    import_id INTEGER NOT NULL REFERENCES finance_imports (id) ON DELETE CASCADE,
    source_type TEXT NOT NULL,
    changed_fields_json TEXT,
    occurred_at TEXT NOT NULL
);
INSERT INTO finance_booking_merges SELECT * FROM pms47_finance_booking_merges_old;

CREATE TABLE invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    finance_booking_payout_id INTEGER,
    document_type TEXT NOT NULL DEFAULT 'invoice' CHECK (document_type IN ('invoice', 'proforma')),
    booking_reference TEXT,
    guest_name TEXT,
    invoice_number TEXT NOT NULL,
    sequence_year INTEGER NOT NULL,
    sequence_value INTEGER NOT NULL CHECK (sequence_value > 0),
    language TEXT NOT NULL CHECK (language IN ('sk', 'en')),
    issue_date TEXT NOT NULL,
    taxable_supply_date TEXT,
    due_date TEXT,
    stay_start_date TEXT NOT NULL,
    stay_end_date TEXT NOT NULL,
    supplier_snapshot_json TEXT NOT NULL,
    customer_snapshot_json TEXT NOT NULL,
    price_details TEXT,
    amount_total_cents INTEGER NOT NULL CHECK (amount_total_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'EUR',
    payment_status TEXT NOT NULL DEFAULT 'paid',
    payment_note TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    line_items_json TEXT,
    UNIQUE (property_id, invoice_number),
    UNIQUE (property_id, sequence_year, sequence_value, document_type),
    UNIQUE (property_id, named_stay_id, document_type),
    FOREIGN KEY (property_id, named_stay_id)
        REFERENCES named_stays (property_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (property_id, finance_booking_payout_id, named_stay_id)
        REFERENCES finance_bookings (property_id, id, named_stay_id) ON DELETE RESTRICT,
    CHECK (length(booking_reference) <= 100),
    CHECK (length(guest_name) <= 200),
    CHECK (length(price_details) <= 1000),
    CHECK (line_items_json IS NULL OR CASE WHEN json_valid(line_items_json)
        THEN json_type(line_items_json) = 'array' AND json_array_length(line_items_json) BETWEEN 1 AND 100 ELSE 0 END),
    CHECK (document_type = 'invoice' OR (
        trim(coalesce(booking_reference, '')) <> '' AND trim(coalesce(guest_name, '')) <> ''
        AND payment_status = 'not_applicable' AND payment_note = '' AND finance_booking_payout_id IS NULL
        AND taxable_supply_date IS NULL AND due_date IS NULL AND amount_total_cents > 0
        AND date(stay_end_date) > date(stay_start_date)
        AND json_extract(supplier_snapshot_json, '$.vat_status') = 'non_vat_payer'))
);
INSERT INTO invoices SELECT * FROM pms47_invoices_old;

CREATE TABLE invoice_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL REFERENCES invoices (id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version >= 1),
    file_path TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (invoice_id, version)
);
INSERT INTO invoice_files SELECT * FROM pms47_invoice_files_old;

DROP TABLE pms47_invoice_files_old;
DROP TABLE pms47_invoices_old;
DROP TABLE pms47_finance_booking_merges_old;
DROP TABLE pms47_finance_bookings_old;

CREATE INDEX idx_finance_bookings_property_payout_date ON finance_bookings (property_id, payout_date DESC);
CREATE UNIQUE INDEX ux_finance_bookings_property_channel_reference ON finance_bookings (property_id, source_channel, reference_number);
CREATE INDEX idx_finance_bookings_property_status ON finance_bookings (property_id, status);
CREATE INDEX idx_finance_bookings_property_outcome_override ON finance_bookings (property_id, outcome_override);
CREATE INDEX idx_finance_bookings_property_named_stay ON finance_bookings (property_id, named_stay_id);
CREATE INDEX idx_finance_booking_merges_booking ON finance_booking_merges (booking_id);
CREATE INDEX idx_finance_booking_merges_import ON finance_booking_merges (import_id);
CREATE INDEX idx_invoices_property_issue_date ON invoices (property_id, issue_date DESC, id DESC);
CREATE INDEX idx_invoices_named_stay ON invoices (named_stay_id);
CREATE UNIQUE INDEX ux_invoices_property_booking_payout ON invoices (property_id, finance_booking_payout_id) WHERE finance_booking_payout_id IS NOT NULL AND document_type = 'invoice';
CREATE INDEX idx_invoice_files_invoice_version ON invoice_files (invoice_id, version DESC);
