CREATE TABLE finance_booking_payouts (
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
    occupancy_id INTEGER REFERENCES occupancies (id) ON DELETE SET NULL,
    raw_row_json TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, reference_number)
);

CREATE INDEX idx_finance_booking_payouts_property_payout_date
ON finance_booking_payouts (property_id, payout_date DESC);

CREATE INDEX idx_finance_booking_payouts_property_occupancy
ON finance_booking_payouts (property_id, occupancy_id);
