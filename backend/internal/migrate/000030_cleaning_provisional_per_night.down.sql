CREATE UNIQUE INDEX IF NOT EXISTS uq_cleaning_calendar_occupancy
    ON cleaning_calendar_events (property_id, occupancy_id)
    WHERE occupancy_id IS NOT NULL;
