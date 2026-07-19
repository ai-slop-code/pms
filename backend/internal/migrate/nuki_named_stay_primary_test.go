package migrate

import (
	"database/sql"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"pms/backend/internal/dbconn"
)

func applyMigrationsThrough(t *testing.T, db *sql.DB, maxName string) {
	t.Helper()
	entries, err := fs.ReadDir(embeddedMigrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") && entry.Name() <= maxName {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := embeddedMigrations.ReadFile(path.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

func TestNukiNamedStayPrimaryMigrationPreservesRowsAndReferences(t *testing.T) {
	db, err := dbconn.Open("sqlite://" + t.TempDir() + "/migration.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	applyMigrationsThrough(t, db, "000035_finance_invoice_named_stay_cutover.up.sql")
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'migration@test.local', 'hash', 'owner', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Migration', 'UTC', 1, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO occupancies (id, property_id, source_type, source_event_uid, start_at, end_at, status, content_hash, imported_at, last_synced_at) VALUES (10, 1, 'manual', 'legacy-10', ?, '2026-01-03T00:00:00Z', 'active', 'hash', ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, created_at, updated_at) VALUES (20, 1, 'Guest', 'booking_com', '2026-01-01', '2026-01-03', 'active', 1, 'confirmed', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nuki_access_codes (id, property_id, occupancy_id, named_stay_id, code_label, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, created_at, updated_at) VALUES (30, 1, 10, 20, 'Booking-Guest', 'encrypted-pin', 'external-30', ?, '2026-01-03T10:00:00Z', 'generated', ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nuki_event_logs (id, property_id, nuki_access_code_id, event_type, message, created_at) VALUES (40, 1, 30, 'generated', 'kept', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nuki_guest_daily_entries (id, property_id, occupancy_id, named_stay_id, day_date, first_entry_at, nuki_event_reference, created_at) VALUES (50, 1, 10, 20, '2026-01-01', ?, 'event-50', ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	body, err := embeddedMigrations.ReadFile("000036_nuki_named_stay_primary.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	var codeID, occupancyID, stayID int64
	var pin, externalID string
	if err := db.QueryRow(`SELECT id, occupancy_id, named_stay_id, generated_pin_plain, external_nuki_id FROM nuki_access_codes WHERE id = 30`).Scan(&codeID, &occupancyID, &stayID, &pin, &externalID); err != nil {
		t.Fatal(err)
	}
	if codeID != 30 || occupancyID != 10 || stayID != 20 || pin != "encrypted-pin" || externalID != "external-30" {
		t.Fatalf("access code changed: id=%d occupancy=%d stay=%d pin=%q external=%q", codeID, occupancyID, stayID, pin, externalID)
	}
	var eventCodeID int64
	if err := db.QueryRow(`SELECT nuki_access_code_id FROM nuki_event_logs WHERE id = 40`).Scan(&eventCodeID); err != nil || eventCodeID != 30 {
		t.Fatalf("event reference=%d err=%v", eventCodeID, err)
	}
	var entryID int64
	if err := db.QueryRow(`SELECT id FROM nuki_guest_daily_entries WHERE id = 50 AND occupancy_id = 10 AND named_stay_id = 20 AND nuki_event_reference = 'event-50'`).Scan(&entryID); err != nil || entryID != 50 {
		t.Fatalf("guest entry id=%d err=%v", entryID, err)
	}
	if _, err := db.Exec(`INSERT INTO nuki_access_codes (property_id, named_stay_id, code_label, valid_from, valid_until, status, created_at, updated_at) VALUES (1, 20, 'duplicate', ?, ?, 'not_generated', ?, ?)`, now, now, now, now); err == nil {
		t.Fatal("named-stay uniqueness was not enforced")
	}
	if _, err := db.Exec(`INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, created_at, updated_at) VALUES (21, 1, 'Named Only', 'booking_com', '2026-02-01', '2026-02-02', 'active', 1, 'confirmed', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nuki_access_codes (property_id, named_stay_id, code_label, valid_from, valid_until, status, created_at, updated_at) VALUES (1, 21, 'Named Only', ?, ?, 'not_generated', ?, ?)`, now, now, now, now); err != nil {
		t.Fatalf("named-only access code: %v", err)
	}
}
