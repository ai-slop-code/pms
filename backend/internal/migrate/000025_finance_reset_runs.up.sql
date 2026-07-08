CREATE TABLE finance_reset_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    deleted_counts_json TEXT NOT NULL,
    preserved_counts_json TEXT NOT NULL,
    regenerated_counts_json TEXT NOT NULL,
    attachment_delete_errors_json TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_finance_reset_runs_property_completed
ON finance_reset_runs (property_id, completed_at DESC);
