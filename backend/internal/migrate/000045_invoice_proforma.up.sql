-- PMS 36: extend the existing invoice aggregate with pro forma documents.
-- The migration runner defers foreign-key checks for this rebuild so invoice
-- files can retain their IDs while the invoice parent is replaced.

CREATE TEMP TABLE pms36_sequences AS
SELECT name, seq FROM sqlite_sequence
WHERE name IN ('invoice_sequences', 'invoices', 'invoice_files');

ALTER TABLE invoice_sequences RENAME TO pms36_invoice_sequences_old;
DROP INDEX IF EXISTS idx_invoice_sequences_property_year;
CREATE TABLE invoice_sequences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    document_type TEXT NOT NULL DEFAULT 'invoice' CHECK (document_type IN ('invoice', 'proforma')),
    sequence_year INTEGER NOT NULL,
    current_value INTEGER NOT NULL DEFAULT 0 CHECK (current_value >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, document_type, sequence_year)
);
INSERT INTO invoice_sequences (id, property_id, document_type, sequence_year, current_value, created_at, updated_at)
SELECT id, property_id, 'invoice', sequence_year, current_value, created_at, updated_at
FROM pms36_invoice_sequences_old;
CREATE INDEX idx_invoice_sequences_property_year
ON invoice_sequences (property_id, document_type, sequence_year DESC);
DROP TABLE pms36_invoice_sequences_old;

ALTER TABLE invoice_files RENAME TO pms36_invoice_files_old;
ALTER TABLE invoices RENAME TO pms36_invoices_old;
DROP INDEX IF EXISTS idx_invoices_property_issue_date;
DROP INDEX IF EXISTS idx_invoices_named_stay;
DROP INDEX IF EXISTS ux_invoices_property_booking_payout;
DROP INDEX IF EXISTS ux_invoices_property_named_stay;
DROP INDEX IF EXISTS idx_invoice_files_invoice_version;

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
    CHECK (document_type = 'invoice' OR (
        trim(coalesce(booking_reference, '')) <> ''
        AND trim(coalesce(guest_name, '')) <> ''
        AND payment_status = 'not_applicable'
        AND payment_note = ''
        AND finance_booking_payout_id IS NULL
        AND taxable_supply_date IS NULL
        AND due_date IS NULL
        AND amount_total_cents > 0
        AND date(stay_end_date) > date(stay_start_date)
        AND json_extract(supplier_snapshot_json, '$.vat_status') = 'non_vat_payer'
    ))
);
INSERT INTO invoices (
    id, property_id, named_stay_id, finance_booking_payout_id, document_type,
    invoice_number, sequence_year, sequence_value, language, issue_date,
    taxable_supply_date, due_date, stay_start_date, stay_end_date,
    supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency,
    payment_status, payment_note, version, created_by, created_at, updated_at
)
SELECT id, property_id, named_stay_id, finance_booking_payout_id, 'invoice',
       invoice_number, sequence_year, sequence_value, language, issue_date,
       taxable_supply_date, due_date, stay_start_date, stay_end_date,
       supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency,
       payment_status, payment_note, version, created_by, created_at, updated_at
FROM pms36_invoices_old;
CREATE INDEX idx_invoices_property_issue_date
    ON invoices (property_id, issue_date DESC, id DESC);
CREATE INDEX idx_invoices_named_stay ON invoices (named_stay_id);
CREATE UNIQUE INDEX ux_invoices_property_booking_payout
    ON invoices (property_id, finance_booking_payout_id)
    WHERE finance_booking_payout_id IS NOT NULL AND document_type = 'invoice';

CREATE TABLE invoice_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL REFERENCES invoices (id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version >= 1),
    file_path TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (invoice_id, version)
);
INSERT INTO invoice_files (id, invoice_id, version, file_path, file_size_bytes, created_at)
SELECT id, invoice_id, version, file_path, file_size_bytes, created_at
FROM pms36_invoice_files_old;
CREATE INDEX idx_invoice_files_invoice_version ON invoice_files (invoice_id, version DESC);

DROP TABLE pms36_invoice_files_old;
DROP TABLE pms36_invoices_old;

INSERT INTO sqlite_sequence (name, seq)
SELECT 'invoices', coalesce(max(id), 0) FROM invoices
WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'invoices');
UPDATE sqlite_sequence SET seq = (SELECT coalesce(max(id), 0) FROM invoices) WHERE name = 'invoices';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'invoice_files', coalesce(max(id), 0) FROM invoice_files
WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'invoice_files');
UPDATE sqlite_sequence SET seq = (SELECT coalesce(max(id), 0) FROM invoice_files) WHERE name = 'invoice_files';

UPDATE sqlite_sequence
SET seq = max(seq, coalesce((SELECT p.seq FROM pms36_sequences p WHERE p.name = sqlite_sequence.name), 0))
WHERE name IN ('invoice_sequences', 'invoices', 'invoice_files');
DROP TABLE pms36_sequences;
