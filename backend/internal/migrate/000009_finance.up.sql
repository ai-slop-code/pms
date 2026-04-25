CREATE TABLE finance_categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER REFERENCES properties (id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    title TEXT NOT NULL,
    direction TEXT NOT NULL DEFAULT 'both' CHECK (direction IN ('incoming', 'outgoing', 'both')),
    counts_toward_property_income INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (property_id, code)
);

CREATE INDEX idx_finance_categories_property
ON finance_categories (property_id, active, title);

CREATE TABLE finance_transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    transaction_date TEXT NOT NULL,
    direction TEXT NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
    amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
    category_id INTEGER REFERENCES finance_categories (id) ON DELETE SET NULL,
    note TEXT,
    source_type TEXT NOT NULL DEFAULT 'manual',
    source_reference_id TEXT,
    is_auto_generated INTEGER NOT NULL DEFAULT 0,
    attachment_path TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_finance_transactions_property_date
ON finance_transactions (property_id, transaction_date DESC, id DESC);

CREATE INDEX idx_finance_transactions_property_source
ON finance_transactions (property_id, source_type, source_reference_id);

CREATE UNIQUE INDEX ux_finance_transactions_autogen_ref
ON finance_transactions (property_id, source_type, source_reference_id, transaction_date)
WHERE source_type IN ('recurring_rule', 'cleaning_salary') AND source_reference_id IS NOT NULL;

CREATE TABLE finance_recurring_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    category_id INTEGER REFERENCES finance_categories (id) ON DELETE SET NULL,
    amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
    direction TEXT NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
    frequency TEXT NOT NULL DEFAULT 'monthly' CHECK (frequency = 'monthly'),
    start_month TEXT NOT NULL,
    end_month TEXT,
    effective_from TEXT NOT NULL,
    effective_to TEXT,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_finance_recurring_rules_property
ON finance_recurring_rules (property_id, active, start_month, end_month);

CREATE TABLE finance_month_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    month TEXT NOT NULL,
    opened_at TEXT NOT NULL,
    opened_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (property_id, month)
);

CREATE INDEX idx_finance_month_states_property
ON finance_month_states (property_id, month DESC);

INSERT INTO finance_categories (property_id, code, title, direction, counts_toward_property_income, active, created_at, updated_at)
VALUES
    (NULL, 'booking_income', 'Booking Income', 'incoming', 1, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'utility_refund', 'Utility Refund', 'incoming', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'mortgage', 'Mortgage', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'utilities', 'Utilities', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'internet', 'Internet', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'cleaning_salary', 'Cleaning Salary', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'maintenance', 'Maintenance', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'tax', 'Tax', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'supplies', 'Supplies', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'other_income', 'Other Income', 'incoming', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    (NULL, 'other_expense', 'Other Expense', 'outgoing', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));
