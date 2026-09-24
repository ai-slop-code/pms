package migrate

import (
	"context"
	"strings"
	"testing"

	"pms/backend/internal/cleaningcalendarcleanup"
	"pms/backend/internal/store"
)

func TestUpCleaningCalendarRequiresPMS21AndOnlyAppliesPMS22(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	recordMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	if err := UpCleaningCalendar(db); err == nil || !strings.Contains(err.Error(), "000039") {
		t.Fatalf("expected PMS-21 prerequisite error, got %v", err)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 0)
	if err := applyCleanupInTransaction(t, db); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO schema_migrations VALUES ('000039_legacy_occupancy_removal')`)
	if err := UpCleaningCalendar(db); err != nil {
		t.Fatal(err)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000040_named_stay_only_cleaning_calendar'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version > '000040_named_stay_only_cleaning_calendar'`, 0)
	if err := UpCleaningCalendar(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	assertDatabaseChecks(t, db)
}

func TestUpCleaningCalendarRepairsMissingColumnsAfterCleanup(t *testing.T) {
	db := openMigrationTestDB(t)
	db.SetMaxOpenConns(8) // The cleanup runner reads properties while writing state.
	applyMigrationsThrough(t, db, cleanupMigration)
	recordMigrationsThrough(t, db, cleanupMigration)
	if err := UpAutomatic(db); err != nil { // Production shape: later ordinary migrations installed, 40 absent.
		t.Fatal(err)
	}
	const now = "2026-09-24T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
		VALUES (1, 'calendar@test.local', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at)
		VALUES (1, 'Calendar', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO named_stays
		(id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, first_known_at, created_at, updated_at)
		VALUES (1, 1, 'Guest', 'external', '2026-09-24', '2026-09-25', 'active', 1, 'confirmed', ?, ?, ?)`, now, now, now)
	mustExec(t, db, `INSERT INTO property_google_cleaning_settings (property_id, calendar_id, updated_at)
		VALUES (1, 'calendar', ?)`, now)
	mustExec(t, db, `INSERT INTO cleaning_calendar_events
		(id, property_id, named_stay_id, cleaning_kind, cleaning_identity, google_calendar_id, google_event_id,
		 cleaning_date, starts_at, ends_at, title, status, created_at, updated_at)
		VALUES (1, 1, 1, 'named_stay', 'stay:1:1:2026-09-25', 'calendar', 'google-event',
		 '2026-09-25', '2026-09-25T09:00:00Z', '2026-09-25T12:00:00Z', 'Cleaning', 'synced', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO cleaning_calendar_event_logs
		(property_id, cleaning_calendar_event_id, action, created_at) VALUES (1, 1, 'upsert', ?)`, now)
	mustExec(t, db, `INSERT INTO raw_booking_blocks
		(id, property_id, source_event_uid, check_in_date, check_out_date, status, content_hash, imported_at, last_synced_at, created_at, updated_at)
		VALUES (1, 1, 'raw', '2026-09-24', '2026-09-25', 'active', 'hash', ?, ?, ?, ?)`, now, now, now, now)
	mustExec(t, db, `INSERT INTO cleaning_calendar_events
		(id, property_id, raw_booking_block_id, cleaning_kind, google_calendar_id, google_event_id,
		 cleaning_date, starts_at, ends_at, title, status, created_at, updated_at)
		VALUES (2, 1, 1, 'provisional_block', 'calendar', 'raw-google-event',
		 '2026-09-25', '2026-09-25T09:00:00Z', '2026-09-25T12:00:00Z', 'Cleaning', 'synced', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO cleaning_calendar_event_logs
		(property_id, cleaning_calendar_event_id, action, created_at) VALUES (1, 2, 'upsert', ?)`, now)
	st := &store.Store{DB: db}
	if _, err := st.ListCleaningCalendarEventsForMonth(context.Background(), 1, "2026-09"); err == nil || !strings.Contains(err.Error(), "pending_action") {
		t.Fatalf("expected incident missing-column error, got %v", err)
	}
	assertBlocked := func() {
		t.Helper()
		if err := UpCleaningCalendar(db); err == nil || !strings.Contains(err.Error(), "cleanup is incomplete") {
			t.Fatalf("expected cleanup prerequisite error, got %v", err)
		}
		assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000040_named_stay_only_cleaning_calendar'`, 0)
		assertIntQuery(t, db, `SELECT count(*) FROM cleaning_calendar_events`, 2)
	}
	assertBlocked()
	runner := &cleaningcalendarcleanup.Runner{DB: db}
	if err := runner.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertBlocked()
	mustExec(t, db, `UPDATE cleaning_calendar_cleanup_state SET phase = 'complete'`)
	mustExec(t, db, `INSERT INTO cleaning_calendar_cleanup_items
		(property_id, calendar_id, google_event_id, created_at, updated_at) VALUES (1, 'calendar', 'pending-delete', ?, ?)`, now, now)
	assertBlocked()
	mustExec(t, db, `DELETE FROM cleaning_calendar_cleanup_items`)
	mustExec(t, db, `UPDATE property_google_cleaning_settings SET calendar_id = 'changed'`)
	assertBlocked()
	mustExec(t, db, `UPDATE property_google_cleaning_settings SET calendar_id = 'calendar'`)
	if err := UpCleaningCalendar(db); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListCleaningCalendarEventsForMonth(context.Background(), 1, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != 1 || events[0].GoogleEventID.String != "google-event" || events[0].PendingAction != "none" {
		t.Fatalf("named event not preserved: %+v", events)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM cleaning_calendar_event_logs WHERE cleaning_calendar_event_id = 1`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM cleaning_calendar_event_logs WHERE cleaning_calendar_event_id = 2`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM raw_booking_blocks`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE name LIKE 'cleaning_calendar_cleanup_%'`, 0)
	if err := UpCleaningCalendar(db); err != nil {
		t.Fatalf("repeat migration after cleanup tables removed: %v", err)
	}
	assertDatabaseChecks(t, db)
}
