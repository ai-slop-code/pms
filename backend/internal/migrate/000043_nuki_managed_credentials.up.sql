CREATE TABLE nuki_managed_credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    named_stay_id INTEGER REFERENCES named_stays (id) ON DELETE SET NULL,
    nuki_access_code_id INTEGER REFERENCES nuki_access_codes (id) ON DELETE SET NULL,
    smartlock_id TEXT NOT NULL,
    remote_id TEXT,
    provenance TEXT NOT NULL CHECK (provenance IN ('direct_provider_identity', 'correlated_creation_intent', 'validated_legacy_link', 'evidence_backed_incident_recovery')),
    provenance_reference TEXT,
    desired_label TEXT NOT NULL,
    desired_valid_from TEXT NOT NULL,
    desired_valid_until TEXT NOT NULL,
    desired_enabled INTEGER NOT NULL DEFAULT 1,
    operation_state TEXT NOT NULL CHECK (operation_state IN ('create_pending', 'active', 'update_pending', 'delete_pending', 'deleted', 'needs_review')),
    operation_revision INTEGER NOT NULL DEFAULT 1,
    observed_json TEXT,
    observed_at TEXT,
    latest_error TEXT,
    last_attempt_at TEXT,
    next_retry_at TEXT,
    pending_pin TEXT,
    request_accepted_at TEXT,
    completion_observed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_nuki_managed_credentials_remote
    ON nuki_managed_credentials (property_id, smartlock_id, remote_id)
    WHERE remote_id IS NOT NULL;
CREATE UNIQUE INDEX idx_nuki_managed_credentials_pending_stay
    ON nuki_managed_credentials (property_id, named_stay_id)
    WHERE named_stay_id IS NOT NULL AND operation_state NOT IN ('deleted', 'needs_review');
CREATE INDEX idx_nuki_managed_credentials_operations
    ON nuki_managed_credentials (property_id, operation_state, next_retry_at);
