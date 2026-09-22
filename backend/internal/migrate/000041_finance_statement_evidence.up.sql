ALTER TABLE finance_imports ADD COLUMN row_count_skipped_cancellations INTEGER NOT NULL DEFAULT 0;

CREATE TABLE finance_statement_evidence (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    source_channel TEXT NOT NULL DEFAULT 'booking_com',
    reference_number TEXT NOT NULL,
    hotel_id TEXT,
    status TEXT NOT NULL,
    booked_on TEXT NOT NULL,
    check_in_date TEXT NOT NULL,
    check_out_date TEXT NOT NULL,
    raw_statement_row_json TEXT NOT NULL,
    last_import_id INTEGER NOT NULL REFERENCES finance_imports (id) ON DELETE CASCADE,
    source_line INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, source_channel, reference_number)
);
CREATE INDEX idx_finance_statement_evidence_property_booked
    ON finance_statement_evidence (property_id, booked_on);
CREATE INDEX idx_finance_statement_evidence_property_arrival
    ON finance_statement_evidence (property_id, check_in_date);
