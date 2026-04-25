CREATE TABLE invoice_sequences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    sequence_year INTEGER NOT NULL,
    current_value INTEGER NOT NULL DEFAULT 0 CHECK (current_value >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, sequence_year)
);

CREATE INDEX idx_invoice_sequences_property_year
ON invoice_sequences (property_id, sequence_year DESC);

CREATE TABLE invoices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE SET NULL,
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
    UNIQUE (property_id, sequence_year, sequence_value)
);

CREATE UNIQUE INDEX ux_invoices_property_occupancy
ON invoices (property_id, occupancy_id)
WHERE occupancy_id IS NOT NULL;

CREATE INDEX idx_invoices_property_issue_date
ON invoices (property_id, issue_date DESC, id DESC);

CREATE TABLE invoice_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version >= 1),
    file_path TEXT NOT NULL,
    file_size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE (invoice_id, version)
);

CREATE INDEX idx_invoice_files_invoice_version
ON invoice_files (invoice_id, version DESC);
