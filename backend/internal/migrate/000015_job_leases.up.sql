-- Cooperative lease table used by in-process schedulers so only one instance
-- runs each periodic job at a time. Each row represents a named job with the
-- current owner (process instance ID) and an expiration stamp.
CREATE TABLE job_leases (
    job_name TEXT PRIMARY KEY,
    owner TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_job_leases_expires_at ON job_leases (expires_at);
