DROP INDEX IF EXISTS ux_invoices_property_booking_payout;

-- SQLite cannot DROP COLUMN easily; recreate would be heavy. For dev rollback, keep columns or run manual migration.
-- No-op for down in v1 tooling; fresh DBs use full migrate from scratch.
