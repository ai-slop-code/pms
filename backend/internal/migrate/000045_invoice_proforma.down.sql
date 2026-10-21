-- Refuse a downgrade that would lose issued pro formas or their metadata.
CREATE TEMP TABLE pms36_down_guard (id INTEGER);
CREATE TEMP TRIGGER pms36_down_guard_trigger
BEFORE INSERT ON pms36_down_guard
WHEN EXISTS (SELECT 1 FROM invoices WHERE document_type = 'proforma')
  OR EXISTS (SELECT 1 FROM invoice_sequences WHERE document_type = 'proforma')
  OR EXISTS (SELECT 1 FROM invoices WHERE booking_reference IS NOT NULL OR guest_name IS NOT NULL OR price_details IS NOT NULL)
BEGIN
  SELECT RAISE(ABORT, 'cannot downgrade invoices while pro forma data exists');
END;
INSERT INTO pms36_down_guard VALUES (1);
DROP TABLE pms36_down_guard;

CREATE TEMP TABLE pms36_down_sequences AS
SELECT name, seq FROM sqlite_sequence
WHERE name IN ('invoice_sequences', 'invoices', 'invoice_files');

ALTER TABLE invoice_files RENAME TO pms36_invoice_files_new;
ALTER TABLE invoices RENAME TO pms36_invoices_new;
ALTER TABLE invoice_sequences RENAME TO pms36_invoice_sequences_new;
DROP INDEX IF EXISTS idx_invoices_property_issue_date;
DROP INDEX IF EXISTS idx_invoices_named_stay;
DROP INDEX IF EXISTS ux_invoices_property_booking_payout;
DROP INDEX IF EXISTS ux_invoices_property_named_stay;
DROP INDEX IF EXISTS idx_invoice_files_invoice_version;
DROP INDEX IF EXISTS idx_invoice_sequences_property_year;

CREATE TABLE invoice_sequences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    sequence_year INTEGER NOT NULL,
    current_value INTEGER NOT NULL DEFAULT 0 CHECK (current_value >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, sequence_year)
);
INSERT INTO invoice_sequences SELECT id, property_id, sequence_year, current_value, created_at, updated_at FROM pms36_invoice_sequences_new;
CREATE INDEX idx_invoice_sequences_property_year ON invoice_sequences (property_id, sequence_year DESC);

CREATE TABLE invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER NOT NULL,
    finance_booking_payout_id INTEGER,
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
    UNIQUE (property_id, invoice_number),
    UNIQUE (property_id, sequence_year, sequence_value),
    UNIQUE (property_id, named_stay_id),
    FOREIGN KEY (property_id, named_stay_id) REFERENCES named_stays (property_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (property_id, finance_booking_payout_id, named_stay_id) REFERENCES finance_bookings (property_id, id, named_stay_id) ON DELETE RESTRICT
);
INSERT INTO invoices (
    id, property_id, named_stay_id, finance_booking_payout_id, invoice_number,
    sequence_year, sequence_value, language, issue_date, taxable_supply_date,
    due_date, stay_start_date, stay_end_date, supplier_snapshot_json,
    customer_snapshot_json, amount_total_cents, currency, payment_status,
    payment_note, version, created_by, created_at, updated_at
)
SELECT id, property_id, named_stay_id, finance_booking_payout_id, invoice_number,
       sequence_year, sequence_value, language, issue_date, taxable_supply_date,
       due_date, stay_start_date, stay_end_date, supplier_snapshot_json,
       customer_snapshot_json, amount_total_cents, currency, payment_status,
       payment_note, version, created_by, created_at, updated_at
FROM pms36_invoices_new;
CREATE INDEX idx_invoices_property_issue_date ON invoices (property_id, issue_date DESC, id DESC);
CREATE INDEX idx_invoices_named_stay ON invoices (named_stay_id);
CREATE UNIQUE INDEX ux_invoices_property_booking_payout ON invoices (property_id, finance_booking_payout_id) WHERE finance_booking_payout_id IS NOT NULL;

CREATE TABLE invoice_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL REFERENCES invoices (id) ON DELETE RESTRICT,
    version INTEGER NOT NULL CHECK (version >= 1),
    file_path TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (invoice_id, version)
);
INSERT INTO invoice_files SELECT id, invoice_id, version, file_path, file_size_bytes, created_at FROM pms36_invoice_files_new;
CREATE INDEX idx_invoice_files_invoice_version ON invoice_files (invoice_id, version DESC);

DROP TABLE pms36_invoice_files_new;
DROP TABLE pms36_invoices_new;
DROP TABLE pms36_invoice_sequences_new;

UPDATE sqlite_sequence
SET seq = max(seq, coalesce((SELECT p.seq FROM pms36_down_sequences p WHERE p.name = sqlite_sequence.name), 0))
WHERE name IN ('invoice_sequences', 'invoices', 'invoice_files');
DROP TABLE pms36_down_sequences;
