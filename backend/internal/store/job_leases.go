package store

import (
	"context"
	"time"
)

// TryAcquireJobLease attempts to grab or extend a named scheduler lease for the
// given owner (typically a per-process instance ID). Returns true when the
// caller now holds the lease until `now + ttl`. Returns false when another
// owner holds an unexpired lease.
//
// The operation is atomic: concurrent callers race on the UPSERT's WHERE clause
// so at most one of them observes a successful acquisition.
func (s *Store) TryAcquireJobLease(ctx context.Context, jobName, owner string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	nowS := now.Format(time.RFC3339)
	expS := expires.Format(time.RFC3339)
	// Upsert semantics: insert when the row is missing; on conflict only update
	// when the current owner still matches (renewal) OR the existing lease has
	// expired. SQLite compares TEXT timestamps lexicographically, which is
	// correct for RFC3339 UTC values.
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO job_leases (job_name, owner, acquired_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(job_name) DO UPDATE SET
			owner = excluded.owner,
			acquired_at = excluded.acquired_at,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at
		WHERE job_leases.owner = excluded.owner
		   OR job_leases.expires_at < excluded.acquired_at`,
		jobName, owner, nowS, expS, nowS)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// Another instance holds the lease.
	return false, nil
}

// ReleaseJobLease releases a lease previously acquired by the given owner. A
// mismatched owner is treated as a no-op so a stale caller cannot evict an
// unrelated holder.
func (s *Store) ReleaseJobLease(ctx context.Context, jobName, owner string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM job_leases
		WHERE job_name = ? AND owner = ?`, jobName, owner)
	return err
}
