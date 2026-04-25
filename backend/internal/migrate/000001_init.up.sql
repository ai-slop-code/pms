-- Identity and access (PostgreSQL-friendly types: INTEGER id, TEXT timestamps)
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('super_admin', 'owner', 'property_manager', 'read_only')),
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE auth_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_auth_sessions_token_hash ON auth_sessions (token_hash);
CREATE INDEX idx_auth_sessions_user_id ON auth_sessions (user_id);

CREATE TABLE properties (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'Europe/Bratislava',
    default_language TEXT NOT NULL DEFAULT 'sk',
    default_currency TEXT NOT NULL DEFAULT 'EUR',
    owner_user_id INTEGER NOT NULL REFERENCES users (id),
    address_line1 TEXT,
    city TEXT,
    postal_code TEXT,
    country TEXT,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_properties_owner_user_id ON properties (owner_user_id);

CREATE TABLE property_profiles (
    property_id INTEGER PRIMARY KEY REFERENCES properties (id) ON DELETE CASCADE,
    legal_owner_name TEXT,
    billing_name TEXT,
    billing_address TEXT,
    city TEXT,
    postal_code TEXT,
    country TEXT,
    ico TEXT,
    dic TEXT,
    vat_id TEXT,
    contact_phone TEXT,
    wifi_ssid TEXT,
    wifi_password TEXT,
    parking_instructions TEXT,
    default_check_in_time TEXT NOT NULL DEFAULT '14:00',
    default_check_out_time TEXT NOT NULL DEFAULT '10:00',
    cleaner_nuki_auth_id TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE property_secrets (
    property_id INTEGER PRIMARY KEY REFERENCES properties (id) ON DELETE CASCADE,
    booking_ics_url TEXT,
    nuki_api_token TEXT,
    nuki_smartlock_id TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE property_user_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    property_id INTEGER NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
    module TEXT NOT NULL,
    permission_level TEXT NOT NULL CHECK (permission_level IN ('read', 'write', 'admin')),
    created_at TEXT NOT NULL,
    UNIQUE (user_id, property_id, module)
);

CREATE INDEX idx_property_user_permissions_user ON property_user_permissions (user_id);
CREATE INDEX idx_property_user_permissions_property ON property_user_permissions (property_id);

CREATE TABLE api_audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT,
    outcome TEXT NOT NULL,
    request_method TEXT,
    request_path TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_api_audit_logs_actor ON api_audit_logs (actor_user_id);
CREATE INDEX idx_api_audit_logs_created ON api_audit_logs (created_at);
