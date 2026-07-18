ALTER TABLE occupancy_sync_runs ADD COLUMN raw_blocks_inserted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN raw_blocks_updated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN raw_blocks_unchanged INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN raw_blocks_deleted_from_source INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancy_sync_runs ADD COLUMN raw_block_conflicts INTEGER NOT NULL DEFAULT 0;
