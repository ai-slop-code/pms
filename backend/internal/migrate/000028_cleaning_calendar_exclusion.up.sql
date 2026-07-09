ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded INTEGER NOT NULL DEFAULT 0;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_exclusion_reason TEXT;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE occupancies ADD COLUMN cleaning_calendar_excluded_at TEXT;

CREATE INDEX idx_occupancies_property_cleaning_calendar_excluded
    ON occupancies (property_id, cleaning_calendar_excluded);
