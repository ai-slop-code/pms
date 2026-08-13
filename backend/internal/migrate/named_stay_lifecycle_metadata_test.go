package migrate

import (
	"database/sql"
	"testing"

	"pms/backend/internal/dbconn"
)

func TestNamedStayLifecycleMetadataBackfillUsesDefensibleEvidence(t *testing.T) {
	db, err := dbconn.Open("sqlite://" + t.TempDir() + "/migration.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	applyMigrationsThrough(t, db, "000037_finance_evidence_confirms_named_stays.up.sql")

	created := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'lifecycle@test.local', 'hash', 'owner', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Migration', 'UTC', 1, ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	for id, status := range map[int]string{
		10: "active",
		11: "cancelled",
		12: "archived",
		13: "archived",
		14: "cancelled",
		15: "cancelled",
		16: "archived",
		17: "archived",
	} {
		if _, err := db.Exec(`
			INSERT INTO named_stays (
				id, property_id, display_name, stay_type, check_in_date, check_out_date,
				status, cleaning_required, review_status, nuki_generation_status,
				stay_outcome, stay_outcome_reason, stay_outcome_marked_by_user_id,
				stay_outcome_marked_at, created_at, updated_at
			) VALUES (?, 1, ?, 'booking_com', '2026-02-01', '2026-02-02', ?, 1,
				'confirmed', 'not_applicable', 'no_show', 'legacy reason', 1,
				NULL, ?, ?)`,
			id, status, status, created, created); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE named_stays SET stay_outcome_marked_at = '2025-06-01T00:00:00Z' WHERE id = 11`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE named_stays SET created_by_user_id = 1 WHERE id IN (12, 13)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE named_stays SET source_channel = 'manual' WHERE id IN (12, 16, 17)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, booked_on, created_at, updated_at
		) VALUES (1, 'DIRECT', '2026-02-01', '2026-02-02', 'active', 10000,
			'2026-02-03', 10, '2025-01-01T00:00:00Z', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, named_stay_id, check_in_date, check_out_date,
			guest_name, net_cents, payout_date, created_at, updated_at
		) VALUES (1, 'MANUAL-FINANCE-MIGRATION', 17, '2026-05-01', '2026-05-02',
			'manual finance migration', 0, '2026-05-03', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			content_hash, imported_at, last_synced_at, stay_outcome, stay_outcome_marked_at
		) VALUES (1, 'booking_payout', 'mapped', '2026-02-01T00:00:00Z',
			'2026-02-02T00:00:00Z', 'cancelled', 'hash', '2025-02-01T00:00:00Z', '2025-02-15T00:00:00Z',
			'no_show', '2025-04-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	occupancyID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO occupancy_stay_migration_map (
			old_occupancy_id, property_id, named_stay_id, migration_kind, created_at
		) VALUES (?, 1, 11, 'named_stay', ?)`, occupancyID, created); err != nil {
		t.Fatal(err)
	}
	res, err = db.Exec(`
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			content_hash, imported_at, last_synced_at
		) VALUES (1, 'manual', 'unsupported-migrated', '2026-02-01T00:00:00Z',
			'2026-02-02T00:00:00Z', 'archived', 'unsupported-hash', 'not-a-date', ?)`, created)
	if err != nil {
		t.Fatal(err)
	}
	unsupportedOccupancyID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO occupancy_stay_migration_map (
			old_occupancy_id, property_id, named_stay_id, migration_kind, created_at
		) VALUES (?, 1, 12, 'named_stay', ?)`, unsupportedOccupancyID, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, occupancy_id, check_in_date, check_out_date,
			guest_name, net_cents, payout_date, booked_on, outcome_override,
			outcome_override_marked_at, created_at, updated_at
		) VALUES (1, 'MAPPED', ?, '2026-02-01', '2026-02-02', 'mapped', 10000,
			'2026-02-03', '2025-03-01T00:00:00Z', 'no_show',
			'2025-03-15T00:00:00Z', ?, ?)`, occupancyID, created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, named_stay_id, check_in_date, check_out_date,
			guest_name, net_cents, payout_date, booked_on, outcome_override,
			outcome_override_marked_at, created_at, updated_at
		) VALUES (1, 'DIRECT-LATER', 11, '2026-02-01', '2026-02-02', 'direct later',
			10000, '2026-02-03', '2025-04-01T00:00:00Z', 'no_show',
			'2025-03-01T00:00:00Z', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}
	res, err = db.Exec(`
		INSERT INTO raw_booking_blocks (
			property_id, source_event_uid, check_in_date, check_out_date, status,
			content_hash, imported_at, last_synced_at, created_at, updated_at,
			deleted_from_source_at
		) VALUES (1, 'raw-earliest', '2026-02-01', '2026-02-02', 'deleted_from_source', 'raw-hash',
			'2024-12-01T00:00:00Z', ?, ?, ?, '2025-02-01T00:00:00Z')`, created, created, created)
	if err != nil {
		t.Fatal(err)
	}
	rawID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO stay_source_links (
			property_id, named_stay_id, raw_booking_block_id, linked_check_in_date,
			linked_check_out_date, link_status, created_at, updated_at
		) VALUES (1, 11, ?, '2026-02-01', '2026-02-02', 'source_deleted', ?, ?)`, rawID, created, created); err != nil {
		t.Fatal(err)
	}
	res, err = db.Exec(`
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			content_hash, imported_at, last_synced_at
		) VALUES (1, 'booking_payout', 'cancelled-sync', '2026-03-01T00:00:00Z',
			'2026-03-02T00:00:00Z', 'cancelled', 'sync-hash',
			'2025-01-10T00:00:00Z', '2025-01-20T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	cancelledOccupancyID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO occupancy_stay_migration_map (
			old_occupancy_id, property_id, named_stay_id, migration_kind, created_at
		) VALUES (?, 1, 14, 'named_stay', ?)`, cancelledOccupancyID, created); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, named_stay_id, check_in_date, check_out_date,
			guest_name, net_cents, payout_date, booked_on, outcome_override,
			outcome_override_marked_at, created_at, updated_at
		) VALUES (1, 'UNSUPPORTED-CANCEL', 15, '2026-04-01', '2026-04-02',
			'unsupported cancellation', 0, '2026-04-03', '2025-01-05T00:00:00Z',
			'no_show', 'not-a-date', ?, ?)`, created, created); err != nil {
		t.Fatal(err)
	}

	body, err := embeddedMigrations.ReadFile("000038_named_stay_lifecycle_metadata.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[int]struct {
		firstKnown   string
		cancellation string
	}{
		10: {firstKnown: "2025-01-01T00:00:00Z"},
		11: {firstKnown: "2024-12-01T00:00:00Z", cancellation: "2025-02-01T00:00:00Z"},
		12: {},
		13: {firstKnown: created},
		14: {firstKnown: "2025-01-10T00:00:00Z", cancellation: "2025-01-20T00:00:00Z"},
		15: {firstKnown: "2025-01-05T00:00:00Z"},
		16: {firstKnown: created},
		17: {},
	} {
		var firstKnown sql.NullString
		var cancellation sql.NullString
		var outcomeActor sql.NullInt64
		var reviewResolution sql.NullString
		if err := db.QueryRow(`
			SELECT first_known_at, cancellation_effective_at, stay_outcome_actor_user_id, review_resolution
			FROM named_stays WHERE id = ?`, id).Scan(&firstKnown, &cancellation, &outcomeActor, &reviewResolution); err != nil {
			t.Fatal(err)
		}
		if firstKnown.String != want.firstKnown || firstKnown.Valid != (want.firstKnown != "") ||
			outcomeActor.Int64 != 1 || reviewResolution.String != "confirmed" {
			t.Fatalf("stay %d first_known=%v actor=%v resolution=%v want %q/1/confirmed", id, firstKnown, outcomeActor, reviewResolution, want.firstKnown)
		}
		if cancellation.String != want.cancellation || cancellation.Valid != (want.cancellation != "") {
			t.Fatalf("stay %d cancellation=%v want %q", id, cancellation, want.cancellation)
		}
	}
	var legacyActorColumns int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('named_stays') WHERE name = 'stay_outcome_marked_by_user_id'`).Scan(&legacyActorColumns); err != nil {
		t.Fatal(err)
	}
	if legacyActorColumns != 0 {
		t.Fatal("legacy stay outcome actor column was retained")
	}
}
