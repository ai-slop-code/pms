package migrate

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"

	"pms/backend/internal/dbconn"
)

const cleanupMigration = "000039_legacy_occupancy_removal.up.sql"

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := dbconn.Open("sqlite://" + t.TempDir() + "/migration.db")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func applyCleanupInTransaction(t *testing.T, db *sql.DB) error {
	t.Helper()
	body, err := embeddedMigrations.ReadFile(cleanupMigration)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(string(body)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func recordMigrationsThrough(t *testing.T, db *sql.DB, maxName string) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
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
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, strings.TrimSuffix(name, ".up.sql")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyOccupancyRemovalFreshReplay(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := Up(db); err != nil {
		t.Fatal(err)
	}

	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("cleanup migration applied=%d err=%v", applied, err)
	}
	assertCleanLatestSchema(t, db)
	assertDatabaseChecks(t, db)
}

func TestStartupAppliesCleanupForFreshDatabase(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := UpStartup(db); err != nil {
		t.Fatal(err)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 1)
	assertCleanLatestSchema(t, db)
}

func TestStartupLeavesCleanupPendingForExistingDatabase(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000037_finance_evidence_confirms_named_stays.up.sql")
	recordMigrationsThrough(t, db, "000037_finance_evidence_confirms_named_stays.up.sql")
	if err := UpStartup(db); err != nil {
		t.Fatal(err)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000038_named_stay_lifecycle_metadata'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`, 1)
}

func TestAutomaticUpLeavesCleanupPendingForExplicitUp(t *testing.T) {
	db := openMigrationTestDB(t)
	if err := UpAutomatic(db); err != nil {
		t.Fatal(err)
	}

	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000038_named_stay_lifecycle_metadata'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`, 1)

	if err := Up(db); err != nil {
		t.Fatal(err)
	}
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 1)
	assertCleanLatestSchema(t, db)
}

func TestAutomaticMigrationRegistryDoesNotMatchFutureOrdinaryVersions(t *testing.T) {
	if _, manual := manualMigrations["000039_legacy_occupancy_removal"]; !manual {
		t.Fatal("cleanup migration is not registered as manual")
	}
	if _, manual := manualMigrations["000040_future_ordinary_migration"]; manual {
		t.Fatal("future ordinary migration was classified as manual")
	}
}

func TestLegacyOccupancyRemovalPreservesRowsValuesReferencesAndSequences(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	seedCleanupFixture(t, db)

	if err := applyCleanupInTransaction(t, db); err != nil {
		t.Fatal(err)
	}

	var codeID, stayID int64
	var pin, masked, externalID, status, revoked string
	if err := db.QueryRow(`
		SELECT id, named_stay_id, generated_pin_plain, access_code_masked,
		       external_nuki_id, status, revoked_at
		FROM nuki_access_codes WHERE id = 60`).Scan(
		&codeID, &stayID, &pin, &masked, &externalID, &status, &revoked); err != nil {
		t.Fatal(err)
	}
	if codeID != 60 || stayID != 20 || pin != "stored-pin-ciphertext" || masked != "12**" ||
		externalID != "nuki-external-60" || status != "revoked" || revoked != "2026-01-05T00:00:00Z" {
		t.Fatalf("Nuki critical values changed: %d %d %q %q %q %q %q", codeID, stayID, pin, masked, externalID, status, revoked)
	}
	assertIntQuery(t, db, `SELECT nuki_access_code_id FROM nuki_event_logs WHERE id = 61`, 60)
	assertIntQuery(t, db, `SELECT named_stay_id FROM nuki_guest_daily_entries WHERE id = 62`, 20)

	assertIntQuery(t, db, `SELECT cleaning_calendar_event_id FROM cleaning_calendar_event_logs WHERE id = 81`, 80)
	assertIntQuery(t, db, `SELECT cleaning_calendar_event_id FROM cleaning_calendar_event_logs WHERE id = 83`, 82)
	var googleID, identity, desiredHash, warning, eventStatus string
	if err := db.QueryRow(`
		SELECT google_event_id, cleaning_identity, desired_hash, warning_message, status
		FROM cleaning_calendar_events WHERE id = 80`).Scan(
		&googleID, &identity, &desiredHash, &warning, &eventStatus); err != nil {
		t.Fatal(err)
	}
	if googleID != "google-event-80" || identity != "cleaning-80" || desiredHash != "desired-80" || warning != "warning-80" || eventStatus != "synced" {
		t.Fatalf("cleaning values changed: %q %q %q %q %q", googleID, identity, desiredHash, warning, eventStatus)
	}

	assertIntQuery(t, db, `SELECT booking_id FROM finance_booking_merges WHERE id = 101`, 100)
	assertIntQuery(t, db, `SELECT import_id FROM finance_booking_merges WHERE id = 101`, 91)
	assertIntQuery(t, db, `SELECT transaction_id FROM finance_bookings WHERE id = 100`, 90)
	assertIntQuery(t, db, `SELECT finance_booking_payout_id FROM invoices WHERE id = 110`, 100)
	assertIntQuery(t, db, `SELECT invoice_id FROM invoice_files WHERE id = 111`, 110)
	assertIntQuery(t, db, `SELECT named_stay_id FROM named_stay_nights WHERE id = 201`, 20)
	assertIntQuery(t, db, `SELECT raw_booking_block_id FROM raw_booking_block_nights WHERE id = 301`, 30)
	assertIntQuery(t, db, `SELECT raw_booking_block_id FROM stay_source_links WHERE id = 32`, 30)
	assertIntQuery(t, db, `SELECT property_id FROM property_availability_blocks WHERE id = 40`, 1)
	var reference, rawPayout, rawStatement string
	var net int64
	if err := db.QueryRow(`
		SELECT reference_number, net_cents, raw_payout_row_json, raw_statement_row_json
		FROM finance_bookings WHERE id = 100`).Scan(&reference, &net, &rawPayout, &rawStatement); err != nil {
		t.Fatal(err)
	}
	if reference != "BOOK-100" || net != 12345 || rawPayout != `{"payout":true}` || rawStatement != `{"statement":true}` {
		t.Fatalf("finance values changed: %q %d %q %q", reference, net, rawPayout, rawStatement)
	}
	var invoiceNumber, supplier, customer, filePath string
	var fileSize int64
	if err := db.QueryRow(`
		SELECT i.invoice_number, i.supplier_snapshot_json, i.customer_snapshot_json,
		       f.file_path, f.file_size_bytes
		FROM invoices i JOIN invoice_files f ON f.invoice_id = i.id
		WHERE i.id = 110`).Scan(&invoiceNumber, &supplier, &customer, &filePath, &fileSize); err != nil {
		t.Fatal(err)
	}
	if invoiceNumber != "INV-2026-001" || supplier != `{"supplier":"snapshot"}` ||
		customer != `{"customer":"snapshot"}` || filePath != "/invoices/110-v1.pdf" || fileSize != 4321 {
		t.Fatalf("invoice/file values changed: %q %q %q %q %d", invoiceNumber, supplier, customer, filePath, fileSize)
	}

	for _, table := range []string{
		"nuki_access_codes", "nuki_event_logs", "nuki_guest_daily_entries",
		"cleaning_calendar_events", "cleaning_calendar_event_logs", "finance_bookings",
		"finance_booking_merges", "invoices", "invoice_files", "property_availability_blocks",
		"named_stay_nights", "raw_booking_block_nights", "stay_source_links",
	} {
		var seq int64
		if err := db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = ?`, table).Scan(&seq); err != nil {
			t.Fatalf("sequence %s: %v", table, err)
		}
		if seq != 500 {
			t.Fatalf("sequence %s=%d want 500", table, seq)
		}
	}
	res, err := db.Exec(`
		INSERT INTO nuki_guest_daily_entries
		(property_id, named_stay_id, day_date, first_entry_at, created_at)
		VALUES (1, 20, '2026-01-02', '2026-01-02T12:00:00Z', '2026-01-02T12:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := res.LastInsertId(); err != nil || id != 501 {
		t.Fatalf("next guest-entry id=%d err=%v", id, err)
	}

	assertCleanLatestSchema(t, db)
	assertDatabaseChecks(t, db)
	assertFinalConstraints(t, db)
	assertExpectedIndexes(t, db)
}

func TestLegacyOccupancyRemovalRollsBackInjectedMidRebuildFailure(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	seedCleanupFixture(t, db)
	want := snapshotMigrationDatabase(t, db)

	body, err := embeddedMigrations.ReadFile(cleanupMigration)
	if err != nil {
		t.Fatal(err)
	}
	const injectionPoint = "ALTER TABLE nuki_access_codes_v3 RENAME TO nuki_access_codes;"
	if strings.Count(string(body), injectionPoint) != 1 {
		t.Fatalf("cleanup migration injection point count changed")
	}
	injected := strings.Replace(string(body), injectionPoint, injectionPoint+`
INSERT INTO pms21_deliberate_failure (value) VALUES (1);`, 1)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, execErr := tx.Exec(injected)
	if execErr == nil {
		_ = tx.Rollback()
		t.Fatal("injected migration unexpectedly succeeded")
	}
	if !strings.Contains(execErr.Error(), "pms21_deliberate_failure") {
		_ = tx.Rollback()
		t.Fatalf("migration failed somewhere other than the injected statement: %v", execErr)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	assertMigrationSnapshotsEqual(t, want, snapshotMigrationDatabase(t, db))
	assertIntQuery(t, db, `
		SELECT count(*) FROM sqlite_schema
		WHERE name GLOB 'pms21_*' OR name GLOB '*_v2' OR name GLOB '*_v3'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_temp_schema`, 0)
	assertDatabaseChecks(t, db)
}

func TestHistoricalUpgradeSchemaConvergence(t *testing.T) {
	fresh := openMigrationTestDB(t)
	if err := Up(fresh); err != nil {
		t.Fatal(err)
	}
	want := normalizedMigrationSchemaShape(t, fresh)

	for _, cutPoint := range []string{
		"000001_init.up.sql",
		"000020_nuki_guest_daily_entries.up.sql",
		"000031_ics_dtstamp.up.sql",
		"000035_finance_invoice_named_stay_cutover.up.sql",
		"000038_named_stay_lifecycle_metadata.up.sql",
	} {
		t.Run(strings.TrimSuffix(cutPoint, ".up.sql"), func(t *testing.T) {
			db := openMigrationTestDB(t)
			applyMigrationsThrough(t, db, cutPoint)
			recordMigrationsThrough(t, db, cutPoint)
			if err := Up(db); err != nil {
				t.Fatal(err)
			}

			assertMigrationSnapshotsEqual(t, want, normalizedMigrationSchemaShape(t, db))
			assertCleanLatestSchema(t, db)
			assertDatabaseChecks(t, db)
		})
	}
}

func TestLegacyOccupancyRemovalRejectsInvalidOwnersAtomically(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	now := "2026-01-01T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
		VALUES (1, 'invalid-cleanup@test.local', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at)
		VALUES (1, 'Invalid', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, review_status, first_known_at, created_at, updated_at
		) VALUES (20, 1, 'Unresolved', 'external', '2026-01-01', '2026-01-02',
			'archived', 0, 'needs_review', ?, ?, ?)`, now, now, now)

	recordMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	if err := Up(db); err == nil {
		t.Fatal("cleanup accepted an unresolved needs_review stay")
	}

	assertIntQuery(t, db, `SELECT count(*) FROM named_stays WHERE id = 20`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM pragma_table_info('nuki_access_codes') WHERE name = 'occupancy_id'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE name LIKE '%_v2' OR name LIKE '%_v3'`, 0)
	assertDatabaseChecks(t, db)
}

func TestLegacyOccupancyRemovalRejectsMissingLifecycleEvidenceAtomically(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	now := "2026-01-01T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
		VALUES (1, 'lifecycle-cleanup@test.local', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at)
		VALUES (1, 'Lifecycle', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, source_channel, review_status, review_resolution,
			created_at, updated_at
		) VALUES (20, 1, 'Unsupported migrated stay', 'booking_com', '2026-01-01',
			'2026-01-02', 'archived', 0, 'booking_com', 'confirmed', 'confirmed', ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, source_channel, review_status, review_resolution,
			first_known_at, created_at, updated_at
		) VALUES (21, 1, 'Unsupported migrated cancellation', 'booking_com', '2026-02-01',
			'2026-02-02', 'cancelled', 0, 'booking_com', 'confirmed', 'confirmed', ?, ?, ?)`, now, now, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, created_at, updated_at
		) VALUES (1, 'NO-LIFECYCLE-EVIDENCE', '2026-01-01', '2026-01-02',
			'Unsupported migrated stay', 0, '2026-01-03', 20, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, booked_on, created_at, updated_at
		) VALUES (1, 'NO-CANCELLATION-EVIDENCE', '2026-02-01', '2026-02-02',
			'Unsupported migrated cancellation', 0, '2026-02-03', 21, ?, ?, ?)`, now, now, now)

	recordMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	if err := Up(db); err == nil {
		t.Fatal("cleanup accepted a migrated stay without lifecycle evidence")
	}

	assertIntQuery(t, db, `SELECT count(*) FROM named_stays WHERE id = 20 AND first_known_at IS NULL`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM named_stays WHERE id = 21 AND first_known_at IS NOT NULL AND cancellation_effective_at IS NULL`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`, 1)
	assertIntQuery(t, db, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`, 0)
	assertDatabaseChecks(t, db)
}

func TestLegacyOccupancyRemovalAcceptsRejectedReviewResolution(t *testing.T) {
	db := openMigrationTestDB(t)
	applyMigrationsThrough(t, db, "000038_named_stay_lifecycle_metadata.up.sql")
	seedCleanupFixture(t, db)
	mustExec(t, db, `
		UPDATE named_stays
		SET review_status = 'needs_review', review_resolution = 'rejected',
		    review_actor_user_id = 1, reviewed_at = '2026-01-02T00:00:00Z'
		WHERE id = 21`)

	if err := applyCleanupInTransaction(t, db); err != nil {
		t.Fatalf("cleanup rejected a resolved rejection: %v", err)
	}
	var legacyStatus, resolution string
	if err := db.QueryRow(`SELECT review_status, review_resolution FROM named_stays WHERE id = 21`).Scan(&legacyStatus, &resolution); err != nil {
		t.Fatal(err)
	}
	if legacyStatus != "needs_review" || resolution != "rejected" {
		t.Fatalf("review state=%q/%q want needs_review/rejected", legacyStatus, resolution)
	}
}

func seedCleanupFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-01-01T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at)
		VALUES (1, 'cleanup@test.local', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at)
		VALUES (1, 'Primary', 'UTC', 1, ?, ?), (2, 'Other', 'UTC', 1, ?, ?)`, now, now, now, now)
	mustExec(t, db, `
		INSERT INTO occupancies (
			id, property_id, source_type, source_event_uid, start_at, end_at, status,
			guest_display_name, content_hash, imported_at, last_synced_at
		) VALUES (10, 1, 'booking_ics', 'legacy-10', '2026-01-01T00:00:00Z',
			'2026-01-03T00:00:00Z', 'active', 'Guest', 'legacy-hash', ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, review_status, review_resolution, first_known_at,
			nuki_generation_status, created_at, updated_at
		) VALUES
			(20, 1, 'Guest', 'booking_com', '2026-01-01', '2026-01-03',
			 'active', 1, 'confirmed', 'confirmed', ?, 'generated', ?, ?),
			(21, 1, 'Archived', 'external', '2026-02-01', '2026-02-02',
			 'archived', 0, 'confirmed', 'confirmed', ?, 'not_applicable', ?, ?)`, now, now, now, now, now, now)
	mustExec(t, db, `INSERT INTO named_stay_nights
		(id, property_id, named_stay_id, local_night_date, active, created_at)
		VALUES (201, 1, 20, '2026-01-01', 1, ?), (202, 1, 20, '2026-01-02', 1, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO raw_booking_blocks (
			id, property_id, source_type, source_event_uid, check_in_date, check_out_date,
			status, raw_summary, content_hash, imported_at, last_synced_at, created_at, updated_at
		) VALUES (30, 1, 'booking_ics', 'raw-30', '2026-01-01', '2026-01-03',
			'active', 'Reserved', 'raw-hash', ?, ?, ?, ?)`, now, now, now, now)
	mustExec(t, db, `INSERT INTO raw_booking_block_nights
		(id, property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
		VALUES (301, 1, 30, '2026-01-01', 1, ?, ?),
		       (302, 1, 30, '2026-01-02', 1, ?, ?)`, now, now, now, now)
	mustExec(t, db, `
		INSERT INTO stay_source_links (
			id, property_id, named_stay_id, raw_booking_block_id, source_type,
			source_event_uid, linked_check_in_date, linked_check_out_date, link_status,
			created_at, updated_at
		) VALUES (32, 1, 20, 30, 'booking_ics', 'raw-30', '2026-01-01',
			'2026-01-03', 'active', ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO property_availability_blocks (
			id, property_id, block_type, start_date, end_date, reason, status,
			created_by_user_id, updated_by_user_id, created_at, updated_at
		) VALUES (40, 1, 'off_market', '2026-03-01', '2026-03-03', 'Repairs',
			'active', 1, 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO occupancy_stay_migration_map
		(old_occupancy_id, property_id, named_stay_id, migration_kind, notes, created_at)
		VALUES (10, 1, 20, 'named_stay', 'verified', ?)`, now)

	mustExec(t, db, `INSERT INTO nuki_sync_runs
		(id, property_id, started_at, finished_at, status, trigger, created_at)
		VALUES (50, 1, ?, ?, 'success', 'manual', ?)`, now, now, now)
	mustExec(t, db, `
		INSERT INTO nuki_access_codes (
			id, property_id, occupancy_id, named_stay_id, code_label, access_code_masked,
			generated_pin_plain, external_nuki_id, valid_from, valid_until, status,
			error_message, last_sync_run_id, created_at, updated_at, revoked_at
		) VALUES (60, 1, 10, 20, 'Guest code', '12**', 'stored-pin-ciphertext',
			'nuki-external-60', ?, '2026-01-03T10:00:00Z', 'revoked', 'kept error',
			50, ?, ?, '2026-01-05T00:00:00Z')`, now, now, now)
	mustExec(t, db, `INSERT INTO nuki_event_logs
		(id, property_id, nuki_access_code_id, sync_run_id, event_type, message, payload_json, created_at)
		VALUES (61, 1, 60, 50, 'revoked', 'kept log', '{"event":61}', ?)`, now)
	mustExec(t, db, `INSERT INTO nuki_guest_daily_entries
		(id, property_id, occupancy_id, named_stay_id, day_date, first_entry_at,
		 nuki_event_reference, created_at)
		VALUES (62, 1, 10, 20, '2026-01-01', '2026-01-01T15:00:00Z', 'event-62', ?)`, now)

	mustExec(t, db, `INSERT INTO cleaning_calendar_sync_runs
		(id, property_id, started_at, finished_at, status, trigger, created_at)
		VALUES (70, 1, ?, ?, 'success', 'manual', ?)`, now, now, now)
	mustExec(t, db, `
		INSERT INTO cleaning_calendar_events (
			id, property_id, occupancy_id, upstream_event_uid, checkout_date, cleaning_kind,
			google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at,
			same_day_arrival, next_occupancy_id, title, status, warning_message,
			error_message, last_synced_at, created_at, updated_at, named_stay_id,
			raw_booking_block_id, cleaning_identity, desired_hash, last_google_seen_at
		) VALUES
			(80, 1, 10, 'raw-30', '2026-01-03', 'named_stay', 'calendar-1',
			 'google-event-80', '2026-01-03', '2026-01-03T10:00:00Z',
			 '2026-01-03T13:00:00Z', 1, NULL, 'Clean Guest', 'synced', 'warning-80',
			 'error-80', ?, ?, ?, 20, NULL, 'cleaning-80', 'desired-80', ?),
			(82, 1, NULL, 'raw-30', '2026-01-02', 'provisional_block', 'calendar-1',
			 'google-event-82', '2026-01-02', '2026-01-02T10:00:00Z',
			 '2026-01-02T13:00:00Z', 0, NULL, 'Provisional', 'pending', NULL,
			 NULL, NULL, ?, ?, NULL, 30, 'cleaning-82', 'desired-82', NULL)`, now, now, now, now, now, now)
	mustExec(t, db, `INSERT INTO cleaning_calendar_event_logs
		(id, property_id, cleaning_calendar_event_id, sync_run_id, action, message, created_at)
		VALUES (81, 1, 80, 70, 'updated', 'named log', ?),
		       (83, 1, 82, 70, 'created', 'raw log', ?)`, now, now)

	mustExec(t, db, `INSERT INTO finance_transactions
		(id, property_id, transaction_date, direction, amount_cents, source_type,
		 source_reference_id, created_at, updated_at)
		VALUES (90, 1, '2026-01-04', 'incoming', 12345, 'booking_payout', 'BOOK-100', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO finance_imports
		(id, property_id, source_type, source_channel, uploaded_at, file_sha256)
		VALUES (91, 1, 'statement', 'booking_com', ?, 'sha256-fixture')`, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			id, property_id, reference_number, payout_id, row_type, check_in_date,
			check_out_date, guest_name, reservation_status, currency, payment_status,
			amount_cents, commission_cents, payment_service_fee_cents, net_cents,
			payout_date, transaction_id, occupancy_id, raw_payout_row_json, created_at,
			updated_at, booked_on, original_amount_cents, commission_pct, persons, rooms,
			room_nights, booker_name, guest_request, invoice_number, hotel_id,
			property_label, country, source_channel, has_payout_data, has_statement_data,
			raw_statement_row_json, status, outcome_override, outcome_override_marked_at,
			named_stay_id
		) VALUES (100, 1, 'BOOK-100', 'PAYOUT-100', 'reservation', '2026-01-01',
			'2026-01-03', 'Guest', 'OK', 'EUR', 'paid', 15000, 2000, 655, 12345,
			'2026-01-04', 90, 10, '{"payout":true}', ?, ?, '2025-12-01', 15000,
			13.3, 2, 1, 2, 'Booker', 'Late arrival', 'B-COM-INV', 'HOTEL-1',
			'Primary', 'SK', 'booking_com', 1, 1, '{"statement":true}', 'OK',
			'no_show', ?, 20)`, now, now, now)
	mustExec(t, db, `UPDATE occupancies SET finance_booking_id = 100 WHERE id = 10`)
	mustExec(t, db, `INSERT INTO finance_booking_merges
		(id, booking_id, import_id, source_type, changed_fields_json, occurred_at)
		VALUES (101, 100, 91, 'statement', '["status"]', ?)`, now)
	mustExec(t, db, `
		INSERT INTO invoices (
			id, property_id, occupancy_id, invoice_number, sequence_year, sequence_value,
			language, issue_date, taxable_supply_date, due_date, stay_start_date,
			stay_end_date, supplier_snapshot_json, customer_snapshot_json,
			amount_total_cents, currency, payment_status, payment_note, version,
			created_by, created_at, updated_at, finance_booking_payout_id, named_stay_id
		) VALUES (110, 1, 10, 'INV-2026-001', 2026, 1, 'en', '2026-01-03',
			'2026-01-03', '2026-01-10', '2026-01-01', '2026-01-03',
			'{"supplier":"snapshot"}', '{"customer":"snapshot"}', 12345, 'EUR',
			'paid', 'Paid by card', 1, 1, ?, ?, 100, 20)`, now, now)
	mustExec(t, db, `INSERT INTO invoice_files
		(id, invoice_id, version, file_path, file_size_bytes, created_at)
		VALUES (111, 110, 1, '/invoices/110-v1.pdf', 4321, ?)`, now)

	for _, table := range []string{
		"nuki_access_codes", "nuki_event_logs", "nuki_guest_daily_entries",
		"cleaning_calendar_events", "cleaning_calendar_event_logs", "finance_bookings",
		"finance_booking_merges", "invoices", "invoice_files", "property_availability_blocks",
		"named_stay_nights", "raw_booking_block_nights", "stay_source_links",
	} {
		mustExec(t, db, `UPDATE sqlite_sequence SET seq = 500 WHERE name = ?`, table)
	}
}

func snapshotMigrationDatabase(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var snapshot []string
	var tables []string
	rows, err := db.Query(`
		SELECT type, name, tbl_name, sql
		FROM sqlite_schema
		ORDER BY type, name, tbl_name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var objectType, name, table string
		var definition sql.NullString
		if err := rows.Scan(&objectType, &name, &table, &definition); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		snapshot = append(snapshot, strings.Join([]string{"schema", objectType, name, table, definition.String}, "\x00"))
		if objectType == "table" {
			tables = append(tables, name)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, table := range tables {
		rows, err := db.Query(`SELECT * FROM ` + quoteMigrationIdentifier(table))
		if err != nil {
			t.Fatalf("snapshot table %s: %v", table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatal(err)
		}
		snapshot = append(snapshot, strings.Join(append([]string{"columns", table}, columns...), "\x00"))
		var tableRows []string
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err := rows.Scan(destinations...); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			encoded := []string{"row", table}
			for _, value := range values {
				encoded = append(encoded, encodeMigrationSnapshotValue(value))
			}
			tableRows = append(tableRows, strings.Join(encoded, "\x00"))
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		sort.Strings(tableRows)
		snapshot = append(snapshot, tableRows...)
	}

	rows, err = db.Query(`
		SELECT type, name, tbl_name, COALESCE(sql, '')
		FROM sqlite_temp_schema
		ORDER BY type, name, tbl_name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var objectType, name, table, definition string
		if err := rows.Scan(&objectType, &name, &table, &definition); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		snapshot = append(snapshot, strings.Join([]string{"temp-schema", objectType, name, table, definition}, "\x00"))
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func normalizedMigrationSchemaShape(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var shape []string
	var tables []string
	rows, err := db.Query(`
		SELECT type, name, tbl_name, sql
		FROM sqlite_schema
		ORDER BY type, name, tbl_name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var objectType, name, table string
		var definition sql.NullString
		if err := rows.Scan(&objectType, &name, &table, &definition); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		shape = append(shape, strings.Join([]string{
			"schema", objectType, name, table, strings.Join(strings.Fields(definition.String), " "),
		}, "\x00"))
		if objectType == "table" {
			tables = append(tables, name)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, table := range tables {
		indexRows, err := db.Query(`
			SELECT name, "unique", origin, partial
			FROM pragma_index_list(?)
			ORDER BY name`, table)
		if err != nil {
			t.Fatalf("indexes for %s: %v", table, err)
		}
		var indexes []string
		for indexRows.Next() {
			var name, origin string
			var unique, partial int
			if err := indexRows.Scan(&name, &unique, &origin, &partial); err != nil {
				indexRows.Close()
				t.Fatal(err)
			}
			indexes = append(indexes, name)
			shape = append(shape, strings.Join([]string{
				"index", table, name, strconv.Itoa(unique), origin, strconv.Itoa(partial),
			}, "\x00"))
		}
		if err := indexRows.Close(); err != nil {
			t.Fatal(err)
		}
		if err := indexRows.Err(); err != nil {
			t.Fatal(err)
		}
		for _, index := range indexes {
			detailRows, err := db.Query(`
				SELECT seqno, cid, name, "desc", coll, key
				FROM pragma_index_xinfo(?)
				ORDER BY seqno`, index)
			if err != nil {
				t.Fatalf("index shape for %s: %v", index, err)
			}
			for detailRows.Next() {
				var seqno, cid, descending, key int
				var name, collation sql.NullString
				if err := detailRows.Scan(&seqno, &cid, &name, &descending, &collation, &key); err != nil {
					detailRows.Close()
					t.Fatal(err)
				}
				shape = append(shape, strings.Join([]string{
					"index-column", table, index, strconv.Itoa(seqno), strconv.Itoa(cid),
					name.String, strconv.Itoa(descending), collation.String, strconv.Itoa(key),
				}, "\x00"))
			}
			if err := detailRows.Close(); err != nil {
				t.Fatal(err)
			}
			if err := detailRows.Err(); err != nil {
				t.Fatal(err)
			}
		}

		foreignKeyRows, err := db.Query(`
			SELECT id, seq, "table", "from", "to", on_update, on_delete, match
			FROM pragma_foreign_key_list(?)
			ORDER BY id, seq`, table)
		if err != nil {
			t.Fatalf("foreign keys for %s: %v", table, err)
		}
		for foreignKeyRows.Next() {
			var id, sequence int
			var parent, from, onUpdate, onDelete, match string
			var to sql.NullString
			if err := foreignKeyRows.Scan(&id, &sequence, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				foreignKeyRows.Close()
				t.Fatal(err)
			}
			shape = append(shape, strings.Join([]string{
				"foreign-key", table, strconv.Itoa(id), strconv.Itoa(sequence), parent,
				from, to.String, onUpdate, onDelete, match,
			}, "\x00"))
		}
		if err := foreignKeyRows.Close(); err != nil {
			t.Fatal(err)
		}
		if err := foreignKeyRows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(shape)
	return shape
}

func quoteMigrationIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func encodeMigrationSnapshotValue(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case int64:
		return "integer:" + strconv.FormatInt(value, 10)
	case float64:
		return "real:" + strconv.FormatFloat(value, 'g', -1, 64)
	case bool:
		return "boolean:" + strconv.FormatBool(value)
	case []byte:
		return "blob:" + hex.EncodeToString(value)
	case string:
		return "text:" + hex.EncodeToString([]byte(value))
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func assertMigrationSnapshotsEqual(t *testing.T, want, got []string) {
	t.Helper()
	limit := len(want)
	if len(got) < limit {
		limit = len(got)
	}
	for i := 0; i < limit; i++ {
		if want[i] != got[i] {
			t.Fatalf("migration snapshot differs at entry %d:\nwant %q\n got %q", i, want[i], got[i])
		}
	}
	if len(want) != len(got) {
		t.Fatalf("migration snapshot entry count=%d want %d", len(got), len(want))
	}
}

func assertFinalConstraints(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-04-01T00:00:00Z"
	assertExecFails(t, db, `INSERT INTO nuki_access_codes
		(property_id, named_stay_id, code_label, valid_from, valid_until, status, created_at, updated_at)
		VALUES (1, NULL, 'null owner', ?, ?, 'not_generated', ?, ?)`, now, now, now, now)
	assertExecFails(t, db, `INSERT INTO nuki_access_codes
		(property_id, named_stay_id, code_label, valid_from, valid_until, status, created_at, updated_at)
		VALUES (2, 20, 'cross property', ?, ?, 'not_generated', ?, ?)`, now, now, now, now)
	assertExecFails(t, db, `INSERT INTO nuki_guest_daily_entries
		(property_id, named_stay_id, day_date, first_entry_at, created_at)
		VALUES (1, NULL, '2026-04-01', ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO nuki_guest_daily_entries
		(property_id, named_stay_id, day_date, first_entry_at, created_at)
		VALUES (2, 20, '2026-04-01', ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO finance_bookings
		(property_id, reference_number, net_cents, payout_date, named_stay_id, created_at, updated_at)
		VALUES (1, 'NULL-STAY', 1, '2026-04-01', NULL, ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO finance_bookings
		(property_id, reference_number, net_cents, payout_date, named_stay_id, created_at, updated_at)
		VALUES (2, 'CROSS-STAY', 1, '2026-04-01', 20, ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO invoices
		(property_id, invoice_number, sequence_year, sequence_value, language, issue_date,
		 taxable_supply_date, due_date, stay_start_date, stay_end_date,
		 supplier_snapshot_json, customer_snapshot_json, amount_total_cents, payment_note,
		 created_at, updated_at, named_stay_id)
		VALUES (1, 'NULL-STAY', 2026, 2, 'en', '2026-04-01', '2026-04-01',
		 '2026-04-02', '2026-04-01', '2026-04-02', '{}', '{}', 1, 'x', ?, ?, NULL)`, now, now)
	assertExecFails(t, db, `INSERT INTO cleaning_calendar_events
		(property_id, cleaning_kind, google_calendar_id, cleaning_date, starts_at, ends_at,
		 title, status, created_at, updated_at)
		VALUES (1, 'named_stay', 'calendar', '2026-04-01', ?, ?, 'No owner', 'pending', ?, ?)`, now, now, now, now)
	assertExecFails(t, db, `INSERT INTO cleaning_calendar_events
		(property_id, cleaning_kind, google_calendar_id, cleaning_date, starts_at, ends_at,
		 title, status, named_stay_id, raw_booking_block_id, created_at, updated_at)
		VALUES (1, 'named_stay', 'calendar', '2026-04-01', ?, ?, 'Both', 'pending', 20, 30, ?, ?)`, now, now, now, now)
	assertExecFails(t, db, `INSERT INTO cleaning_calendar_events
		(property_id, cleaning_kind, google_calendar_id, cleaning_date, starts_at, ends_at,
		 title, status, named_stay_id, created_at, updated_at)
		VALUES (2, 'named_stay', 'calendar', '2026-04-01', ?, ?, 'Cross', 'pending', 20, ?, ?)`, now, now, now, now)
	assertExecFails(t, db, `INSERT INTO named_stay_nights
		(property_id, named_stay_id, local_night_date, active, created_at)
		VALUES (2, 20, '2026-04-01', 0, ?)`, now)
	assertExecFails(t, db, `INSERT INTO raw_booking_block_nights
		(property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
		VALUES (2, 30, '2026-04-01', 0, ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO stay_source_links
		(property_id, named_stay_id, raw_booking_block_id, linked_check_in_date,
		 linked_check_out_date, link_status, created_at, updated_at)
		VALUES (2, 20, 30, '2026-01-01', '2026-01-03', 'active', ?, ?)`, now, now)

	mustExec(t, db, `INSERT INTO finance_bookings
		(property_id, reference_number, net_cents, payout_date, named_stay_id, created_at, updated_at)
		VALUES (1, 'BOOK-SECOND', 100, '2026-04-01', 21, ?, ?)`, now, now)
	assertExecFails(t, db, `INSERT INTO invoices
		(property_id, invoice_number, sequence_year, sequence_value, language, issue_date,
		 taxable_supply_date, due_date, stay_start_date, stay_end_date,
		 supplier_snapshot_json, customer_snapshot_json, amount_total_cents, payment_note,
		 created_at, updated_at, finance_booking_payout_id, named_stay_id)
		VALUES (1, 'MISMATCH', 2026, 3, 'en', '2026-04-01', '2026-04-01',
		 '2026-04-02', '2026-04-01', '2026-04-02', '{}', '{}', 1, 'x', ?, ?, 100, 21)`, now, now)
	assertExecFails(t, db, `DELETE FROM named_stays WHERE id = 20`)
	assertExecFails(t, db, `DELETE FROM nuki_access_codes WHERE id = 60`)
	assertExecFails(t, db, `DELETE FROM cleaning_calendar_events WHERE id = 80`)
	assertExecFails(t, db, `DELETE FROM finance_bookings WHERE id = 100`)
	assertExecFails(t, db, `DELETE FROM invoices WHERE id = 110`)

	mustExec(t, db, `INSERT INTO named_stays
		(id, property_id, display_name, stay_type, check_in_date, check_out_date, status,
		 cleaning_required, review_status, first_known_at, created_at, updated_at)
		VALUES (22, 1, 'Cascade stay', 'external', '2026-05-01', '2026-05-02',
		 'archived', 0, 'confirmed', ?, ?, ?)`, now, now, now)
	mustExec(t, db, `INSERT INTO named_stay_nights
		(property_id, named_stay_id, local_night_date, active, created_at)
		VALUES (1, 22, '2026-05-01', 0, ?)`, now)
	mustExec(t, db, `DELETE FROM named_stays WHERE id = 22`)
	assertIntQuery(t, db, `SELECT count(*) FROM named_stay_nights WHERE named_stay_id = 22`, 0)
	mustExec(t, db, `INSERT INTO raw_booking_blocks
		(id, property_id, source_event_uid, check_in_date, check_out_date, status,
		 content_hash, imported_at, last_synced_at, created_at, updated_at)
		VALUES (31, 1, 'cascade-raw', '2026-05-01', '2026-05-02', 'deleted_from_source',
		 'hash', ?, ?, ?, ?)`, now, now, now, now)
	mustExec(t, db, `INSERT INTO raw_booking_block_nights
		(property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
		VALUES (1, 31, '2026-05-01', 0, ?, ?)`, now, now)
	mustExec(t, db, `DELETE FROM raw_booking_blocks WHERE id = 31`)
	assertIntQuery(t, db, `SELECT count(*) FROM raw_booking_block_nights WHERE raw_booking_block_id = 31`, 0)
}

func assertCleanLatestSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	assertIntQuery(t, db, `
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN
		('occupancies', 'occupancy_nights', 'occupancy_api_tokens', 'occupancy_stay_migration_map')`, 0)
	assertIntQuery(t, db, `
		SELECT count(*)
		FROM sqlite_schema s, pragma_table_info(s.name) c
		WHERE s.type = 'table' AND c.name IN
		('occupancy_id', 'next_occupancy_id', 'source_occupancy_id', 'old_occupancy_id')`, 0)
	assertIntQuery(t, db, `
		SELECT count(*) FROM sqlite_schema
		WHERE lower(coalesce(sql, '')) LIKE '%references occupancies%'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM pragma_table_info('named_stays') WHERE name = 'stay_outcome_marked_by_user_id'`, 0)
	assertIntQuery(t, db, `SELECT count(*) FROM pragma_table_info('named_stays') WHERE name IN ('stay_outcome_actor_user_id', 'review_resolution')`, 2)

	for table, columns := range map[string][]string{
		"nuki_access_codes":            {"generated_pin_plain", "external_nuki_id", "last_sync_run_id", "revoked_at"},
		"cleaning_calendar_events":     {"upstream_event_uid", "cleaning_identity", "desired_hash", "last_google_seen_at"},
		"finance_bookings":             {"raw_payout_row_json", "raw_statement_row_json", "outcome_override", "named_stay_id"},
		"invoices":                     {"supplier_snapshot_json", "finance_booking_payout_id", "named_stay_id"},
		"property_availability_blocks": {"reason", "created_by_user_id", "updated_by_user_id"},
	} {
		for _, column := range columns {
			assertIntQuery(t, db, `SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, 1, table, column)
		}
	}
}

func assertExpectedIndexes(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{
		"uq_named_stays_property_id", "uq_raw_booking_blocks_property_id",
		"uq_nuki_access_codes_property_named_stay", "uq_nuki_guest_daily_entries_property_named_stay_day",
		"uq_cleaning_calendar_identity", "uq_cleaning_calendar_events_identity",
		"ux_finance_bookings_property_channel_reference", "ux_invoices_property_booking_payout",
		"ux_invoices_property_named_stay", "uq_named_stay_nights_active_property_date",
	} {
		assertIntQuery(t, db, `SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?`, 1, name)
	}
}

func assertDatabaseChecks(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned a violation")
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q", integrity)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func assertExecFails(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatalf("statement unexpectedly succeeded: %s", query)
	}
}

func assertIntQuery(t *testing.T, db *sql.DB, query string, want int64, args ...any) {
	t.Helper()
	var got int64
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("query %s: %v", query, err)
	}
	if got != want {
		t.Fatalf("query %s=%d want %d", query, got, want)
	}
}

func TestCleanupMigrationIsEmbeddedAfterLifecycleMetadata(t *testing.T) {
	entries, err := fs.ReadDir(embeddedMigrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, entry := range entries {
		if path.Base(entry.Name()) == cleanupMigration {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s is not embedded", cleanupMigration)
	}
}
