ALTER TABLE invoices ADD COLUMN line_items_json TEXT
    CHECK (line_items_json IS NULL OR
        CASE WHEN json_valid(line_items_json)
        THEN json_type(line_items_json) = 'array' AND json_array_length(line_items_json) BETWEEN 1 AND 100
        ELSE 0 END);
