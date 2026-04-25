CREATE TABLE cleaner_fee_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    cleaning_fee_amount_cents INTEGER NOT NULL DEFAULT 0,
    washing_fee_amount_cents INTEGER NOT NULL DEFAULT 0,
    effective_from TEXT NOT NULL,
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_cleaner_fee_history_property_effective
ON cleaner_fee_history (property_id, effective_from DESC);

CREATE TABLE cleaning_daily_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    day_date TEXT NOT NULL,
    first_entry_at TEXT,
    nuki_event_reference TEXT,
    counted_for_salary INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    UNIQUE (property_id, day_date)
);

CREATE INDEX idx_cleaning_daily_logs_property_day
ON cleaning_daily_logs (property_id, day_date);

CREATE TABLE cleaning_monthly_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    counted_days INTEGER NOT NULL DEFAULT 0,
    base_salary_cents INTEGER NOT NULL DEFAULT 0,
    adjustments_total_cents INTEGER NOT NULL DEFAULT 0,
    final_salary_cents INTEGER NOT NULL DEFAULT 0,
    computed_at TEXT NOT NULL,
    UNIQUE (property_id, year, month)
);

CREATE INDEX idx_cleaning_monthly_summaries_property_month
ON cleaning_monthly_summaries (property_id, year, month);

CREATE TABLE cleaning_salary_adjustments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    adjustment_amount_cents INTEGER NOT NULL,
    reason TEXT NOT NULL,
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_cleaning_salary_adjustments_property_month
ON cleaning_salary_adjustments (property_id, year, month);
