CREATE TABLE finance_long_term_rent_rates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    effective_from_month TEXT NOT NULL,
    monthly_rent_cents INTEGER NOT NULL CHECK (monthly_rent_cents > 0),
    currency TEXT NOT NULL DEFAULT 'EUR' CHECK (currency = 'EUR'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, effective_from_month)
);

CREATE INDEX idx_finance_long_term_rent_rates_property_month
    ON finance_long_term_rent_rates (property_id, effective_from_month);
