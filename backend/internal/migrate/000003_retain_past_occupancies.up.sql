-- Keep historical occupancy for analytics: if previously marked deleted_from_source
-- but stay ended in the past, restore to active.
UPDATE occupancies
SET status = 'active'
WHERE status = 'deleted_from_source'
  AND end_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now');
