DROP INDEX IF EXISTS idx_nuki_managed_credentials_remote;
CREATE UNIQUE INDEX idx_nuki_managed_credentials_remote
    ON nuki_managed_credentials (property_id, smartlock_id, remote_id)
    WHERE remote_id IS NOT NULL AND operation_state <> 'deleted';
