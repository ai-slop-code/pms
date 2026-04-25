CREATE TABLE message_templates_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    template_type TEXT NOT NULL DEFAULT 'check_in',
    title TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO message_templates_new (id, property_id, language_code, template_type, title, body, active, created_at, updated_at)
SELECT id, property_id, language_code, template_type, title, body, active, created_at, updated_at
FROM message_templates;

DROP TABLE message_templates;

ALTER TABLE message_templates_new RENAME TO message_templates;
